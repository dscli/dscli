// Package history registers the recall tool for LLM-driven history search.
package history

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gitcode.com/dscli/dscli/internal/history"
	"gitcode.com/dscli/dscli/internal/prompt"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

func init() {
	toolcall.RegisterTool(toolcall.ToolDef{
		Name: "recall",
		Description: `搜索当前项目的历史消息，支持多关键词（空格分隔，OR逻辑）。

仅搜索 user 消息和助手总结（无工具调用的 assistant 消息），限定当前项目。

参数说明：
- keywords: 搜索关键词，空格分隔，OR逻辑（匹配任一即返回），必需
- days: 搜索最近N天的消息，默认30，可选
- limit: 返回结果数量上限，默认5，可选

返回结果包含时间、角色和内容。`,
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
		Category: "memory",
		Timeout:  10 * time.Second,
		Handler:  handleRecall,
	})
}

func handleRecall(ctx context.Context, args toolcall.ToolArgs) (result string, suggestion string, err error) {
	keywordsStr := toolcall.ToolArgsValue(args, "keywords", "")
	if keywordsStr == "" {
		err = fmt.Errorf("参数 'keywords' 缺失")
		return
	}

	days := toolcall.ToolArgsValue(args, "days", 30)
	limit := toolcall.ToolArgsValue(args, "limit", 5)

	// 按空格拆分关键词
	var keywords []string
	for _, kw := range strings.Fields(keywordsStr) {
		kw = strings.TrimSpace(kw)
		if kw != "" {
			keywords = append(keywords, kw)
		}
	}

	if len(keywords) == 0 {
		err = fmt.Errorf("没有有效的搜索关键词")
		return
	}

	results, searchErr := history.SearchMessages(ctx, keywords, days, limit)
	if searchErr != nil {
		err = searchErr
		return
	}

	if len(results) == 0 {
		result = "没有找到匹配的历史消息。"
		return
	}

	// 格式化结果
	var b strings.Builder
	b.WriteString(fmt.Sprintf("找到 **%d** 条相关历史消息：\n\n", len(results)))
	for i, r := range results {
		roleLabel := "🙋 用户"
		if r.Message.Role == "assistant" {
			roleLabel = "🤖 助手"
		}
		timeStr := prompt.FormatTime(r.Message.CreatedAt)

		b.WriteString(fmt.Sprintf("%d. %s %s %s\n", i+1, timeStr, roleLabel, r.Message.Content))
	}

	result = b.String()
	return
}
