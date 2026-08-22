package vision

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/dscli/dscli/internal/dsc"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/toolcall"
	"github.com/nanjj/clog"
)

//go:embed vision_file_upload.md
var uploadMd string

func init() {
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "vision_file_upload",
		DisplayName: "Vision File Upload",
		Description: uploadMd,
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file": map[string]any{
					"type":        "string",
					"description": "Local image file path (JPEG/PNG/GIF/WebP, max 64 MiB)",
				},
				"expires_seconds": map[string]any{
					"type":        "integer",
					"minimum":     3600,
					"maximum":     2592000,
					"description": "Validity in seconds (3600-2592000). Omit for permanent storage.",
				},
			},
			"required":             []string{"file"},
			"additionalProperties": false,
		},
		Category: "vision",
		Timeout:  300 * time.Second, // 大文件上传最长 10 分钟，给足余量
		Handler:  handleUpload,
	})
}

// handleUpload 上传本地图片并返回文件对象 JSON（含 file_id）。
func handleUpload(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	span, ctx := clog.StartSpanFromContext(ctx, "vision_file_upload")
	defer span.Finish()

	filePath := toolcall.ToolArgsValue(args, "file", "")
	if filePath == "" {
		return "", "", fmt.Errorf("file is required")
	}
	expires := toolcall.ToolArgsValue(args, "expires_seconds", int64(0))

	file, err := newFilesAPI().Upload(ctx, filePath, dsc.UploadOptions{ExpiresSeconds: expires})
	if err != nil {
		return "", "", err
	}
	data, err := outfmt.JSONMarshal(file)
	if err != nil {
		return "", "", err
	}
	return string(data), "", nil
}
