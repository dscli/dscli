package ask

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	ictx "github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/lp"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/prompt"
	"github.com/dscli/dscli/internal/toolcall"
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

// askExpertWithRoleFunc is the function used to call the expert.
// It is a package-level variable so tests can replace it with a mock.
var askExpertWithRoleFunc = askExpertWebChat

func init() {
	// WebChat is always available (free DeepSeek V4 Pro) — no API key needed.
	// The only prerequisite is Chrome installed and logged in once.
	toolcall.RegisterTool(askExpertTool)

	// Test optimization: use mock to skip browser automation.
	if ictx.IsTesting() {
		askExpertWithRoleFunc = func(_ context.Context, _, _ string) (string, error) {
			return "[MOCK]", nil
		}
	}
}

// handleAskExpert handles the ask_expert tool call.
func handleAskExpert(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	summary := toolcall.ToolArgsValue(args, "summary", "")
	content := toolcall.ToolArgsValue(args, "content", "")
	attachments := toolcall.ToolArgsValue(args, "attachments", []string{})

	if content == "" {
		err = fmt.Errorf("content cannot be empty")
		return result, warning, err
	}

	// Show what was asked (truncate long content for display)
	summaryDisplay := summary
	if summaryDisplay == "" {
		summaryDisplay = truncateForDisplay(content, 120)
	}
	outfmt.Println("📞 Consulting expert via DeepSeek Web (free V4 Pro)...")
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

	result, err = askExpertWithRoleFunc(ctx, structuredRequest, "expert")
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

// AskExpert calls the AI expert model via DeepSeek Web (free V4 Pro).
//
// It renders the "expert" system prompt, prepends it to the input, and
// sends the combined message to chat.deepseek.com via Chrome/CDP.
// Each call starts a new conversation.
//
// Parameters:
//
//	ctx: context object for passing execution environment configuration
//	input: input text to send to the AI model, can be any length
//
// Returns:
//
//	reply: the AI model's response text
//	err: error during execution
func AskExpert(ctx context.Context, input string) (reply string, err error) {
	return askExpertWithRoleFunc(ctx, input, "expert")
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
	return askExpertWithRoleFunc(ctx, input, role)
}

// askExpertWebChat is the real implementation: renders the role prompt and
// sends the combined message via lp.WebChat.
func askExpertWebChat(ctx context.Context, input, role string) (reply string, err error) {
	// Render the role-specific system prompt (expert.md / review.md / dev.md).
	// This replaces the API's system message — WebChat has no system prompt
	// concept, so we prepend it to the user message.
	systemPrompt := prompt.RenderPromptForRole(ctx, role)

	// Build the full message: system prompt + separator + user request.
	// The separator helps the web model distinguish the persona instructions
	// from the actual task.
	fullMessage := systemPrompt + "\n\n---\n\n## User Request\n\n" + input

	// Start a new WebChat conversation (free DeepSeek V4 Pro).
	return lp.WebChat(ctx, fullMessage)
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
