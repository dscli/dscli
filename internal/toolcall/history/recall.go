// Package history registers the recall tool for LLM-driven history search.
package history

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"gitcode.com/dscli/dscli/internal/prompt"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

//go:embed recall.md
var recall_md string
func init() {
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "recall",
		Description: recall_md,
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"keywords": map[string]any{
					"type":        "string",
					"description": "Search keywords, space-separated, OR logic (matches any)",
				},
				"days": map[string]any{
					"type":        "integer",
					"description": "Search N recent days (default 30)",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Max results (default 5)",
				},
			},
			"required":             []string{"keywords"},
			"additionalProperties": false,
		},
		Category: "history",
		Timeout:  10 * time.Second,
		Handler:  handleRecall,
	})
}

func handleRecall(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	keywordsStr := toolcall.ToolArgsValue(args, "keywords", "")
	if keywordsStr == "" {
		err = fmt.Errorf("参数 'keywords' 缺失")
		return result, warning, err
	}

	days := toolcall.ToolArgsValue(args, "days", 30)
	limit := toolcall.ToolArgsValue(args, "limit", 5)
	result, warning, err = prompt.HandleRecall(ctx, keywordsStr, days, limit)
	return result, warning, err
}