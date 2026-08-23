package vision

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/toolcall"
	"github.com/nanjj/clog"
)

//go:embed vision_file_info.md
var infoMd string

func init() {
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "vision_file_info",
		DisplayName: "Vision File Info",
		Description: infoMd,
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_id": map[string]any{
					"type":        "string",
					"description": "file_id returned by vision_file_read (required)",
				},
			},
			"required":             []string{"file_id"},
			"additionalProperties": false,
		},
		Category: "vision",
		Handler:  handleInfo,
	})
}

func handleInfo(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	span, ctx := clog.StartSpanFromContext(ctx, "vision_file_info")
	defer span.Finish()

	fileID := toolcall.ToolArgsValue(args, "file_id", "")
	if fileID == "" {
		return "", "", fmt.Errorf("file_id is required")
	}
	file, err := newFilesAPI().Info(ctx, fileID)
	if err != nil {
		return "", "", err
	}
	data, err := outfmt.JSONMarshal(file)
	if err != nil {
		return "", "", err
	}
	return string(data), "", nil
}
