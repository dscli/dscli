package prompt

import (
	"context"
	"strings"
	"testing"
)

func TestRecentMessages_Smoke(t *testing.T) {
	ctx := context.Background()
	msgs, err := RecentMessages(ctx, 5)
	if err != nil {
		t.Fatal("RecentMessages 失败:", err)
	}
	// 空结果也合法（新会话无消息）
	if len(msgs) > 5 {
		t.Errorf("预期最多 5 条，实际 %d", len(msgs))
	}
	// 验证按时间降序（i < j 则 created_at[i] >= created_at[j]）
	for i := 1; i < len(msgs); i++ {
		if msgs[i].CreatedAt.After(msgs[i-1].CreatedAt) {
			t.Errorf("消息未按降序排列: msgs[%d]=%v > msgs[%d]=%v",
				i, msgs[i].CreatedAt, i-1, msgs[i-1].CreatedAt)
		}
	}
}
func TestHandleRecent_Smoke(t *testing.T) {
	ctx := context.Background()
	result, _, err := HandleRecent(ctx, 20)
	if err != nil {
		t.Fatal("HandleRecent 失败:", err)
	}
	if result == "" {
		t.Error("HandleRecent 返回空结果")
	}
	// 空会话或有消息都合法：有消息应有表格，无消息应有提示
	if !strings.Contains(result, "| ID |") && !strings.Contains(result, "没有历史消息") {
		t.Errorf("HandleRecent 结果异常: %s", result)
	}
}

func TestHandleRecent_ZeroLimit(t *testing.T) {
	ctx := context.Background()
	result, _, err := HandleRecent(ctx, 0)
	if err != nil {
		t.Fatal("HandleRecent(0) 失败:", err)
	}
	if result == "" {
		t.Error("HandleRecent(0) 返回空结果")
	}
}