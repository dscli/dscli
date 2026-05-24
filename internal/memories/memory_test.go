package memories

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitcode.com/dscli/dscli/internal/sqlite"
)

func TestTruncate(t *testing.T) {
	tests := []struct {
		input string
		max   int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello world", 20, "hello world"},
		{"", 10, ""},
		{"hello", 5, "hello"},
		{"你好世界", 4, "你好世界"},
		{"hello world", 5, "hello..."},
		{"你好世界", 3, "你好世..."},
		{"一二三四五六", 5, "一二三四五..."},
		{"一二三四五", 5, "一二三四五"},
		{"AB中文", 3, "AB中..."},
		{"中文AB", 3, "中文A..."},
	}
	for _, tt := range tests {
		name := fmt.Sprintf("%q_%d", tt.input, tt.max)
		t.Run(name, func(t *testing.T) {
			got := truncate(tt.input, tt.max)
			if got != tt.want {
				t.Errorf("truncate(%q, %d)\n  got:  %q\n  want: %q",
					tt.input, tt.max, got, tt.want)
			}
		})
	}
}

// === FTS Sync Helpers =========================================================

func TestInsertAndDeleteFTS(t *testing.T) {
	newTestDB(t)
	db, err := openDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Insert with tokenized Chinese
	if err := insertFTS(db, 1, "测试标题", "中文内容 English", "test"); err != nil {
		t.Fatalf("insertFTS failed: %v", err)
	}

	// Verify FTS row exists
	var count int
	db.QueryRow("SELECT COUNT(*) FROM memories_fts WHERE rowid = 1").Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 FTS row, got %d", count)
	}

	// Delete
	if err := deleteFTS(db, 1); err != nil {
		t.Fatalf("deleteFTS failed: %v", err)
	}

	db.QueryRow("SELECT COUNT(*) FROM memories_fts WHERE rowid = 1").Scan(&count)
	if count != 0 {
		t.Errorf("expected 0 FTS rows after delete, got %d", count)
	}
}

// === DB-Integrated Tests ======================================================

func newTestDB(t *testing.T) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "memories-test.db")
	sqlite.SetDBPath(dbPath)
}

// found reports whether the search result indicates matches were found.
func found(content string) bool {
	return !strings.Contains(content, "未找到")
}

// saveMem is a helper that inserts a memory and returns its ID.
func saveMem(t *testing.T, title, body, typ string) int64 {
	t.Helper()
	content, _, err := HandleMemSave(context.Background(), title, body, typ)
	if err != nil {
		t.Fatalf("saveMem(%q): %v", title, err)
	}
	var id int64
	if _, scanErr := fmt.Sscanf(content, "✅ 记忆已保存: #%d", &id); scanErr != nil {
		t.Fatalf("saveMem(%q): failed to parse ID from: %s", title, content)
	}
	return id
}

// === HandleMemSave ============================================================

func TestHandleMemSave(t *testing.T) {
	newTestDB(t)

	t.Run("success", func(t *testing.T) {
		content, _, err := HandleMemSave(context.Background(),
			"测试标题", "这是测试内容", "test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(content, "记忆已保存") {
			t.Errorf("expected '记忆已保存', got: %s", content)
		}
	})

	t.Run("empty title", func(t *testing.T) {
		_, _, err := HandleMemSave(context.Background(), "", "content", "test")
		if err == nil {
			t.Error("expected error for empty title")
		}
	})

	t.Run("empty content", func(t *testing.T) {
		_, _, err := HandleMemSave(context.Background(), "title", "", "test")
		if err == nil {
			t.Error("expected error for empty content")
		}
	})

	t.Run("chinese content", func(t *testing.T) {
		_, _, err := HandleMemSave(context.Background(),
			"中文记忆", "这是一段中文内容，包含标点符号和数字123。", "test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("very long content", func(t *testing.T) {
		longContent := strings.Repeat("A", 50001)
		_, _, err := HandleMemSave(context.Background(),
			"长内容测试", longContent, "test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// === HandleMemUpdate ==========================================================

func TestHandleMemUpdate(t *testing.T) {
	newTestDB(t)

	t.Run("update title", func(t *testing.T) {
		id := saveMem(t, "原标题", "原始内容", "test")
		content, _, err := HandleMemUpdate(context.Background(), id, "新标题", "", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(content, "已更新") {
			t.Errorf("expected '已更新', got: %s", content)
		}
	})

	t.Run("update content", func(t *testing.T) {
		id := saveMem(t, "标题", "旧内容", "test")
		content, _, err := HandleMemUpdate(context.Background(), id, "", "新内容", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(content, "已更新") {
			t.Errorf("expected '已更新', got: %s", content)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, _, err := HandleMemUpdate(context.Background(), 99999, "title", "", "")
		if err == nil {
			t.Error("expected error for non-existent ID")
		}
	})

	t.Run("update chinese content", func(t *testing.T) {
		id := saveMem(t, "中文更新", "旧的中文内容", "test")
		_, _, err := HandleMemUpdate(context.Background(), id, "", "新的中文内容", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// === HandleMemSearch ==========================================================

func TestHandleMemSearch(t *testing.T) {
	newTestDB(t)

	saveMem(t, "JWT认证中间件实现", "实现了基于RS256的JWT token验证", "architecture")
	saveMem(t, "修复登录超时bug", "当token过期时前端没有正确处理401响应", "bugfix")
	saveMem(t, "Go单元测试最佳实践", "使用table-driven tests模式组织测试用例", "pattern")
	saveMem(t, "中文全文搜索测试", "FTS5默认分词器对中文的处理方式是按字分词", "test")
	saveMem(t, "混合内容English混合", "This content mixes Chinese 中文 and English for testing purposes", "test")

	t.Run("basic search (ASCII)", func(t *testing.T) {
		content, _, err := HandleMemSearch(context.Background(), "JWT", "", 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !found(content) {
			t.Errorf("expected results for 'JWT', got: %s", content)
		}
	})

	t.Run("chinese word search", func(t *testing.T) {
		// "全文搜索" → gse: ["全文", "搜索"] → AND match
		content, _, err := HandleMemSearch(context.Background(), "全文搜索", "", 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !found(content) {
			t.Errorf("expected '全文搜索' to find results, got: %s", content)
		}
	})

	t.Run("chinese word match — '分词'", func(t *testing.T) {
		// "分词" as a gse word appears in content of #4
		content, _, err := HandleMemSearch(context.Background(), "分词", "", 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !found(content) {
			t.Errorf("expected '分词' to find results, got: %s", content)
		}
	})

	t.Run("chinese — '测试' finds multiple", func(t *testing.T) {
		content, _, err := HandleMemSearch(context.Background(), "测试", "", 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !found(content) {
			t.Errorf("expected '测试' to find results, got: %s", content)
		}
	})

	t.Run("single chinese character", func(t *testing.T) {
		// gse CutSearch: "按字分词" → ["按", "字", "分词"], "字" is standalone
		content, _, err := HandleMemSearch(context.Background(), "字", "", 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !found(content) {
			t.Errorf("expected '字' to find results, got: %s", content)
		}
	})

	t.Run("mixed language search", func(t *testing.T) {
		content, _, err := HandleMemSearch(context.Background(), "English 中文", "", 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !found(content) {
			t.Errorf("expected results for 'English 中文', got: %s", content)
		}
	})

	t.Run("mixed CJK+ASCII", func(t *testing.T) {
		// "Go单元" → gse: ["go", "单元"] → AND match
		content, _, err := HandleMemSearch(context.Background(), "Go单元", "", 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !found(content) {
			t.Errorf("expected 'Go单元' to find results, got: %s", content)
		}
	})

	t.Run("type filter", func(t *testing.T) {
		content, _, err := HandleMemSearch(context.Background(), "测试", "pattern", 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !found(content) {
			t.Errorf("expected type=pattern results for '测试', got: %s", content)
		}
		// Should NOT include type=test results
		if strings.Contains(content, "中文全文搜索测试") {
			t.Errorf("type=test result should not appear in pattern filter")
		}
	})

	t.Run("limit results", func(t *testing.T) {
		content, _, err := HandleMemSearch(context.Background(), "测试", "", 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !found(content) {
			t.Errorf("expected results with limit=1, got: %s", content)
		}
		if strings.Count(content, "[1] #") != 1 {
			t.Errorf("expected exactly 1 result, got: %s", content)
		}
	})

	t.Run("not found", func(t *testing.T) {
		content, _, err := HandleMemSearch(context.Background(), "不存在的关键词xyzabc123", "", 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if found(content) {
			t.Errorf("expected '未找到' for nonexistent term, got: %s", content)
		}
	})

	t.Run("word-level: '数据' does NOT match '数据库'", func(t *testing.T) {
		// gse word-level: "数据" ≠ "数据库"
		// "数据库" is not in the test data, so search for a word that doesn't exist
		content, _, err := HandleMemSearch(context.Background(), "数据库", "", 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if found(content) {
			t.Errorf("expected '数据库' to NOT match (word-level precision), but got: %s", content)
		}
	})
}

// === HandleMemDelete ==========================================================

func TestHandleMemDelete(t *testing.T) {
	newTestDB(t)

	t.Run("success", func(t *testing.T) {
		id := saveMem(t, "待删除", "内容", "test")
		content, _, err := HandleMemDelete(context.Background(), id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(content, "已删除") {
			t.Errorf("expected '已删除', got: %s", content)
		}

		searchContent, _, _ := HandleMemSearch(context.Background(), "待删除", "", 10)
		if found(searchContent) {
			t.Errorf("expected '未找到' after delete, got: %s", searchContent)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, _, err := HandleMemDelete(context.Background(), 99999)
		if err == nil {
			t.Error("expected error for non-existent ID")
		}
	})
}

// === HandleMemGetObservation ==================================================

func TestHandleMemGetObservation(t *testing.T) {
	newTestDB(t)

	t.Run("success", func(t *testing.T) {
		id := saveMem(t, "完整查看测试", "这是完整的记忆内容，用于验证全文获取。", "test")
		content, _, err := HandleMemGetObservation(context.Background(), id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(content, "完整查看测试") {
			t.Errorf("expected title in content, got: %s", content)
		}
		if !strings.Contains(content, "这是完整的记忆内容") {
			t.Errorf("expected full body, got: %s", content)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, _, err := HandleMemGetObservation(context.Background(), 99999)
		if err == nil {
			t.Error("expected error for non-existent ID")
		}
	})
}

// === HandleMemStats ===========================================================

func TestHandleMemStats(t *testing.T) {
	newTestDB(t)

	t.Run("empty", func(t *testing.T) {
		content, _, err := HandleMemStats(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(content, "为空") {
			t.Errorf("expected '为空', got: %s", content)
		}
	})

	t.Run("with data", func(t *testing.T) {
		saveMem(t, "决策1", "内容", "decision")
		saveMem(t, "架构1", "内容", "architecture")
		saveMem(t, "Bug 1", "内容", "bugfix")

		content, _, err := HandleMemStats(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(content, "3 条") {
			t.Errorf("expected '3 条' in stats, got: %s", content)
		}
		if !strings.Contains(content, "最新记忆") {
			t.Errorf("expected '最新记忆', got: %s", content)
		}
	})
}

// === Integration: Full Memory Lifecycle ========================================

func TestMemoryLifecycle(t *testing.T) {
	newTestDB(t)

	// 1. Save
	id := saveMem(t, "集成测试记忆", "生命周期测试内容", "test")

	// 2. Search — should find it
	searchContent, _, err := HandleMemSearch(context.Background(), "集成测试", "", 10)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if !found(searchContent) {
		t.Fatalf("search should find saved memory: %s", searchContent)
	}

	// 3. Get observation
	obsContent, _, err := HandleMemGetObservation(context.Background(), id)
	if err != nil {
		t.Fatalf("get observation failed: %v", err)
	}
	if !strings.Contains(obsContent, "生命周期测试内容") {
		t.Errorf("expected full body: %s", obsContent)
	}

	// 4. Update
	_, _, err = HandleMemUpdate(context.Background(), id, "更新后标题", "更新后内容", "architecture")
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	// 5. Verify update
	obsContent2, _, err := HandleMemGetObservation(context.Background(), id)
	if err != nil {
		t.Fatalf("get observation after update failed: %v", err)
	}
	if !strings.Contains(obsContent2, "更新后标题") {
		t.Errorf("expected updated title: %s", obsContent2)
	}
	if !strings.Contains(obsContent2, "更新后内容") {
		t.Errorf("expected updated content: %s", obsContent2)
	}

	// 6. Stats
	statsContent, _, err := HandleMemStats(context.Background())
	if err != nil {
		t.Fatalf("stats failed: %v", err)
	}
	if !strings.Contains(statsContent, "architecture") {
		t.Errorf("expected architecture type in stats: %s", statsContent)
	}

	// 7. Delete
	_, _, err = HandleMemDelete(context.Background(), id)
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	// 8. Verify gone
	searchContent2, _, _ := HandleMemSearch(context.Background(), "集成测试", "", 10)
	if found(searchContent2) {
		t.Errorf("expected '未找到' after delete, got: %s", searchContent2)
	}
}

// === CJK Tokenization Detail ==================================================

func TestFTSChineseTokenization(t *testing.T) {
	newTestDB(t)

	saveMem(t, "中文测试",
		"dscli项目使用Go语言开发，支持SQLite FTS5全文搜索。中文分词效果需要验证。",
		"test")

	t.Run("exact phrase match", func(t *testing.T) {
		// "全文搜索" → gse: ["全文", "搜索"] → AND match
		content, _, err := HandleMemSearch(context.Background(), "全文搜索", "", 10)
		if err != nil {
			t.Fatalf("search failed: %v", err)
		}
		if !found(content) {
			t.Errorf("should find phrase '全文搜索': %s", content)
		}
	})

	t.Run("word match", func(t *testing.T) {
		// "分词" → gse produces "分词" as a word
		content, _, err := HandleMemSearch(context.Background(), "分词", "", 10)
		if err != nil {
			t.Fatalf("search failed: %v", err)
		}
		if !found(content) {
			t.Errorf("should find word '分词': %s", content)
		}
	})

	t.Run("non-consecutive AND search", func(t *testing.T) {
		// "中文" and "验证" both appear (not consecutively)
		content, _, err := HandleMemSearch(context.Background(), "中文 验证", "", 10)
		if err != nil {
			t.Fatalf("search failed: %v", err)
		}
		if !found(content) {
			t.Errorf("should find doc with both '中文' and '验证': %s", content)
		}
	})

	t.Run("nonexistent phrase", func(t *testing.T) {
		content, _, err := HandleMemSearch(context.Background(), "不存在词", "", 10)
		if err != nil {
			t.Fatalf("search failed: %v", err)
		}
		if found(content) {
			t.Errorf("should NOT find nonexistent phrase: %s", content)
		}
	})

	t.Run("multi-word CJK search", func(t *testing.T) {
		content, _, err := HandleMemSearch(context.Background(), "SQLite FTS5", "", 10)
		if err != nil {
			t.Fatalf("search failed: %v", err)
		}
		if !found(content) {
			t.Errorf("should find 'SQLite FTS5': %s", content)
		}
	})
}

// === Infrastructure ============================================================

func TestOpenDBIdempotent(t *testing.T) {
	newTestDB(t)

	db1, err := openDB()
	if err != nil {
		t.Fatalf("first openDB failed: %v", err)
	}
	defer db1.Close()

	db2, err := openDB()
	if err != nil {
		t.Fatalf("second openDB failed: %v", err)
	}
	defer db2.Close()

	var count1, count2 int
	if err := db1.QueryRow("SELECT COUNT(*) FROM memories").Scan(&count1); err != nil {
		t.Fatalf("query db1 failed: %v", err)
	}
	if err := db2.QueryRow("SELECT COUNT(*) FROM memories").Scan(&count2); err != nil {
		t.Fatalf("query db2 failed: %v", err)
	}
	if count1 != count2 {
		t.Errorf("expected same count: %d vs %d", count1, count2)
	}
}

func TestDBIsolation(t *testing.T) {
	dbPath := sqlite.GetDBPath()
	if !strings.Contains(dbPath, os.TempDir()) {
		t.Errorf("test DB should be in temp dir, got: %s", dbPath)
	}
	if strings.HasSuffix(dbPath, "sqlite.db") {
		t.Errorf("test DB should NOT use production name, got: %s", dbPath)
	}
}
