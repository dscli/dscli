package ask

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"gitcode.com/dscli/dscli/internal/editor"
	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

//go:embed ask_user.md
var ask_user_md string

// askUserTool tool definition
var askUserTool = toolcall.ToolDef{
	Name:        "ask_user",
	DisplayName: "Ask User",
	Description: ask_user_md,
	Strict:      true,
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"content": map[string]any{
				"type":        "string",
				"description": "Content to ask about",
			},
			"timeout": map[string]any{
				"type":        "integer",
				"description": "Timeout in seconds (default 3600). Set shorter (e.g. 300) for simple confirmations.",
			},
		},
		"required":             []string{"content"},
		"additionalProperties": false,
	},
	Category: "communication",
	Timeout:  1 * time.Hour, // 1 hour for user to respond
	Handler:  handleAskUser,
}

func init() {
	toolcall.RegisterTool(askUserTool)
}

// handleAskUser handles the ask_user tool call.
func handleAskUser(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	content := toolcall.ToolArgsValue(args, "content", "")
	if content == "" {
		err = fmt.Errorf("content cannot be empty")
		return result, warning, err
	}

	// Log consultation
	outfmt.Println("📞 Consulting user...")

	// Generate question summary (avoid excessively long output)
	summary := []rune(content)
	if len(summary) > 100 {
		summary = append(summary[:97], []rune("...")...)
	}
	outfmt.Println("  Question summary:", string(summary))

	result, err = editor.OpenEditor(ctx, content)
	if err != nil {
		outfmt.Println("❌ Failed to get user response")
		err = fmt.Errorf("failed to get user response: %v", err)
		return result, warning, err
	}

	// Show user response summary
	if result != "" {
		replySummary := []rune(result)
		if len(replySummary) > 100 {
			replySummary = append(replySummary[:97], []rune("...")...)
		}
		outfmt.Println("  User response summary:", string(replySummary))
	}

	outfmt.Println("✅ User consultation completed")
	return result, warning, err
}