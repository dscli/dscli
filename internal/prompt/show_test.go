package prompt

import (
	"context"
	"strings"
	"testing"
)

func TestHandleShow_NotFound(t *testing.T) {
	ctx := context.Background()
	_, _, err := HandleShow(ctx, 99999999)
	if err == nil {
		t.Error("预期不存在的 ID 返回错误")
	}
	if err != nil && !strings.Contains(err.Error(), "失败") {
		t.Errorf("错误信息应包含'失败': %v", err)
	}
}
