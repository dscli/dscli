package prompt

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/session"
	"github.com/dscli/dscli/internal/sqlite"
)

// withIsolatedSession 让测试使用独立 session，避免与其他测试共享消息数据。
// context.ProjectRoot 是包级变量，测试结束后恢复。
func withIsolatedSession(t *testing.T) context.Context {
	t.Helper()
	old := context.ProjectRoot
	context.ProjectRoot = fmt.Sprintf("/tmp/dscli-interrupt-test-%d", time.Now().UnixNano())
	session.ResetSessionID()
	t.Cleanup(func() {
		context.ProjectRoot = old
		session.ResetSessionID()
	})
	ctx := t.Context()
	return context.WithValue(ctx, context.CurrentModelIDKey, context.DeepseekChat)
}

func testToolCalls() []ToolCall {
	return []ToolCall{
		{ID: "call_1", Type: "function", Function: ToolCallFunction{Name: "git", Arguments: "{}"}},
		{ID: "call_2", Type: "function", Function: ToolCallFunction{Name: "write_file", Arguments: "{}"}},
		{ID: "call_3", Type: "function", Function: ToolCallFunction{Name: "shell", Arguments: "{}"}},
	}
}

// lastToolMessages 查询最近 limit 条 tool 消息（id 降序）。
func lastToolMessages(t *testing.T, ctx context.Context, limit int) []Message {
	t.Helper()
	db, err := sqlite.OpenDB(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close(ctx)
	rows, err := db.QueryContext(ctx, `
		SELECT id, role, content, tool_call_id
		FROM messages
		WHERE session_id = ? AND role = 'tool'
		ORDER BY id DESC LIMIT ?`, session.GetCurrentSessionID(ctx), limit)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var msgs []Message
	for rows.Next() {
		var m Message
		var tcid sql.NullString
		if err := rows.Scan(&m.ID, &m.Role, &m.Content, &tcid); err != nil {
			t.Fatal(err)
		}
		if tcid.Valid {
			m.ToolCallID = tcid.String
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return msgs
}

// TestMarkInterruptedToolCalls 模拟：assistant 发起 3 个工具调用，完成 1 个后
// 用户 Ctrl+C。验证 tool_calls 被裁剪为已完成的 1 个，且插入 2 条占位消息。
func TestMarkInterruptedToolCalls(t *testing.T) {
	ctx := withIsolatedSession(t)
	tcs := testToolCalls()
	if err := SaveMessages(ctx,
		Message{Role: "user", Content: "hello"},
		Message{Role: "assistant", ToolCalls: tcs},
		Message{Role: "tool", ToolCallID: "call_1", Content: "ok"},
	); err != nil {
		t.Fatal(err)
	}

	n, err := MarkInterruptedToolCalls(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("want 2 placeholders, got %d", n)
	}

	// assistant 的 tool_calls 应只剩已完成的 call_1
	msg, err := lastToolCallMessage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].ID != "call_1" {
		t.Fatalf("assistant tool_calls should keep only call_1, got %+v", msg.ToolCalls)
	}

	// 应插入 2 条占位 tool 消息，ToolCallID 对应 call_2/call_3
	tools := lastToolMessages(t, ctx, 2)
	if len(tools) != 2 {
		t.Fatalf("want 2 placeholder tool messages, got %d", len(tools))
	}
	gotIDs := []string{tools[1].ToolCallID, tools[0].ToolCallID} // 恢复升序
	if gotIDs[0] != "call_2" || gotIDs[1] != "call_3" {
		t.Fatalf("placeholder ids want call_2,call_3 got %v", gotIDs)
	}
	for _, m := range tools {
		if !strings.Contains(m.Content, "被用户中断") {
			t.Fatalf("placeholder content should mention interrupt: %q", m.Content)
		}
	}
}

// TestMarkInterruptedToolCallsZeroCompleted 模拟：第一个工具执行中即被中断。
// 所有调用都应裁剪为未执行（tool_calls 为空），全部插入占位消息。
func TestMarkInterruptedToolCallsZeroCompleted(t *testing.T) {
	ctx := withIsolatedSession(t)
	tcs := testToolCalls()
	if err := SaveMessages(ctx,
		Message{Role: "user", Content: "hello"},
		Message{Role: "assistant", ToolCalls: tcs},
	); err != nil {
		t.Fatal(err)
	}

	n, err := MarkInterruptedToolCalls(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("want 3 placeholders, got %d", n)
	}

	// tool_calls 被裁剪为空：重启后 ChatRunE 不会重放
	msg, err := lastToolCallMessage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(msg.ToolCalls) != 0 {
		t.Fatalf("tool_calls should be empty, got %+v", msg.ToolCalls)
	}
}

// TestMarkInterruptedToolCallsAllCompleted 模拟：所有工具都已完成时信号到达。
// 不应做任何修改。
func TestMarkInterruptedToolCallsAllCompleted(t *testing.T) {
	ctx := withIsolatedSession(t)
	tcs := testToolCalls()
	if err := SaveMessages(ctx,
		Message{Role: "assistant", ToolCalls: tcs},
		Message{Role: "tool", ToolCallID: "call_1", Content: "ok"},
		Message{Role: "tool", ToolCallID: "call_2", Content: "ok"},
		Message{Role: "tool", ToolCallID: "call_3", Content: "ok"},
	); err != nil {
		t.Fatal(err)
	}

	n, err := MarkInterruptedToolCalls(ctx, len(tcs))
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("want 0 placeholders, got %d", n)
	}

	msg, err := lastToolCallMessage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(msg.ToolCalls) != 3 {
		t.Fatalf("tool_calls should be untouched, got %+v", msg.ToolCalls)
	}
	if got := lastToolMessages(t, ctx, 10); len(got) != 3 {
		t.Fatalf("no placeholder should be inserted, got %d tool messages", len(got))
	}
}

// TestMarkInterruptedToolCallsNoResidue 无残留调用：不应报错也不应修改。
func TestMarkInterruptedToolCallsNoResidue(t *testing.T) {
	ctx := withIsolatedSession(t)
	if err := SaveMessages(ctx, Message{Role: "user", Content: "hello"}); err != nil {
		t.Fatal(err)
	}

	n, err := MarkInterruptedToolCalls(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("want 0 placeholders, got %d", n)
	}
}
