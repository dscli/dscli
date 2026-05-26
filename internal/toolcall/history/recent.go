package history

import (
	"context"
	_ "embed"
	"time"

	"gitcode.com/dscli/dscli/internal/prompt"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

//go:embed recent.md
var recent_md string

func init() {
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "recent",
		Description: recent_md,
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit": map[string]any{
					"type":        "integer",
					"description": "Max results (default 20, max 20)",
				},
			},
			"additionalProperties": false,
		},
		Category: "history",
		Timeout:  5 * time.Second,
		Handler:  handleRecent,
	})
}

func handleRecent(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	limit := toolcall.ToolArgsValue(args, "limit", 20)
	result, warning, err = prompt.HandleRecent(ctx, limit)
	return result, warning, err
}
