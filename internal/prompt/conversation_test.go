package prompt

import (
	"context"
	"testing"

	ictx "github.com/dscli/dscli/internal/context"
)

// TestConversationIDRoundTrip 验证 ConversationID 随消息落库并在
// ShowMessage / ListHistory / LoadHistory 三条读取路径上完整往返。
func TestConversationIDRoundTrip(t *testing.T) {
	ctx := withIsolatedSession(t)
	// 绕过 LoadHistory 的 token 截断逻辑，确保全部消息被加载。
	ctx = context.WithValue(ctx, ictx.LeftTokensKey, 1<<30)

	const convID = "b1e4a7f9-2c3d-4e5f-8a9b-0c1d2e3f4a5b"
	msgs := []Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi", ConversationID: convID},
	}
	if err := SaveMessages(ctx, msgs...); err != nil {
		t.Fatal(err)
	}

	// ListHistory：返回全部消息，assistant 消息字段保留。
	list, err := ListHistory(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("ListHistory len = %d, want 2", len(list))
	}
	assistant := findRole(list, "assistant")
	if assistant == nil {
		t.Fatal("ListHistory: assistant message missing")
	}
	if assistant.ConversationID != convID {
		t.Errorf("ListHistory ConversationID = %q, want %q", assistant.ConversationID, convID)
	}

	// ShowMessage：按实际 id 取回，验证 ConversationID。
	msg, err := ShowMessage(ctx, assistant.ID)
	if err != nil {
		t.Fatal(err)
	}
	if msg.ConversationID != convID {
		t.Errorf("ShowMessage ConversationID = %q, want %q", msg.ConversationID, convID)
	}
	// 用户消息不应带 ConversationID。
	user := findRole(list, "user")
	if user == nil {
		t.Fatal("ListHistory: user message missing")
	}
	userMsg, err := ShowMessage(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if userMsg.ConversationID != "" {
		t.Errorf("user message ConversationID = %q, want empty", userMsg.ConversationID)
	}

	// LoadHistory：重启路径，字段同样保留。
	hist, err := LoadHistory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 2 {
		t.Fatalf("LoadHistory len = %d, want 2", len(hist))
	}
	for _, m := range hist {
		if m.Role == "assistant" {
			if m.ConversationID != convID {
				t.Errorf("LoadHistory ConversationID = %q, want %q", m.ConversationID, convID)
			}
			return
		}
	}
	t.Error("LoadHistory: assistant message missing")
}

// findRole 按 role 查找第一条消息，避免依赖插入顺序/下标。
func findRole(msgs []*Message, role string) *Message {
	for _, m := range msgs {
		if m.Role == role {
			return m
		}
	}
	return nil
}
