// Package history 注册 note 工具，供 LLM 在对话结束时记录笔记
package history

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gitcode.com/dscli/dscli/internal/prompt"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

func init() {
	toolcall.RegisterTool(toolcall.ToolDef{
		Name: "note",
		Description: `Summarize session for future recall.

Record a key summary of the current conversation. Call at the end of a conversation.

Content: 40 characters or less, containing key events and keywords (e.g., "Implemented recall tool with session_id filter").
Auto-truncates content over 40 characters.`,
		Strict: true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"content": map[string]any{
					"type":        "string",
					"description": "对话摘要，40字以内，包含关键事件和关键词",
				},
			},
			"required":             []string{"content"},
			"additionalProperties": false,
		},
		Category: "history",
		Timeout:  5 * time.Second,
		Handler:  handleNote,
	})
}

func handleNote(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	content := toolcall.ToolArgsValue(args, "content", "")
	content = strings.TrimSpace(content)
	if content == "" {
		err = fmt.Errorf("笔记内容不能为空")
		return result, warning, err
	}
	result, warning, err = prompt.HandleNote(ctx, content)
	return result, warning, err
}