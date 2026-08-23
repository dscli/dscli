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
