package dsml

import (
	"fmt"
	"testing"
	"time"

	"github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/session"
)

// withIsolatedDualSession 让测试使用独立 ProjectRoot + session，避免
// 污染真实 sqlite.db / 消息表。（原 internal/toolcall/dual_integration_test.go
// 的同名副本。toolcall 的注册表清理统一走 bridge 导出的 toolcall.UnregisterTool。）
func withIsolatedDualSession(t *testing.T) context.Context {
	t.Helper()
	old := context.ProjectRoot
	context.ProjectRoot = fmt.Sprintf("/tmp/dscli-dsml-test-%d", time.Now().UnixNano())
	session.ResetSessionID()
	t.Cleanup(func() {
		context.ProjectRoot = old
		session.ResetSessionID()
	})
	return context.WithValue(t.Context(), context.CurrentModelIDKey, context.DeepseekChat)
}
