// Package history 注册 recall/note 工具，供 LLM 使用
package history

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"gitcode.com/dscli/dscli/internal/history"
	"gitcode.com/dscli/dscli/internal/toolcall"
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

// TestHandleNote_LongContent 验证超长内容给出建议
func TestHandleNote_LongContent(t *testing.T) {
	ctx := context.Background()
	longContent := strings.Repeat("测试超长笔记内容", 10) // 80 字，超过 40
	args := toolcall.ToolArgs{"content": longContent}
	_, suggestion, err := handleNote(ctx, args)
	if err != nil {
		t.Fatal("handleNote 失败:", err)
	}
	if !strings.Contains(suggestion, "截断") {
		t.Errorf("超长内容应有截断建议: %s", suggestion)
	}
}

// TestHandleRecall_MissingKeywords 验证缺少关键词报错
func TestHandleRecall_MissingKeywords(t *testing.T) {
	ctx := context.Background()
	args := toolcall.ToolArgs{}
	_, _, err := handleRecall(ctx, args)
	if err == nil || !strings.Contains(err.Error(), "keywords") {
		t.Errorf("缺少 keywords 应报错: %v", err)
	}
}

// TestHandleRecall_NoMatch 验证无匹配结果
func TestHandleRecall_NoMatch(t *testing.T) {
	ctx := context.Background()
	// 用纳秒时间戳生成唯一关键词，避免 DB 中实际匹配
	uniqueKeyword := fmt.Sprintf("xy-no-match-%d", time.Now().UnixNano())
	args := toolcall.ToolArgs{
		"keywords": uniqueKeyword,
		"limit":    1,
	}
	result, _, err := handleRecall(ctx, args)
	if err != nil {
		t.Fatal("handleRecall 失败:", err)
	}
	if !strings.Contains(result, "没有找到匹配") {
		t.Errorf("期望无匹配提示，实际: %s", history.Truncate(result, 120))
	}
}