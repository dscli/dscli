package prompt

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	ictx "github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/session"
	"github.com/dscli/dscli/internal/sqlite"
)

// withIsolatedSession 让测试使用独立 session，避免与其他测试共享消息数据。
// context.ProjectRoot 是包级变量，测试结束后恢复。
func withIsolatedSession(t *testing.T) context.Context {
	t.Helper()
	old := ictx.ProjectRoot
	ictx.ProjectRoot = fmt.Sprintf("/tmp/dscli-interrupt-test-%d", time.Now().UnixNano())
	session.ResetSessionID()
	t.Cleanup(func() {
		ictx.ProjectRoot = old
		session.ResetSessionID()
	})
	ctx := t.Context()
	return ictx.WithValue(ctx, ictx.CurrentModelIDKey, ictx.DeepseekChat)
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
// 用户 Ctrl+C。验证 tool_calls 完整保留（不裁剪），插入 2 条占位消息。
func TestMarkInterruptedToolCalls(t *testing.T) {
	ctx := withIsolatedSession(t)
	tcs := testToolCalls()
	if err := SaveMessages(
		ctx,
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

	// assistant 的 tool_calls 应完整保留（不裁剪）：CleanupReverse 按
	// 数量配对，裁剪会破坏配对导致整块历史被丢弃。
	msg, err := lastToolCallMessage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(msg.ToolCalls) != 3 ||
		msg.ToolCalls[0].ID != "call_1" || msg.ToolCalls[2].ID != "call_3" {
		t.Fatalf("assistant tool_calls should keep all 3 calls, got %+v", msg.ToolCalls)
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
// tool_calls 完整保留，全部 3 个调用插入占位消息。
func TestMarkInterruptedToolCallsZeroCompleted(t *testing.T) {
	ctx := withIsolatedSession(t)
	tcs := testToolCalls()
	if err := SaveMessages(
		ctx,
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

	// tool_calls 完整保留（不裁剪为空）：历史最后一条是占位 tool 消息，
	// 重启后 ChatRunE 不会重放。
	msg, err := lastToolCallMessage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(msg.ToolCalls) != 3 {
		t.Fatalf("tool_calls should keep all 3 calls, got %+v", msg.ToolCalls)
	}
}

// TestMarkInterruptedToolCallsAllCompleted 模拟：所有工具都已完成时信号到达。
// 不应做任何修改。
func TestMarkInterruptedToolCallsAllCompleted(t *testing.T) {
	ctx := withIsolatedSession(t)
	tcs := testToolCalls()
	if err := SaveMessages(
		ctx,
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

// TestMarkInterruptedToolCallsKeepsHistory 集成测试：标记中断后 LoadHistory
// 必须保留完整历史（user + assistant(tc×3) + tool×3），且最后一条是占位
// tool 消息——ChatRunE 的重放条件（最后一条 assistant 带 tool_calls）不成立。
// 这是 review 指出的盲区：只验证 DB 状态不够，必须覆盖重启路径。
func TestMarkInterruptedToolCallsKeepsHistory(t *testing.T) {
	ctx := withIsolatedSession(t)
	// 绕过 LoadHistory 的 token 截断逻辑，确保全部消息被加载。
	ctx = context.WithValue(ctx, ictx.LeftTokensKey, 1<<30)

	tcs := testToolCalls()
	if err := SaveMessages(
		ctx,
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

	// 重启路径：LoadHistory 必须完整保留 5 条消息（不裁剪时 CleanupReverse
	// 按 3 个 tool_calls 配对 3 条 tool 消息，通过校验）。
	hist, err := LoadHistory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 5 {
		t.Fatalf("want full history of 5 messages, got %d: %+v", len(hist), hist)
	}
	wantRoles := []string{"user", "assistant", "tool", "tool", "tool"}
	for i, m := range hist {
		if m.Role != wantRoles[i] {
			t.Fatalf("message %d role = %q, want %q", i, m.Role, wantRoles[i])
		}
	}
	if len(hist[1].ToolCalls) != 3 {
		t.Fatalf("assistant should keep all 3 tool_calls, got %+v", hist[1].ToolCalls)
	}

	// 最后一条是占位 tool 消息 → 不触发重放。
	last := hist[len(hist)-1]
	if last.Role != "tool" || last.ToolCallID != "call_3" ||
		!strings.Contains(last.Content, "被用户中断") {
		t.Fatalf("last message should be call_3 placeholder, got %+v", last)
	}
}
