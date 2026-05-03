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
		Description: `记录当前对话的关键摘要，供未来对话回忆使用。应在对话结束时调用。

内容要求：40字以内，包含关键事件和关键词（如"实现recall工具，添加session_id过滤"）。
自动截断超过40字的内容。`,
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
		Category: "memory",
		Timeout:  5 * time.Second,
		Handler:  handleNote,
	})
}

func handleNote(ctx context.Context, args toolcall.ToolArgs) (result string, suggestion string, err error) {
	content := toolcall.ToolArgsValue(args, "content", "")
	content = strings.TrimSpace(content)
	if content == "" {
		err = fmt.Errorf("笔记内容不能为空")
		return
	}

	// 警告超过 40 字（实际 SaveNote 也会截断）
	if len([]rune(content)) > 40 {
		suggestion = "笔记超过40字已自动截断。下次请控制在40字以内。"
	}

	if saveErr := prompt.SaveNote(ctx, content); saveErr != nil {
		err = saveErr
		return
	}

	result = "笔记已保存。"
	return
}
