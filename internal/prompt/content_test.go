package prompt

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMessageMarshalJSONPlain(t *testing.T) {
	m := Message{Role: "user", Content: "hello"}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, `"content":"hello"`) {
		t.Fatalf("plain content should serialize as string, got %s", got)
	}
	if strings.Contains(got, "image_url") || strings.Contains(got, "file_id") {
		t.Fatalf("plain content should not contain block fields: %s", got)
	}
}

func TestMessageMarshalJSONBlocks(t *testing.T) {
	m := Message{
		Role:    "user",
		Content: "这张图片里有什么？",
		ContentBlocks: []ContentBlock{
			TextBlock("这张图片里有什么？"),
			FileBlock("file-api-123"),
		},
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, `"type":"file"`) || !strings.Contains(got, `"file_id":"file-api-123"`) {
		t.Fatalf("blocks should serialize as array with file block, got %s", got)
	}
	// 字段顺序固定：type 必须在 text/file_id 之前（保证缓存前缀稳定）
	typeIdx := strings.Index(got, `"type"`)
	textIdx := strings.Index(got, `"text"`)
	fileIdx := strings.Index(got, `"file_id"`)
	if typeIdx == -1 || typeIdx > textIdx || typeIdx > fileIdx {
		t.Fatalf("unexpected field order: %s", got)
	}
}

func TestMessageUnmarshalJSONString(t *testing.T) {
	var m Message
	data := []byte(`{"role":"assistant","content":"你好","reasoning_content":"thinking"}`)
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if m.Content != "你好" || m.Role != "assistant" || m.ReasoningContent != "thinking" || len(m.ContentBlocks) != 0 {
		t.Fatalf("unexpected result: %+v", m)
	}
}

func TestMessageUnmarshalJSONBlocks(t *testing.T) {
	var m Message
	data := []byte(`{"role":"user","content":[{"type":"text","text":"描述图片"},{"type":"file","file_id":"file-api-9"}]}`)
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if len(m.ContentBlocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(m.ContentBlocks))
	}
	if m.ContentBlocks[1].FileID != "file-api-9" {
		t.Fatalf("expected file_id, got %+v", m.ContentBlocks[1])
	}
	// 纯文本提取：text 块拼进 Content
	if m.Content != "描述图片" {
		t.Fatalf("expected plain text extraction, got %q", m.Content)
	}
}

func TestBlocksRoundTrip(t *testing.T) {
	blocks := []ContentBlock{
		TextBlock("分析"),
		{Type: "image_url", ImageURL: &BlockImageURL{URL: "https://example.com/a.png", Detail: "low"}},
	}
	s, err := BlocksToJSON(blocks)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, `"detail":"low"`) {
		t.Fatalf("detail should be kept: %s", s)
	}
	got, err := BlocksFromJSON(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1].ImageURL == nil || got[1].ImageURL.URL != "https://example.com/a.png" {
		t.Fatalf("round trip failed: %+v", got)
	}
}

func TestIsVisionModel(t *testing.T) {
	cases := map[string]bool{
		"deepseek-v4-flash-vision-exp": true,
		"deepseek-v4-flash":            false,
		"deepseek-v4-vision":           true,
		"":                             false,
	}
	for model, want := range cases {
		if got := IsVisionModel(model); got != want {
			t.Errorf("IsVisionModel(%q) = %v, want %v", model, got, want)
		}
	}
}
