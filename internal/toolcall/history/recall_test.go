// Package history 注册 recall/note 工具，供 LLM 使用
package history

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dscli/dscli/internal/prompt"
	"github.com/dscli/dscli/internal/toolcall"
)

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
		t.Errorf("期望无匹配提示，实际: %s", prompt.Truncate(result, 120))
	}
}
