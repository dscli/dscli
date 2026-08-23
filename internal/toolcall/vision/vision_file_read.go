package vision

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	dcontext "github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/dsc"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/prompt"
	"github.com/dscli/dscli/internal/toolcall"
	"github.com/nanjj/clog"
)

//go:embed vision_file_read.md
var readMd string

func init() {
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "vision_file_read",
		DisplayName: "Vision File Read",
		Description: readMd,
		// 旧名兼容：历史记录/模型缓存中可能仍以 vision_file_upload 调用
		//（中断恢复重放）；别名可解析但不出现在新工具列表。
		Aliases: []string{"vision_file_upload"},
		Strict:  true,
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
		Timeout:  600 * time.Second, // API 允许上传最长 10 分钟
		Handler:  handleRead,
	})
}

// handleRead 读取（上传）本地图片并返回文件对象 JSON（含 file_id）。
//
// 视觉模型下返回 DualMessage 双消息：tool 消息保留文件元数据（file_id
// 等，list/info/delete 依赖），附加 user 消息携带 file 块注入对话——
// Handler 框架（toolcall.HandleToolCalls）自动拆分，模型下一轮请求即可
// "看到"图片，无需等下一个 user turn。非视觉模型只返回元数据 JSON
// （OpenAI 协议中图片块仅允许出现在 user 消息，且非视觉模型会 400）。
func handleRead(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	span, ctx := clog.StartSpanFromContext(ctx, "vision_file_read")
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

	// 视觉模型：注入带 file 块的 user 消息（双消息协议）。
	model := dcontext.ContextValue(ctx, dcontext.CurrentModelNameKey, "")
	if prompt.IsVisionModel(model) {
		text := "图片已加载（file_id: " + file.ID + "），请结合图片内容继续回答。"
		msg := &prompt.Message{
			Content: text,
			ContentBlocks: []prompt.ContentBlock{
				prompt.TextBlock(text),
				prompt.FileBlock(file.ID),
			},
		}
		dual, marshalErr := toolcall.MarshalDual(toolcall.NewDual(string(data), msg))
		if marshalErr != nil {
			return "", "", marshalErr
		}
		return dual, "", nil
	}
	return string(data), "", nil
}
