package ask

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	ictx "github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/lp"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/prompt"
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
				"description": "The question to ask. If the value starts with @ and points to an existing file (e.g. @question.txt or @~/notes/q.txt), the file content is read from disk as the question.",
			},
			"attachments": map[string]any{
				"type":        "array",
				"description": "File attachments list (optional). Images are uploaded and analyzed visually; other files inlined as text (1MB max, safe paths = cwd relative or ~/... and $HOME absolute, ≤50 files/≤100MB).",
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
				"description": "Continue a previous conversation (default empty = new). \"last\" continues the most recent one; a conversation ID from a previous result's conversation_id line continues that one; a full chat.deepseek.com URL (copied from a browser) also works. \"list\" lists all saved conversations instead of asking.",
			},
			"timeout": map[string]any{
				"type":        "integer",
				"description": "Timeout in seconds (default 600). Set longer for complex questions requiring deep analysis.",
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
// uploaded to the web chat. It returns the reply text and the conversation
// URL (empty when unknown) so callers can continue the conversation later.
var askExpertWithRoleFunc = askExpertWebChat

func init() {
	// WebChat is always available (free DeepSeek V4 Pro) — no API key needed.
	// The only prerequisite is Chrome installed and logged in once.
	toolcall.RegisterTool(askExpertTool)

	// Test optimization: use mock to skip browser automation.
	if ictx.IsTesting() {
		askExpertWithRoleFunc = func(_ context.Context, _, _, _, _, _ string, _ []string) (string, string, error) {
			return "[MOCK]", "", nil
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

	// keep="list" is a query, not a message: return the saved conversation
	// registry so the caller can pick an ID to continue.
	if keep == "list" {
		return listConversations()
	}

	// Split attachments by type: image files are uploaded to the web chat
	// (flash/vision modes), everything else is inlined as text. Uploaded
	// paths are still sandboxed to the current directory (symlink-resolved)
	// so the LLM cannot exfiltrate arbitrary local files.
	var uploads, inline []string
	var attachmentErrors []error
	for _, a := range attachments {
		if lp.IsImageFile(a) {
			if err := verifySafePath(a); err != nil {
				attachmentErrors = append(attachmentErrors, err)
				continue
			}
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

	// Build the structured request with inlined text attachments.
	structuredRequest, inlineErrors := buildStructuredRequest(content, inline)
	attachmentErrors = append(attachmentErrors, inlineErrors...)

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
	result, convURL, err := askExpertWithRoleFunc(ctx, structuredRequest, "", "", mode, keep, uploads)
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

	outfmt.Printf("✅ Expert consultation completed\n\n%s\n", result)
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
	reply, _, err = askExpertWithRoleFunc(ctx, input, role, "", "", "", nil)
	return reply, err
}

// askExpertWebChat is the real implementation: renders the system prompt
// (either the raw system text or the role template) and sends the combined
// message via lp.WebChatWithOptions. When both role and system are empty,
// no persona is injected and the input is sent verbatim (the ask_expert
// tool relies on the caller's own context; code_review passes a role).
// keep continues a previous conversation; it is passed through to
// lp.WebChatOptions.Keep ("" = new, "last", ID, or URL).
func askExpertWebChat(ctx context.Context, input, role, system, mode, keep string, attachments []string) (reply, convURL string, err error) {
	// WebChat has no system prompt concept, so we prepend it to the user
	// message. The separator helps the web model distinguish the persona
	// instructions from the actual task.
	fullMessage := input
	if system != "" {
		fullMessage = system + "\n\n---\n\n## User Request\n\n" + input
	} else if role != "" {
		// Render the role-specific template (expert.md / review.md / dev.md
		// or a custom override via the prompt override chain).
		fullMessage = prompt.RenderPromptForRole(ctx, role) + "\n\n---\n\n## User Request\n\n" + input
	}

	// Start a new WebChat conversation (keep="" — the default) or continue
	// a saved one. Image attachments are uploaded as real files; an empty
	// mode auto-selects vision (with uploads) or pro.
	res, err := lp.WebChatWithOptions(ctx, fullMessage, lp.WebChatOptions{
		Mode:        lp.Mode(mode),
		Attachments: attachments,
		Keep:        keep,
	})
	if err != nil {
		return "", "", err
	}
	return res.Text, res.URL, nil
}

// maxAttachmentSize is the maximum allowed size for a single attachment (1MB).
const maxAttachmentSize = 1 << 20

// buildStructuredRequest builds a structured request for the expert.
func buildStructuredRequest(content string, attachments []string) (string, []error) {
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
// (current directory and subdirectories, or the user's home directory via
// ~/... or absolute paths under $HOME, no ".." traversal), 1MB size limit,
// non-empty content. Errors are explicit, never silent.
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

// verifySafePath checks that filename is safe to read or upload: relative
// inside the current directory, or under the user's home directory (either
// ~/... or an absolute path under $HOME). isSafePath only checks the
// textual path, so a symlink inside a sandbox could otherwise smuggle in
// arbitrary files from outside it; EvalSymlinks resolves the real target
// (relative paths stay relative, so absolutize first).
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
	if !pathWithinCwd(resolved) && !pathWithinHome(resolved) {
		return fmt.Errorf("unsafe path: %s resolves outside the current directory and home directory", filename)
	}
	return nil
}

// isSafePath checks if the file path is safe.
// Prevents path traversal attacks. Allows the current directory and its
// subdirectories (relative paths), plus the user's home directory
// (~/... and absolute paths under $HOME).
func isSafePath(filename string) bool {
	// Expand a leading ~ to the user's home directory.
	cleanPath := filepath.Clean(expandHome(filename))

	// Check for path traversal
	if strings.Contains(cleanPath, "..") {
		return false
	}

	// Absolute paths are allowed only under the home directory.
	if filepath.IsAbs(cleanPath) {
		return pathWithinHome(cleanPath)
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

// pathWithinDir reports whether absPath stays under dir. The separator
// guard prevents a sibling directory with a shared prefix (e.g. /proj2 vs
// /proj) from passing.
func pathWithinDir(absPath, dir string) bool {
	if absPath == dir {
		return true
	}
	return strings.HasPrefix(absPath, dir+string(os.PathSeparator))
}
