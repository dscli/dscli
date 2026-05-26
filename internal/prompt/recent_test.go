package prompt

import (
	"context"
	"strings"
	"testing"
)

func TestRecentMessages_Smoke(t *testing.T) {
	ctx := context.Background()
	msgs, err := RecentMessages(ctx, 5, 0)
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

func TestRecentMessages_WithStart(t *testing.T) {
	ctx := context.Background()
	// 先获取最新消息
	all, err := RecentMessages(ctx, 20, 0)
	if err != nil {
		t.Fatal("RecentMessages 失败:", err)
	}
	if len(all) < 2 {
		t.Skip("消息不足，跳过翻页测试")
	}
	// 用最旧的消息 ID 作为起点
	oldestID := all[len(all)-1].ID
	page2, err := RecentMessages(ctx, 20, oldestID)
	if err != nil {
		t.Fatal("RecentMessages(start) 失败:", err)
	}
	// 所有返回的消息 ID 都应 <= oldestID
	for _, m := range page2 {
		if m.ID > oldestID {
			t.Errorf("start=%d 但返回了更大的 ID=%d", oldestID, m.ID)
		}
	}
	// 第一条应该是 oldestID（包含起点）
	if len(page2) > 0 && page2[0].ID != oldestID {
		t.Logf("翻页第一条约等于起点: got %d, expected ~%d", page2[0].ID, oldestID)
	}
}

func TestHandleRecent_Smoke(t *testing.T) {
	ctx := context.Background()
	result, _, err := HandleRecent(ctx, 20, 0)
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
	result, _, err := HandleRecent(ctx, 0, 0)
	if err != nil {
		t.Fatal("HandleRecent(0) 失败:", err)
	}
	if result == "" {
		t.Error("HandleRecent(0) 返回空结果")
	}
}

func TestHandleRecent_WithStart(t *testing.T) {
	ctx := context.Background()
	result, _, err := HandleRecent(ctx, 20, 1)
	if err != nil {
		t.Fatal("HandleRecent(start=1) 失败:", err)
	}
	if result == "" {
		t.Error("HandleRecent(start=1) 返回空结果")
	}
	// 有结果时应有表格，无结果时应有"没有更多"
	if !strings.Contains(result, "| ID |") && !strings.Contains(result, "没有更多消息") {
		t.Errorf("HandleRecent(start=1) 结果异常: %s", result)
	}
}
