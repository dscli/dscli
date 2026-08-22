package vision

import (
	"context"
	_ "embed"

	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/toolcall"
	"github.com/nanjj/clog"
)

//go:embed vision_file_list.md
var listMd string

func init() {
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "vision_file_list",
		DisplayName: "Vision File List",
		Description: listMd,
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"after": map[string]any{
					"type":        "string",
					"description": "Pagination cursor: return files after this file_id",
				},
				"limit": map[string]any{
					"type":        "integer",
					"minimum":     1,
					"maximum":     1000,
					"description": "Number of files to return (default 1000)",
				},
				"order": map[string]any{
					"type":        "string",
					"enum":        []string{"asc", "desc"},
					"description": "Sort by creation time: asc (default) or desc",
				},
			},
			"additionalProperties": false,
		},
		Category: "vision",
		Handler:  handleList,
	})
}

func handleList(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	span, ctx := clog.StartSpanFromContext(ctx, "vision_file_list")
	defer span.Finish()

	after := toolcall.ToolArgsValue(args, "after", "")
	limit := int(toolcall.ToolArgsValue(args, "limit", int64(0)))
	order := toolcall.ToolArgsValue(args, "order", "")

	list, err := newFilesAPI().List(ctx, after, limit, order, "")
	if err != nil {
		return "", "", err
	}
	data, err := outfmt.JSONMarshal(list)
	if err != nil {
		return "", "", err
	}
	return string(data), "", nil
}
