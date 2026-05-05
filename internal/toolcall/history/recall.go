// Package history registers the recall tool for LLM-driven history search.
package history

import (
	"context"
	"fmt"
	"time"

	"gitcode.com/dscli/dscli/internal/prompt"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

func init() {
	toolcall.RegisterTool(toolcall.ToolDef{
		Name: "recall",
		Description: `Recall conversation history by keywords.

Search historical messages in the current project. Multiple keywords (space-separated, OR logic).
Only searches user messages and assistant summaries, limited to current project.

Parameters:
- keywords: search keywords, space-separated, OR logic (required)
- days: search N recent days (default 30)
- limit: max results (default 5)`,
		Strict: true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"keywords": map[string]any{
					"type":        "string",
					"description": "搜索关键词，空格分隔，OR逻辑（匹配任一即返回）",
				},
				"days": map[string]any{
					"type":        "integer",
					"description": "搜索最近N天的消息（默认30）",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "返回结果数量上限（默认5）",
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