package toolcall

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/prompt"
	"github.com/dscli/dscli/internal/session"
	"github.com/dscli/dscli/internal/sqlite"
)

// withIsolatedToolcallSession 让测试使用独立 session，避免共享消息数据。
func withIsolatedToolcallSession(t *testing.T) context.Context {
	t.Helper()
	old := context.ProjectRoot
	context.ProjectRoot = fmt.Sprintf("/tmp/dscli-interrupt-toolcall-test-%d", time.Now().UnixNano())
	session.ResetSessionID()
	t.Cleanup(func() {
		context.ProjectRoot = old
		session.ResetSessionID()
	})
	ctx := t.Context()
	return context.WithValue(ctx, context.CurrentModelIDKey, context.DeepseekChat)
}

// TestHandleInterrupt 模拟：assistant 发起 2 个工具调用，完成 1 个后收到
// SIGINT。验证 handleInterrupt 以 130 退出、tool_calls 被裁剪为已完成的
// 1 个、插入 1 条占位消息，且打印提示。
func TestHandleInterrupt(t *testing.T) {
	ctx := withIsolatedToolcallSession(t)
	tcs := []prompt.ToolCall{
		{ID: "call_1", Type: "function", Function: prompt.ToolCallFunction{Name: "git", Arguments: "{}"}},
		{ID: "call_2", Type: "function", Function: prompt.ToolCallFunction{Name: "shell", Arguments: "{}"}},
	}
	if err := prompt.SaveMessages(ctx,
		prompt.Message{Role: "user", Content: "hello"},
		prompt.Message{Role: "assistant", ToolCalls: tcs},
		prompt.Message{Role: "tool", ToolCallID: "call_1", Content: "ok"},
	); err != nil {
		t.Fatal(err)
	}

	// 模拟 HandleToolCalls 进度：已完成 1 个
	toolProgress.Store(1)

	exitCode := -1
	handleInterrupt(ctx, func(code int) { exitCode = code })

	if exitCode != 130 {
		t.Fatalf("want exit code 130, got %d", exitCode)
	}

	// 验证 DB 状态：assistant 的 tool_calls 只剩 call_1
	db, err := sqlite.OpenDB(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close(ctx)
	var toolCalls sql.NullString
	err = db.QueryRowContext(ctx, `
		SELECT tool_calls FROM messages
		WHERE session_id = ? AND role = 'assistant' AND tool_calls IS NOT NULL
		ORDER BY id DESC LIMIT 1`, session.GetCurrentSessionID(ctx)).Scan(&toolCalls)
	if err != nil {
		t.Fatal(err)
	}
	var got []prompt.ToolCall
	if err := json.Unmarshal([]byte(toolCalls.String), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "call_1" {
		t.Fatalf("assistant tool_calls should keep only call_1, got %+v", got)
	}

	// 验证占位消息：1 条 tool 消息（call_2），内容含中断提示
	var content string
	err = db.QueryRowContext(ctx, `
		SELECT content FROM messages
		WHERE session_id = ? AND role = 'tool' AND tool_call_id = 'call_2'`,
		session.GetCurrentSessionID(ctx)).Scan(&content)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "被用户中断") {
		t.Fatalf("placeholder content should mention interrupt: %q", content)
	}
}

// TestInstallInterruptHandler 冒烟测试：注册/注销不应 panic，stop 可重复调用。
func TestInstallInterruptHandler(t *testing.T) {
	ctx := t.Context()
	stop := InstallInterruptHandler(ctx)
	stop()
	stop() // 重复调用应安全（sync.Once 保护）
}

// TestInterruptedToolContent 占位消息内容应包含工具名和中断提示。
func TestInterruptedToolContent(t *testing.T) {
	c := prompt.InterruptedToolContent("write_file")
	if !strings.Contains(c, "write_file") || !strings.Contains(c, "被用户中断") {
		t.Fatalf("unexpected placeholder content: %q", c)
	}
}
