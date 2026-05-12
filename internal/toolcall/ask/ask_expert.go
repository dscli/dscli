package ask

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/shell"
	"gitcode.com/dscli/dscli/internal/toolcall"
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
			"summary": map[string]any{
				"type":        "string",
				"description": "Brief summary (optional)",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Detailed question (required)",
			},
			"attachments": map[string]any{
				"type":        "array",
				"description": "File attachments list (optional)",
				"items": map[string]string{
					"type":        "string",
					"description": "Attachment filename",
				},
			},
			"timeout": map[string]any{
				"type":        "integer",
				"description": "Timeout in seconds (default 600). Set longer for complex questions requiring deep analysis.",
			},
		},
		"required":             []string{"content"},
		"additionalProperties": false,
	},
	Category: "communication",
	Timeout:  10 * time.Minute, // 10 minutes for expert to respond
	Handler:  handleAskExpert,
}

// dscliCmd is the command string to invoke dscli.
// In test environments it is automatically replaced with echo to avoid
// triggering ~60s AI reasoning model calls on every test.
var dscliCmd = "dscli"

func init() {
	if context.ReasonerModelOK() {
		toolcall.RegisterTool(askExpertTool)
	}

	// Test optimization: use echo instead of dscli to skip expensive AI inference.
	// context.IsTesting() checks whether os.Args[0] ends with ".test".
	if context.IsTesting() {
		dscliCmd = "echo '[MOCK]'"
	}
}

// handleAskExpert handles the ask_expert tool call.
func handleAskExpert(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	// Backward compatibility: support old parameter names
	summary := toolcall.ToolArgsValue(args, "summary", "")
	content := toolcall.ToolArgsValue(args, "content", "")
	attachments := toolcall.ToolArgsValue(args, "attachments", []string{})

	// If content is empty, try the old parameter name
	if content == "" {
		content = toolcall.ToolArgsValue(args, "question", "")
	}

	if content == "" {
		err = fmt.Errorf("content cannot be empty")
		return result, warning, err
	}

	// Show what was asked (truncate long content for display)
	summaryDisplay := summary
	if summaryDisplay == "" {
		summaryDisplay = truncateForDisplay(content, 120)
	}
	outfmt.Println("📞 Consulting expert...")
	outfmt.Println("  Question:", summaryDisplay)

	// Build structured request (does not ask expert to generate summary)
	structuredRequest, attachmentErrors := buildStructuredRequest(summary, content, attachments)

	// Report attachment errors to user but continue execution
	if len(attachmentErrors) > 0 {
		outfmt.Println("⚠️  Attachment warnings:")
		for _, attachmentErr := range attachmentErrors {
			outfmt.Printf("  - %v\n", attachmentErr)
		}
	}

	result, err = AskExpert(ctx, structuredRequest)
	if err != nil {
		outfmt.Println("❌ Expert consultation failed")
		return result, warning, err
	}

	// Trim leading/trailing whitespace from expert response
	result = strings.TrimSpace(result)

	outfmt.Printf("✅ Expert consultation completed\n\n%s\n", result)
	return result, warning, err
}

// truncateForDisplay truncates s to maxLen runes for terminal display.
func truncateForDisplay(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-3]) + "..."
}

// AskExpert calls the AI expert model for consultation and returns the reply.
//
// This function executes the dscli chat command via internal/shell, writes
// the input content to a temporary file, and passes it to dscli via the
// --input flag, avoiding command-line length limits and stdin issues.
//
// Parameters:
//
//	ctx: context object for passing execution environment configuration
//	input: input text to send to the AI model, can be any length
//	       (limited by system memory)
//
// Returns:
//
//	reply: the AI model's response text. Returns empty string if execution
//	       fails without obtaining a reply.
//	err: error during execution. Returns nil on success. Common errors include:
//	     - dscli command execution failure
//	     - temporary file creation/write failure
//
// Details:
//
//	The function invokes the AI model by executing the following command:
//	     dscli chat --no-color --no-timestamp --histsize 0 --model <model_name> --input <temp_file>
//	where the model name is specified by ModelDeepseekReasoner.
//
// Notes:
//   - Ensure the dscli CLI tool is properly installed and configured.
//   - Input content is passed via a temporary file that is automatically
//     cleaned up after execution.
//
// Example:
//
//	ctx := context.Background()
//	reply, err := AskExpert(ctx, "Please analyze the quality of this code")
//	if err != nil {
//	    log.Printf("Consultation failed: %v", err)
//	} else {
//	    fmt.Println(reply)
//	}
//
// See also:
//   - shell.SimpleExecute: function for executing shell commands
//   - handleAskExpert: tool handler function that uses this function
func AskExpert(ctx context.Context, input string) (reply string, err error) {
	return AskExpertWithRole(ctx, input, "expert")
}

// AskExpertWithRole calls the AI model for consultation with a specified
// role (dev/expert/review).
//
// This function executes the dscli chat command via internal/shell, writes
// the input content to a temporary file, and passes it to dscli via the
// --input and --role flags.
//
// Parameters:
//
//	ctx: context object
//	input: input text to send to the AI model
//	role: role (dev/expert/review)
//
// Returns:
//
//	reply: the AI model's response text
//	err: error during execution
func AskExpertWithRole(ctx context.Context, input, role string) (reply string, err error) {
	// Write input content to a temporary file to avoid shell command
	// length limits and stdin passing issues.
	tmpFile, err := os.CreateTemp("", "dscli-ask-*.md")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.WriteString(input); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("failed to close temp file: %w", err)
	}

	script := fmt.Sprintf(`%s chat --no-color --no-timestamp --histsize 0 --model %s --role %s --input %s`, dscliCmd,
		context.ModelDeepseekReasoner, role, tmpPath)
	reply, err = shell.SimpleExecute(ctx, script)
	return reply, err
}

// maxAttachmentSize is the maximum allowed size for a single attachment (1MB).
const maxAttachmentSize = 1 << 20

// buildStructuredRequest builds a structured request for the expert.
func buildStructuredRequest(userSummary, originalContent string, attachments []string) (string, []error) {
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

			// Check file size (limit to 1MB)
			if info, err := os.Stat(filename); err == nil && info.Size() > maxAttachmentSize {
				errors = append(errors, fmt.Errorf("file too large: %s (%d bytes > 1MB)", filename, info.Size()))
				continue
			}

			b, err := os.ReadFile(filename)
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

	request := `Please answer the following question in a structured format.

`
	if userSummary != "" {
		request += `
## Background
` + userSummary + `

## Detailed Question
` + originalContent + attachmentSection
	} else {
		request += originalContent + attachmentSection
	}
	request += `

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

// isSafePath checks if the file path is safe.
// Prevents path traversal attacks, only allows current directory and subdirectories.
func isSafePath(filename string) bool {
	// Clean path
	cleanPath := filepath.Clean(filename)

	// Check for path traversal
	if strings.Contains(cleanPath, "..") {
		return false
	}

	// Check if absolute path
	if filepath.IsAbs(cleanPath) {
		return false
	}

	// Check if under current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return false
	}

	fullPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return false
	}

	return strings.HasPrefix(fullPath, cwd)
}