package toolcall

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/prompt"
	"github.com/dscli/dscli/internal/session"
)

// withIsolatedDualSession 让测试使用独立 ProjectRoot + session，避免
// 污染真实 sqlite.db / 消息表。
func withIsolatedDualSession(t *testing.T) context.Context {
	t.Helper()
	old := context.ProjectRoot
	context.ProjectRoot = fmt.Sprintf("/tmp/dscli-dual-test-%d", time.Now().UnixNano())
	session.ResetSessionID()
	t.Cleanup(func() {
		context.ProjectRoot = old
		session.ResetSessionID()
	})
	return context.WithValue(t.Context(), context.CurrentModelIDKey, context.DeepseekChat)
}

// unregisterToolForTest 删除测试临时注册的工具（注册表是包级 map，
// 测试间共享；删除避免影响其他用例的工具列表断言）。
func unregisterToolForTest(name string) {
	toolRegistryRWMutex.Lock()
	defer toolRegistryRWMutex.Unlock()
	delete(toolRegistry, name)
	// Aliases must go with their tool, or a later test registering the
	// same alias (e.g. vision_file_upload) fails with "already registered".
	for alias, canonical := range toolAliases {
		if canonical == name {
			delete(toolAliases, alias)
		}
	}
}

// TestHandleToolCallsDualInjection 端到端验证：注册一个返回 DualMessage
// 的工具，HandleToolCalls 应拆分出 tool 消息 + user 消息（顺序：全部
// tool 在前，user 在后），且都落库。
func TestHandleToolCallsDualInjection(t *testing.T) {
	ctx := withIsolatedDualSession(t)

	// 注册临时工具（与 vision_file_read 同协议）
	const toolName = "test_dual_reader"
	if err := RegisterTool(ToolDef{
		Name:     toolName,
		Category: "vision",
		Handler: func(_ context.Context, _ ToolArgs) (result, warning string, err error) {
			dual, mErr := MarshalDual(NewDual(`{"id":"file-api-777","object":"file"}`, &prompt.Message{
				Content: "图片已加载（file_id: file-api-777）",
				ContentBlocks: []prompt.ContentBlock{
					prompt.TextBlock("图片已加载"),
					prompt.FileBlock("file-api-777"),
				},
			}))
			return dual, "", mErr
		},
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { unregisterToolForTest(toolName) })

	tcs := []prompt.ToolCall{
		{ID: "call_1", Type: "function", Function: prompt.ToolCallFunction{Name: toolName, Arguments: `{"file":"a.png"}`}},
	}

	inputs := HandleToolCalls(ctx, tcs)
	if len(inputs) != 2 {
		t.Fatalf("expected 2 messages (tool+user), got %d: %+v", len(inputs), inputs)
	}
	if inputs[0].Role != "tool" || inputs[0].ToolCallID != "call_1" {
		t.Fatalf("first message should be tool: %+v", inputs[0])
	}
	if !strings.Contains(inputs[0].Content, "file-api-777") {
		t.Fatalf("tool content should keep metadata: %q", inputs[0].Content)
	}
	if inputs[1].Role != "user" || len(inputs[1].ContentBlocks) != 2 {
		t.Fatalf("second message should be user with file block: %+v", inputs[1])
	}
	if inputs[1].ContentBlocks[1].FileID != "file-api-777" {
		t.Fatalf("file block id mismatch: %+v", inputs[1].ContentBlocks[1])
	}
}
