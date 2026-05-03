package prompt

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestFormatTime(t *testing.T) {
	now := time.Now()

	today := now.Format("15:04")
	if got := FormatTime(now); got != today {
		t.Errorf("FormatTime(today) = %s, want %s", got, today)
	}

	thisYear := time.Date(now.Year(), 6, 15, 10, 30, 0, 0, now.Location())
	wantThisYear := thisYear.Format("01-02 15:04")
	if got := FormatTime(thisYear); got != wantThisYear {
		t.Errorf("FormatTime(this year) = %s, want %s", got, wantThisYear)
	}

	otherYear := time.Date(2024, 12, 1, 8, 0, 0, 0, now.Location())
	wantOtherYear := otherYear.Format("2006-01-02 15:04")
	if got := FormatTime(otherYear); got != wantOtherYear {
		t.Errorf("FormatTime(other year) = %s, want %s", got, wantOtherYear)
	}
}

// TestBuildNotePrompt_Empty 验证无笔记时返回空字符串
// TestBuildNotePrompt_Smoke 冒烟测试：验证 BuildNotePrompt 能正常运行
// 测试环境中可能有历史笔记，不强断言结果为空。
func TestBuildNotePrompt_Smoke(t *testing.T) {
	ctx := context.Background()
	result := BuildNotePrompt(ctx)
	// 有笔记时应包含标题，无笔记时为空——两种都合法
	if result != "" {
		if !strings.Contains(result, "近期对话笔记") {
			t.Errorf("BuildNotePrompt 有内容但缺少标题: %s", result)
		}
	}
	// 确保不会 panic
}

// TestSaveNoteAndBuildNotePrompt 集成测试：保存笔记后能通过 BuildNotePrompt 召回
func TestSaveNoteAndBuildNotePrompt(t *testing.T) {
	ctx := context.Background()

	// 保存一条唯一笔记（用时间戳保证不重复）
	uniqueContent := fmt.Sprintf("集成测试-笔记-%s", time.Now().Format("150405"))
	if err := SaveNote(ctx, uniqueContent); err != nil {
		t.Fatal("SaveNote 失败:", err)
	}

	// 立即通过 BuildNotePrompt 检查是否出现
	result := BuildNotePrompt(ctx)
	if result == "" {
		t.Fatal("BuildNotePrompt 返回空，期望包含刚保存的笔记")
	}
	if !strings.Contains(result, uniqueContent) {
		t.Errorf("BuildNotePrompt 不含刚保存的笔记 %q", uniqueContent)
		t.Logf("实际内容: %s", result)
	}
	// 检查格式
	if !strings.Contains(result, "## 📝 近期对话笔记") {
		t.Error("BuildNotePrompt 缺少笔记区域标题")
	}
}

// TestSaveNote_Truncation 验证超过40字自动截断
// TestSaveNote_Truncation 验证超过40字自动截断
func TestSaveNote_Truncation(t *testing.T) {
	ctx := context.Background()

	// 构造明确超过40字的字符串（50个汉字）
	longContent := "这是一条超级长的笔记内容用来测试截断功能是否正常工作这段文字有五十多个汉字应该会被自动截断到前四十个字"
	if len([]rune(longContent)) <= 40 {
		t.Fatalf("测试数据错误: longContent 只有 %d 字，需要 >40", len([]rune(longContent)))
	}

	if err := SaveNote(ctx, longContent); err != nil {
		t.Fatal("SaveNote 失败:", err)
	}

	// 验证截断后的内容长度
	result := BuildNotePrompt(ctx)
	if result == "" {
		t.Fatal("BuildNotePrompt 返回空")
	}
	// 截断后的前40个rune应出现
	expectedTruncated := string([]rune(longContent)[:40])
	if !strings.Contains(result, expectedTruncated) {
		t.Errorf("BuildNotePrompt 中未找到截断后的内容")
		t.Logf("期望包含: %q", expectedTruncated)
		t.Logf("实际内容: %s", result)
	}
	// 原始超长内容不应完整出现（已被截断）
	if strings.Contains(result, longContent) {
		t.Error("BuildNotePrompt 中出现了未截断的超长内容")
	}
}

// TestSaveNote_Empty 验证空内容报错
func TestSaveNote_Empty(t *testing.T) {
	ctx := context.Background()

	err := SaveNote(ctx, "")
	if err == nil {
		t.Error("期望空内容报错，但返回 nil")
	}
	if err != nil && !strings.Contains(err.Error(), "不能为空") {
		t.Errorf("错误信息不匹配: %v", err)
	}
}

// TestLoadNotes_DefaultDays 验证默认天数加载
// TestLoadNotes_RecentFirst 验证笔记按降序排列
func TestLoadNotes_RecentFirst(t *testing.T) {
	ctx := context.Background()

	notes, err := LoadNotes(ctx, 30)
	if err != nil {
		t.Fatal("LoadNotes 失败:", err)
	}
	if len(notes) < 2 {
		t.Skip("需要至少 2 条笔记才能验证降序")
	}
	// 结果按时间降序排列，验证最近的在前面
	for i := 1; i < len(notes); i++ {
		if notes[i].CreatedAt.After(notes[i-1].CreatedAt) {
			t.Errorf("笔记未按降序排列: notes[%d]=%v > notes[%d]=%v",
				i, notes[i].CreatedAt, i-1, notes[i-1].CreatedAt)
		}
	}
}