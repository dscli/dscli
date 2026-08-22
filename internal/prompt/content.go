package prompt

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ContentBlock 是 OpenAI 兼容的 content 块类型，用于图片输入。
// 字段顺序固定（json tag 顺序即序列化顺序），保证同一消息的
// JSON 表示稳定，不影响 DeepSeek 上下文缓存（KV cache）命中。
type ContentBlock struct {
	Type     string         `json:"type"`               // text | image_url | file
	Text     string         `json:"text,omitzero"`      // type=text
	ImageURL *BlockImageURL `json:"image_url,omitzero"` // type=image_url
	FileID   string         `json:"file_id,omitzero"`   // type=file
}

// BlockImageURL 是 image_url 块的 url 字段（可选 detail）。
type BlockImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitzero"` // low | high | original | auto
}

// FileBlock 构建一个引用 Files API file_id 的内容块。
func FileBlock(fileID string) ContentBlock {
	return ContentBlock{Type: "file", FileID: fileID}
}

// TextBlock 构建一个文本内容块。
func TextBlock(text string) ContentBlock {
	return ContentBlock{Type: "text", Text: text}
}

// BlocksToJSON 将内容块序列化为 JSON 数组字符串（用于 sqlite content_blocks 列）。
func BlocksToJSON(blocks []ContentBlock) (string, error) {
	data, err := json.Marshal(blocks)
	if err != nil {
		return "", fmt.Errorf("序列化 content blocks 失败: %w", err)
	}
	return string(data), nil
}

// BlocksFromJSON 解析 content_blocks 列中的 JSON 数组。
func BlocksFromJSON(data string) ([]ContentBlock, error) {
	var blocks []ContentBlock
	if err := json.Unmarshal([]byte(data), &blocks); err != nil {
		return nil, fmt.Errorf("解析 content blocks 失败: %w", err)
	}
	return blocks, nil
}

// IsVisionModel 判断模型是否支持图片输入。
// DeepSeek 视觉模型名包含 "vision"（如 deepseek-v4-flash-vision-exp）。
func IsVisionModel(model string) bool {
	return strings.Contains(model, "vision")
}

// VisionFileUploadTool 是文件上传工具名（模型自主上传图片的入口）。
const VisionFileUploadTool = "vision_file_upload"

// extractUploadedFileID 从 tool 消息的 content 中提取上传结果里的 file_id。
// content 是工具框架拼装的文本，形如
// "Tool result 1 (vision_file_upload):\n### Result\n{json}\n"，
// 因此跳过 ### Result 段前缀后取第一个 JSON 对象解析。
func extractUploadedFileID(content string) string {
	rest := content
	if i := strings.Index(content, "### Result\n"); i >= 0 {
		rest = content[i+len("### Result\n"):]
	}
	idx := strings.Index(rest, "{")
	if idx == -1 {
		return ""
	}
	var res struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(rest[idx:]), &res); err != nil {
		return ""
	}
	if !strings.HasPrefix(res.ID, "file-api-") {
		return ""
	}
	return res.ID
}

// BuildUploadInjection 检测模型本轮调用的 vision_file_upload 工具结果，
// 构造一条新的 user 消息把 file_id 以 file 块形式注入，使视觉模型
// 能在下一轮真正"看到"图片（OpenAI 协议中模型无法自己插入 user 消息，
// 工具结果只作为 tool 消息存在，图片仅允许出现在 user 消息）。
// 非视觉模型返回 nil（上传的文件仍可管理，但不注入引用）。
func BuildUploadInjection(model string, tcs []ToolCall, toolInputs []Message) *Message {
	if !IsVisionModel(model) {
		return nil
	}
	var ids []string
	for _, tc := range tcs {
		if tc.Function.Name != VisionFileUploadTool {
			continue
		}
		for _, m := range toolInputs {
			if m.Role != "tool" || m.ToolCallID != tc.ID {
				continue
			}
			if id := extractUploadedFileID(m.Content); id != "" {
				ids = append(ids, id)
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	text := "图片已通过 vision_file_upload 上传，请结合图片内容继续回答。"
	blocks := make([]ContentBlock, 0, len(ids)+1)
	blocks = append(blocks, TextBlock(text))
	for _, id := range ids {
		blocks = append(blocks, FileBlock(id))
	}
	return &Message{Role: "user", Content: text, ContentBlocks: blocks}
}
