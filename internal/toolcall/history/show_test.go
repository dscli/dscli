package history

import (
	"context"
	"strings"
	"testing"

	"github.com/dscli/dscli/internal/toolcall"
)

func TestHandleShow_MissingID(t *testing.T) {
	ctx := context.Background()
	args := toolcall.ToolArgs{}
	_, _, err := handleShow(ctx, args)
	if err == nil || !strings.Contains(err.Error(), "id") {
		t.Errorf("缺少 id 应报错: %v", err)
	}
}

func TestHandleShow_NotFound(t *testing.T) {
	ctx := context.Background()
	args := toolcall.ToolArgs{"id": 99999999}
	_, _, err := handleShow(ctx, args)
	if err == nil {
		t.Error("预期不存在的 ID 返回错误")
	}
}
