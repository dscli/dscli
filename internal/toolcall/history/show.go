package history

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/dscli/dscli/internal/prompt"
	"github.com/dscli/dscli/internal/toolcall"
)

//go:embed show.md
var show_md string

func init() {
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "show",
		Description: show_md,
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{
					"type":        "integer",
					"description": "Message ID (required)",
				},
			},
			"required":             []string{"id"},
			"additionalProperties": false,
		},
		Category: "history",
		Timeout:  5 * time.Second,
		Handler:  handleShow,
	})
}

func handleShow(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	id := toolcall.ToolArgsValue(args, "id", int64(0))
	if id <= 0 {
		err = fmt.Errorf("参数 'id' 缺失或无效")
		return result, warning, err
	}
	result, warning, err = prompt.HandleShow(ctx, id)
	return result, warning, err
}
