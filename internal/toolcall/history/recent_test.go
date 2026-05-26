package history

import (
	"context"
	"strings"
	"testing"

	"gitcode.com/dscli/dscli/internal/toolcall"
)

func TestHandleRecent_Smoke(t *testing.T) {
	ctx := context.Background()
	args := toolcall.ToolArgs{}
	result, _, err := handleRecent(ctx, args)
	if err != nil {
		t.Fatal("handleRecent 失败:", err)
	}
	// 空会话或有消息都合法
	if result == "" {
		t.Error("handleRecent 返回空结果")
	}
	if !strings.Contains(result, "| ID |") && !strings.Contains(result, "没有历史消息") {
		t.Errorf("handleRecent 结果异常: %s", result)
	}
}

func TestHandleRecent_Limit(t *testing.T) {
	ctx := context.Background()
	args := toolcall.ToolArgs{"limit": 3}
	result, _, err := handleRecent(ctx, args)
	if err != nil {
		t.Fatal("handleRecent 失败:", err)
	}
	if result == "" {
		t.Error("handleRecent 返回空结果")
	}
}
