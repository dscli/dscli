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
	if list[1].ConversationID != convID {
		t.Errorf("ListHistory ConversationID = %q, want %q", list[1].ConversationID, convID)
	}

	// ShowMessage：按实际 id 取回，验证 ConversationID。
	msg, err := ShowMessage(ctx, list[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	if msg.ConversationID != convID {
		t.Errorf("ShowMessage ConversationID = %q, want %q", msg.ConversationID, convID)
	}
	// 用户消息不应带 ConversationID。
	userMsg, err := ShowMessage(ctx, list[0].ID)
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
	if hist[1].ConversationID != convID {
		t.Errorf("LoadHistory ConversationID = %q, want %q", hist[1].ConversationID, convID)
	}
}
