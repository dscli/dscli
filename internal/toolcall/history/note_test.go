// Package history 注册 recall/note 工具，供 LLM 使用
package history

import (
	"context"
	"strings"
	"testing"

	"github.com/dscli/dscli/internal/toolcall"
)

// TestHandleNote_Success 验证成功保存笔记
func TestHandleNote_Success(t *testing.T) {
	ctx := context.Background()
	args := toolcall.ToolArgs{"content": "测试笔记-单元测试"}
	result, suggestion, err := handleNote(ctx, args)
	if err != nil {
		t.Fatal("handleNote 失败:", err)
	}
	if result != "笔记已保存。" {
		t.Errorf("result = %q, want %q", result, "笔记已保存。")
	}
	if suggestion != "" {
		t.Logf("suggestion: %s", suggestion)
	}
}

// TestHandleNote_Empty 验证空内容报错
func TestHandleNote_Empty(t *testing.T) {
	ctx := context.Background()
	args := toolcall.ToolArgs{"content": ""}
	_, _, err := handleNote(ctx, args)
	if err == nil {
		t.Error("空内容应报错")
	}
}

// TestHandleNote_LongContent 验证超长内容返回错误（不再静默截断）
func TestHandleNote_LongContent(t *testing.T) {
	ctx := context.Background()
	longContent := strings.Repeat("测", 121) // 121字，超过 120
	args := toolcall.ToolArgs{"content": longContent}
	_, _, err := handleNote(ctx, args)
	if err == nil {
		t.Error("超长内容应返回错误")
	}
	if err != nil && !strings.Contains(err.Error(), "超过") {
		t.Errorf("错误信息应提示超限: %v", err)
	}
}
