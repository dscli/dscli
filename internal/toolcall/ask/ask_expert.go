package ask

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	ictx "github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/lp"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/toolcall"
	"github.com/nanjj/clog"
)

//go:embed ask_expert.md
var ask_expert_md string

// askExpertTool tool definition
var askExpertTool = toolcall.ToolDef{
	Name:        "ask_expert",
	DisplayName: "Ask Expert",
	Description: ask_expert_md,
	Strict:      true,
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"input": map[string]any{
				"type":        "string",
				"description": "The question to ask. A value starting with @ reads the file at that path (safe paths: cwd, ~/..., $HOME, or /tmp; max 1MB); otherwise sent as plain text.",
			},
			"attachments": map[string]any{
				"type":        "array",
				"description": "File attachments (optional). Images uploaded for visual analysis; other files inlined as text (1MB max each, safe paths: cwd, $HOME, or /tmp; ≤50 files).",
				"items": map[string]string{
					"type":        "string",
					"description": "Attachment filename",
				},
			},
			"mode": map[string]any{
				"type":        "string",
				"description": "Web chat mode: flash (fast, smart search), pro (expert, default), vision (image uploads). Empty: vision if images attached, else pro.",
			},
			"keep": map[string]any{
				"type":        "string",
				"description": "Continue a previous conversation (default new). Pass the conversation_id from a previous result, \"last\" (most recent), or a chat.deepseek.com URL; \"list\" lists saved conversations.",
			},
			"timeout": map[string]any{
				"type":        "integer",
				"description": "Timeout in seconds (default 600). Set longer for complex questions.",
			},
			"raw": map[string]any{
				"type":        "boolean",
				"description": "Send input verbatim, skipping dscli's default response template (default false). Use when the prompt defines the output format (e.g. JSON extraction).",
			},
		},
		"required":             []string{"input"},
		"additionalProperties": false,
	},
	Category: "communication",
	Timeout:  10 * time.Minute, // 10 minutes for expert to respond
	Handler:  handleAskExpert,
}

// askExpertWithRoleFunc is the function used to call the expert.
// It is a package-level variable so tests can replace it with a mock.
// mode selects the web chat mode ("" = auto: pro, or vision with image
// uploads); keep continues a previous conversation ("" = new, "last" =
// most recent, or a conversation ID/URL); attachments are image files
// uploaded to the web chat. It returns the reply text, the conversation
// URL (empty when unknown) so callers can continue the conversation later,
// and printed - the reply was already printed by the DSML tool loop, so
// callers must not re-print it (it would duplicate the final answer).
var askExpertWithRoleFunc = askExpertWebChat

func init() {
	// WebChat is always available (free DeepSeek V4 Pro) — no API key needed.
	// The only prerequisite is Chrome installed and logged in once.
	toolcall.RegisterTool(askExpertTool)

	// Test optimization: use mock to skip browser automation.
	if ictx.IsTesting() {
		askExpertWithRoleFunc = func(_ context.Context, _, _, _, _, _ string, _ []string) (string, string, bool, error) {
			return "[MOCK]", "", false, nil
		}
	}
}

// handleAskExpert handles the ask_expert tool call.
func handleAskExpert(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	span, ctx := clog.StartSpanFromContext(ctx, "handleAskExpert")
	defer span.Finish()
	input := toolcall.ToolArgsValue(args, "input", "")
	// required only guarantees the key exists; the LLM may pass an empty string.
	if strings.TrimSpace(input) == "" {
		err = fmt.Errorf("input is required")
		return result, warning, err
	}

	// An @-prefixed input is a file reference (e.g. @question.txt): read the
	// file when it is a safe path and exists. Lenient fallback: anything else
	// is sent as plain text, so natural language starting with @ (e.g. "@user
	// ...") is never mangled into an error.
	content := input
	if strings.HasPrefix(input, "@") && len(input) > 1 {
		candidate := input[1:]
		if isSafePath(candidate) {
			fileContent, readErr := readContentFile(candidate)
			switch {
			case readErr == nil:
				content = fileContent
				outfmt.Printf("📂 Read question from file: %s (%d bytes)\n", candidate, len(content))
			case errors.Is(readErr, os.ErrNotExist):
				// Not a real file: likely natural language. Fall back to
				// plain text but say so, so a misspelled filename is not
				// silently swallowed.
				outfmt.Printf("⚠️  @%s not found, sending as plain text\n", candidate)
			default:
				// The file exists but cannot be used (too large, empty,
				// symlink escapes cwd): fail loudly instead of silently
				// dropping the user's intent.
				err = readErr
				return result, warning, err
			}
		}
	}

	mode := toolcall.ToolArgsValue(args, "mode", "")
	keep := toolcall.ToolArgsValue(args, "keep", "")
	attachments := toolcall.ToolArgsValue(args, "attachments", []string{})
	raw := toolcall.ToolArgsValue(args, "raw", false)

	// keep="list" is a query, not a message: return the saved conversation
	// registry so the caller can pick an ID to continue.
	if keep == "list" {
		return listConversations()
	}

	// Split attachments by type: image files are uploaded to the web chat
	// (flash/vision modes), everything else is inlined as text. Every
	// attachment is sandboxed to the current directory, the user's home, or
	// the system temp directory (symlink-resolved), so the model cannot
	// exfiltrate arbitrary local files.
	var uploads, inline []string
	for _, a := range attachments {
		// Fail fast on unsafe paths: a path that escapes the sandbox is a
		// hard error, not a skippable attachment. Returning immediately
		// guarantees no file is read and no browser work happens.
		if err := verifySafePath(a); err != nil {
			return result, warning, err
		}
		if lp.IsImageFile(a) {
			uploads = append(uploads, expandHome(a))
		} else {
			inline = append(inline, a)
		}
	}

	// Show what was asked (truncate long content for display)
	summaryDisplay := truncateForDisplay(content, 120)
	if mode != "" {
		outfmt.Printf("📞 Consulting expert via DeepSeek Web (free, mode=%s)...\n", mode)
	} else {
		outfmt.Println("📞 Consulting expert via DeepSeek Web (free V4 Pro)...")
	}
	outfmt.Println("  Question:", summaryDisplay)
	if len(uploads) > 0 {
		outfmt.Printf("📎 Uploading %d image attachment(s)...\n", len(uploads))
	}

	// Build the request with inlined text attachments. raw skips the
	// default response template so the caller's prompt goes out verbatim.
	structuredRequest, attachmentErrors := buildStructuredRequest(content, inline, raw)

	// Report attachment errors to user but continue execution
	if len(attachmentErrors) > 0 {
		outfmt.Println("⚠️  Attachment warnings:")
		for _, attachmentErr := range attachmentErrors {
			outfmt.Printf("  - %v\n", attachmentErr)
		}
	}

	// No persona is injected: role and system are both empty, so
	// askExpertWebChat sends the request verbatim (the caller's own context
	// carries the expertise). code_review still passes a role directly.
	result, convURL, printed, err := askExpertWithRoleFunc(ctx, structuredRequest, "", "", mode, keep, uploads)
	if err != nil {
		outfmt.Println("❌ Expert consultation failed")
		return result, warning, err
	}

	// Trim leading/trailing whitespace from expert response
	result = strings.TrimSpace(result)

	// Surface the conversation ID so the caller can continue this exact
	// conversation later (keep=<id>) — e.g. to correct a misread image
	// while the expert still has the original attachments in context.
	var convID string
	if convURL != "" {
		if convID = lp.ConversationIDFromURL(convURL); convID != "" {
			outfmt.Printf("📋 keep:%s (继续追问请传 keep=%s)\n", convID, convID)
			result += "\n\n---\nconversation_id: " + convID
		} else {
			outfmt.Printf("📋 conversation URL: %s\n", convURL)
		}
	}

	// The DSML tool loop already printed every round (final answer
	// included) via outfmt.PrintContent; re-printing the reply here would
	// duplicate it. printed=false (one-shot reply) keeps the old behavior.
	if printed {
		outfmt.Println("✅ Expert consultation completed")
	} else {
		outfmt.Printf("✅ Expert consultation completed\n\n%s\n", result)
	}
	return result, warning, err
}

// listConversations formats the saved conversation registry as a tool result.
func listConversations() (result, warning string, err error) {
	convs, err := lp.ListConversations()
	if err != nil {
		return "", "", err
	}
	if len(convs) == 0 {
		return "No saved conversations yet. Ask without keep to start one, or pass a chat.deepseek.com URL to register a browser conversation.", "", nil
	}
	var b strings.Builder
	b.WriteString("Saved conversations (most recent first):\n")
	for _, c := range convs {
		mode := string(c.Mode)
		if mode == "" {
			mode = "?"
		}
		fmt.Fprintf(&b, "- %s  [%s]  %s  %s\n", c.ID, mode, c.UpdatedAt, c.URL)
	}
	return b.String(), "", nil
}

// truncateForDisplay truncates s to maxLen runes for terminal display.
func truncateForDisplay(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-3]) + "..."
}

// AskExpertWithRole calls the AI model for consultation with a specified
// role (expert/review/dev) via DeepSeek Web (free V4 Pro).
//
// It renders the role-specific system prompt (e.g. expert.md, review.md),
// prepends it to the input, and sends the combined message to
// chat.deepseek.com. Each call starts a new conversation.
//
// Parameters:
//
//	ctx: context object
//	input: input text to send to the AI model
//	role: role (expert/review/dev)
//
// Returns:
//
//	reply: the AI model's response text
//	err: error during execution
func AskExpertWithRole(ctx context.Context, input, role string) (reply string, err error) {
	// printed is ignored here: the reply is already printed by the DSML
	// tool loop when the expert used tools (see askExpertWebChat).
	reply, _, _, err = askExpertWithRoleFunc(ctx, input, role, "", "", "", nil)
	return reply, err
}

// askExpertWebChat is the real implementation: it maps the expert-call
// parameters onto lp.HandleWebChat, the high-level WebChat entry point that
// renders the role/system prompt, retries transient server overload and
// truncation, and executes DSML tool calls embedded in replies (role-driven
// or plain chat alike - see toolcall.IsDSMLToolCallEnd).
//
// When both role and system are empty, no persona is injected and the input
// is sent verbatim (the ask_expert tool relies on the caller's own context;
// code_review passes a role). keep continues a previous conversation; it is
// passed through to lp.WebChatOptions.Keep ("" = new, "last", ID, or URL).
func askExpertWebChat(ctx context.Context, input, role, system, mode, keep string, attachments []string) (reply, convURL string, printed bool, err error) {
	res, err := lp.HandleWebChat(ctx, input, lp.WebChatOptions{
		Mode:        lp.Mode(mode),
		Attachments: attachments,
		Keep:        keep,
		Role:        role,
		System:      system,
	})
	if err != nil {
		return "", "", false, err
	}
	return res.Content, res.URL, res.Printed, nil
}

// maxAttachmentSize is the maximum allowed size for a single attachment (1MB).
const maxAttachmentSize = 1 << 20

// buildStructuredRequest builds the request text sent to the expert.
// When raw is true, the default response template is skipped: the caller's
// prompt is sent verbatim (text attachments are still inlined), so custom
// output formats (e.g. JSON extraction) are not polluted by the standard
// Problem Analysis / Solutions / Suggestions boilerplate. When raw is false,
// the content is wrapped in the template, which steers the expert toward a
// structured analysis.
func buildStructuredRequest(content string, attachments []string, raw bool) (string, []error) {
	var errors []error
	attachmentSection := ""

	if len(attachments) > 0 {
		var attachmentContent strings.Builder
		attachmentContent.WriteString("\n## Attachments\n")

		for _, filename := range attachments {
			// Security check: prevent path traversal attacks
			if !isSafePath(filename) {
				errors = append(errors, fmt.Errorf("unsafe path: %s", filename))
				continue
			}

			// Check file size (limit to 1MB). Open the ~/-expanded path:
			// os.Stat/ReadFile do not expand a leading ~ themselves.
			path := expandHome(filename)
			if info, err := os.Stat(path); err == nil && info.Size() > maxAttachmentSize {
				errors = append(errors, fmt.Errorf("file too large: %s (%d bytes > 1MB)", filename, info.Size()))
				continue
			}

			b, err := os.ReadFile(path)
			if err != nil {
				errors = append(errors, fmt.Errorf("failed to read file %s: %w", filename, err))
				continue
			}

			content := strings.TrimSpace(string(b))
			if content == "" {
				errors = append(errors, fmt.Errorf("file is empty: %s", filename))
				continue
			}

			// Use Markdown code block format
			fmt.Fprintf(&attachmentContent, "### %s\n```\n%s\n```\n\n", filename, content)
		}

		if attachmentContent.Len() > len("\n## Attachments\n") {
			attachmentSection = attachmentContent.String()
		}
	}

	if raw {
		// Verbatim mode: the caller's own prompt defines the format. The
		// leading newline of the attachment section keeps a separator.
		if attachmentSection == "" {
			return content, errors
		}
		return content + attachmentSection, errors
	}

	request := "Please answer the following question in a structured format.\n\n" + content + attachmentSection + `

## Response Requirements
Please provide detailed analysis and advice, including:
1. Problem Analysis: In-depth analysis of the core issues and key points
2. Solutions: Specific and feasible solutions
3. Suggestions: Actionable recommendations and considerations
4. Risk Assessment: Identify potential risks and countermeasures

## Notes
- Analysis should be logically rigorous and comprehensive
- Suggestions should be specific, actionable, and prioritized
- Risk assessment should be objective and thorough
`
	return request, errors
}

// readContentFile reads the @-referenced question text from disk.
// It applies the same safety rules as attachments: safe path only
// (current directory and subdirectories, the user's home directory via
// ~/... or absolute paths under $HOME, or the system temp directory, no
// ".." traversal), 1MB size limit, non-empty content. Errors are explicit,
// never silent.
func readContentFile(filename string) (string, error) {
	if err := verifySafePath(filename); err != nil {
		return "", err
	}

	f, err := os.Open(expandHome(filename))
	if err != nil {
		return "", fmt.Errorf("failed to open file %s: %w", filename, err)
	}
	defer f.Close()

	// Read at most maxAttachmentSize+1 bytes so an oversized file is caught
	// even if it grows between open and read (no TOCTOU window).
	b, err := io.ReadAll(io.LimitReader(f, maxAttachmentSize+1))
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %w", filename, err)
	}
	if len(b) > maxAttachmentSize {
		return "", fmt.Errorf("file too large: %s (%d bytes > 1MB)", filename, len(b))
	}

	content := strings.TrimSpace(string(b))
	if content == "" {
		return "", fmt.Errorf("file is empty: %s", filename)
	}
	return content, nil
}

// verifySafePath checks that filename is safe to read or upload: a relative
// path inside the current directory, or an absolute path under the user's
// home directory (either ~/... or $HOME/...) or the system temp directory
// (e.g. /tmp/...). isSafePath only checks the textual path, so a symlink
// inside a sandbox could otherwise smuggle in arbitrary files from outside
// it; EvalSymlinks resolves the real target (relative paths stay relative,
// so absolutize first).
func verifySafePath(filename string) error {
	if !isSafePath(filename) {
		return fmt.Errorf("unsafe path: %s", filename)
	}
	abs, err := filepath.Abs(expandHome(filename))
	if err != nil {
		return fmt.Errorf("failed to resolve file %s: %w", filename, err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return fmt.Errorf("failed to resolve file %s: %w", filename, err)
	}
	if !pathWithinCwd(resolved) && !pathWithinHome(resolved) && !pathWithinTemp(resolved) {
		return fmt.Errorf("unsafe path: %s resolves outside the current directory, home directory, or temp directory", filename)
	}
	return nil
}

// isSafePath checks if the file path is safe.
// Prevents path traversal attacks. Allows the current directory and its
// subdirectories (relative paths), plus the user's home directory
// (~/... and absolute paths under $HOME) and the system temp directory
// (e.g. /tmp/...).
func isSafePath(filename string) bool {
	// Expand a leading ~ to the user's home directory.
	cleanPath := filepath.Clean(expandHome(filename))

	// Check for path traversal: no component may be "..". A component
	// check (instead of strings.Contains) avoids rejecting legitimate
	// names like "..hidden" that merely contain two dots.
	for _, comp := range strings.Split(cleanPath, string(os.PathSeparator)) {
		if comp == ".." {
			return false
		}
	}

	// Absolute paths are allowed only under the home directory or the
	// system temp directory.
	if filepath.IsAbs(cleanPath) {
		return pathWithinHome(cleanPath) || pathWithinTemp(cleanPath)
	}

	// Relative paths must stay under the current working directory.
	fullPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return false
	}
	return pathWithinCwd(fullPath)
}

// expandHome expands a leading ~ or ~/ to the user's home directory.
// When the home directory cannot be determined the path is returned
// unchanged, so callers fail safe (a stray ~/ path simply does not exist).
func expandHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	switch {
	case path == "~":
		return home
	case strings.HasPrefix(path, "~"+string(os.PathSeparator)):
		return filepath.Join(home, path[2:])
	}
	return path
}

// pathWithinCwd reports whether the given absolute path stays under the
// current working directory.
func pathWithinCwd(absPath string) bool {
	cwd, err := os.Getwd()
	if err != nil {
		return false
	}
	return pathWithinDir(absPath, cwd)
}

// pathWithinHome reports whether the given absolute path stays under the
// user's home directory.
func pathWithinHome(absPath string) bool {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return false
	}
	return pathWithinDir(absPath, home)
}

// pathWithinTemp reports whether the given absolute path stays under the
// system temporary directory. The raw candidate and its symlink-resolved
// form are both checked, so paths work on systems where the temp dir is
// reached through a link (e.g. macOS /var -> /private/var, /tmp ->
// /private/tmp) no matter which spelling the caller uses.
func pathWithinTemp(absPath string) bool {
	for _, dir := range tempDirs() {
		if pathWithinDir(absPath, dir) {
			return true
		}
		if resolved, err := filepath.EvalSymlinks(dir); err == nil && resolved != dir {
			if pathWithinDir(absPath, resolved) {
				return true
			}
		}
	}
	return false
}

// tempDirs returns the system temporary directory plus the conventional
// /tmp on Unix when it differs (e.g. macOS with TMPDIR set reports
// /var/folders/... while callers often pass /tmp/... paths).
func tempDirs() []string {
	seen := map[string]bool{}
	var dirs []string
	add := func(d string) {
		// Clean first: os.TempDir() inherits TMPDIR, which on macOS carries
		// a trailing slash (e.g. /var/folders/.../T/), breaking the
		// dir+"/" prefix match in pathWithinDir.
		d = filepath.Clean(d)
		if d == "" || d == "." || seen[d] {
			return
		}
		seen[d] = true
		dirs = append(dirs, d)
	}
	add(os.TempDir())
	if runtime.GOOS != "windows" {
		add("/tmp")
	}
	return dirs
}

// pathWithinDir reports whether absPath stays under dir. The separator
// guard prevents a sibling directory with a shared prefix (e.g. /proj2 vs
// /proj) from passing.
func pathWithinDir(absPath, dir string) bool {
	if absPath == dir {
		return true
	}
	return strings.HasPrefix(absPath, dir+string(os.PathSeparator))
}
