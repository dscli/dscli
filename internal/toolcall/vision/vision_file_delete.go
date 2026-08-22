package vision

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/toolcall"
	"github.com/nanjj/clog"
)

//go:embed vision_file_delete.md
var deleteMd string

func init() {
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "vision_file_delete",
		DisplayName: "Vision File Delete",
		Description: deleteMd,
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_id": map[string]any{
					"type":        "string",
					"description": "file_id returned by vision_file_upload (required)",
				},
			},
			"required":             []string{"file_id"},
			"additionalProperties": false,
		},
		Category: "vision",
		Handler:  handleDelete,
	})
}

func handleDelete(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	span, ctx := clog.StartSpanFromContext(ctx, "vision_file_delete")
	defer span.Finish()

	fileID := toolcall.ToolArgsValue(args, "file_id", "")
	if fileID == "" {
		return "", "", fmt.Errorf("file_id is required")
	}
	res, err := newFilesAPI().Delete(ctx, fileID)
	if err != nil {
		return "", "", err
	}
	data, err := outfmt.JSONMarshal(res)
	if err != nil {
		return "", "", err
	}
	return string(data), "", nil
}
