// Package history 注册 note 工具，供 LLM 在对话结束时记录笔记
package history

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"time"

	"github.com/dscli/dscli/internal/prompt"
	"github.com/dscli/dscli/internal/toolcall"
)

//go:embed note.md
var note_md string

func init() {
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "note",
		Description: note_md,
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"content": map[string]any{
					"type":        "string",
					"description": fmt.Sprintf("Summary content, max %d chars, with key events and keywords", prompt.MaxNoteContentLen),
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
