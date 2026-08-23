package toolcall

import (
	"strings"
	"testing"

	"github.com/dscli/dscli/internal/prompt"
)

func TestSplitDualResultNonDual(t *testing.T) {
	// 普通字符串结果：原样返回
	r, m, ok := SplitDualResult("hello")
	if ok || r != "hello" || m != nil {
		t.Fatalf("plain text should not be dual: ok=%v r=%q m=%v", ok, r, m)
	}
	// 普通 JSON 结果（如文件对象）：JSON 可解析但 Dual=false → 不拆分
	r, m, ok = SplitDualResult(`{"id":"file-api-1","object":"file"}`)
	if ok || !strings.Contains(r, "file-api-1") || m != nil {
		t.Fatalf("plain json should not be dual: ok=%v r=%q", ok, r)
	}
	// 空结果
	r, m, ok = SplitDualResult("")
	if ok || r != "" || m != nil {
		t.Fatalf("empty should not be dual: ok=%v", ok)
	}
	// 非 JSON 文本
	r, m, ok = SplitDualResult("Tool result")
	if ok {
		t.Fatalf("text should not be dual: %q", r)
	}
}

func TestSplitDualResultDual(t *testing.T) {
	msg := &prompt.Message{
		Content: "图片已加载（file_id: file-api-9），请结合图片内容继续回答。",
		ContentBlocks: []prompt.ContentBlock{
			prompt.TextBlock("图片已加载"),
			prompt.FileBlock("file-api-9"),
		},
	}
	dual, err := MarshalDual(NewDual(`{"id":"file-api-9","object":"file"}`, msg))
	if err != nil {
		t.Fatal(err)
	}
	r, m, ok := SplitDualResult(dual)
	if !ok {
		t.Fatalf("dual should split: %s", dual)
	}
	if !strings.Contains(r, "file-api-9") {
		t.Fatalf("tool result lost: %q", r)
	}
	if m == nil || m.Role != "user" || len(m.ContentBlocks) != 2 {
		t.Fatalf("user message unexpected: %+v", m)
	}
	if m.ContentBlocks[1].FileID != "file-api-9" {
		t.Fatalf("file block lost: %+v", m.ContentBlocks[1])
	}
}

// TestSplitDualRejectsForeignJSON 普通 JSON 结果即使含 dual 字段但缺
// user 消息 role，不得误拆（三重确认的保护）。
func TestSplitDualRejectsForeignJSON(t *testing.T) {
	// 巧合结构：dual=true 但 UserMessage 为 nil
	r, m, ok := SplitDualResult(`{"dual":true,"tool_result":"x"}`)
	if ok || !strings.Contains(r, "dual") || m != nil {
		t.Fatalf("dual without user message must not split: %q %+v %v", r, m, ok)
	}
	// UserMessage 存在但 role 非 user（如 assistant）→ 不拆
	dual, err := MarshalDual(DualMessage{
		Dual:        true,
		ToolResult:  "meta",
		UserMessage: &prompt.Message{Role: "assistant", Content: "x"},
	})
	if err != nil {
		t.Fatal(err)
	}
	r, m, ok = SplitDualResult(dual)
	if ok || !strings.Contains(r, "meta") || m != nil {
		t.Fatalf("non-user message must not split: %q %+v %v", r, m, ok)
	}
}

func TestDualRoundTrip(t *testing.T) {
	// MarshalDual → SplitDualResult 往返一致
	dual, err := MarshalDual(NewDual("meta", &prompt.Message{Content: "hi"}))
	if err != nil {
		t.Fatal(err)
	}
	r, m, ok := SplitDualResult(dual)
	if !ok || r != "meta" || m == nil || m.Role != "user" || m.Content != "hi" {
		t.Fatalf("round trip failed: %q %+v %v", r, m, ok)
	}
}
