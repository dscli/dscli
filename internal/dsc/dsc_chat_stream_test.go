package dsc

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/prompt"
)

// newSSETestServer 返回一个模拟 DeepSeek SSE 流端点的测试服务器。
// chunks 为 JSON 数据块（不含 data: 前缀），按 SSE 格式以空行分隔，
// 结尾统一补 [DONE]。
func newSSETestServer(t *testing.T, chunks ...string) *httptest.Server {
	t.Helper()
	var body strings.Builder
	for _, c := range chunks {
		body.WriteString("data: ")
		body.WriteString(c)
		body.WriteString("\n\n")
	}
	body.WriteString("data: [DONE]\n\n")
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body.String()))
	}))
}

// TestChatStreamExtractsID 验证 chatStream 从 SSE chunk 提取真实响应 ID
// （而非时间戳占位），供上层落库为 ConversationID。
func TestChatStreamExtractsID(t *testing.T) {
	const streamID = "8f2a1b3c-4d5e-6f70-8a9b-0c1d2e3f4a5b"
	server := newSSETestServer(
		t,
		`{"id":"`+streamID+`","object":"chat.completion.chunk","choices":[{"delta":{"content":"Hel"}}]}`,
		`{"id":"`+streamID+`","object":"chat.completion.chunk","choices":[{"delta":{"content":"lo"}}]}`,
	)
	defer server.Close()

	client := newTestClient("test-key", server.URL)
	req := ChatRequest{
		Model:    context.ModelDeepseekChat,
		Messages: []prompt.Message{{Role: "user", Content: "hi"}},
		Stream:   true,
	}
	resp, err := client.chatStream(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID != streamID {
		t.Errorf("ChatResponse.ID = %q, want %q", resp.ID, streamID)
	}
	if got := resp.Choices[0].Message.Content; got != "Hello" {
		t.Errorf("content = %q, want %q", got, "Hello")
	}
}

// TestChatStreamIDFallback 验证 chunk 无 id 时回退时间戳占位（不产生空 ID）。
func TestChatStreamIDFallback(t *testing.T) {
	server := newSSETestServer(t, `{"choices":[{"delta":{"content":"ok"}}]}`)
	defer server.Close()

	client := newTestClient("test-key", server.URL)
	req := ChatRequest{
		Model:    context.ModelDeepseekChat,
		Messages: []prompt.Message{{Role: "user", Content: "hi"}},
		Stream:   true,
	}
	resp, err := client.chatStream(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(resp.ID, "streaming-response-") {
		t.Errorf("fallback ID = %q, want prefix streaming-response-", resp.ID)
	}
}
