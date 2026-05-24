package mail

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"gitcode.com/dscli/dscli/internal/ainame"
	"gitcode.com/dscli/dscli/internal/session"
	"gitcode.com/dscli/dscli/internal/sqlite"
)

func newTestDB(t *testing.T) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "mail-test.db")
	sqlite.SetDBPath(dbPath)
}

// currentName returns the name_en of the current session's assigned name,
// or "Newton" as a fallback if the session can't be determined.
func currentName() string {
	sessionID := session.GetCurrentSessionID(context.Background())
	cfg := ainame.LoadOrAssign(sessionID)
	if cfg != nil && cfg.NameEN != "" {
		return cfg.NameEN
	}
	return "Newton"
}

// ─── HandleSendMail ───────────────────────────────────────────────────────────

func TestHandleSendMail(t *testing.T) {
	newTestDB(t)
	me := currentName()

	t.Run("success", func(t *testing.T) {
		result, _, err := HandleSendMail(context.Background(), me, "测试主题", "测试正文")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "邮件已发送") {
			t.Errorf("expected '邮件已发送', got: %s", result)
		}
	})

	t.Run("send by email", func(t *testing.T) {
		email := strings.ToLower(me) + "@dscli.io"
		result, _, err := HandleSendMail(context.Background(), email, "Email主题", "正文")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "邮件已发送") {
			t.Errorf("expected '邮件已发送', got: %s", result)
		}
	})

	t.Run("case insensitive recipient", func(t *testing.T) {
		result, _, err := HandleSendMail(context.Background(), strings.ToUpper(me), "大小写测试", "正文")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "邮件已发送") {
			t.Errorf("expected '邮件已发送', got: %s", result)
		}
	})

	t.Run("empty recipient", func(t *testing.T) {
		_, _, err := HandleSendMail(context.Background(), "", "主题", "正文")
		if err == nil {
			t.Error("expected error for empty recipient")
		}
	})

	t.Run("empty subject and body", func(t *testing.T) {
		_, _, err := HandleSendMail(context.Background(), me, "", "")
		if err == nil {
			t.Error("expected error for empty subject and body")
		}
	})

	t.Run("unknown recipient", func(t *testing.T) {
		_, _, err := HandleSendMail(context.Background(), "NonExistentName", "主题", "正文")
		if err == nil {
			t.Error("expected error for unknown recipient")
		}
	})

	t.Run("subject only", func(t *testing.T) {
		result, _, err := HandleSendMail(context.Background(), me, "纯主题邮件", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "邮件已发送") {
			t.Errorf("expected '邮件已发送', got: %s", result)
		}
	})

	t.Run("body only", func(t *testing.T) {
		result, _, err := HandleSendMail(context.Background(), me, "", "纯正文邮件")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "邮件已发送") {
			t.Errorf("expected '邮件已发送', got: %s", result)
		}
	})
}

// ─── HandleReadMail ───────────────────────────────────────────────────────────

func TestHandleReadMail(t *testing.T) {
	newTestDB(t)
	me := currentName()

	t.Run("empty inbox", func(t *testing.T) {
		result, _, err := HandleReadMail(context.Background(), 0, false, 20)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "为空") {
			t.Errorf("expected empty inbox, got: %s", result)
		}
	})

	t.Run("list mails after send", func(t *testing.T) {
		HandleSendMail(context.Background(), me, "列表测试", "正文内容")
		HandleSendMail(context.Background(), me, "第二封", "更多内容")

		result, _, err := HandleReadMail(context.Background(), 0, false, 20)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "列表测试") {
			t.Errorf("expected '列表测试', got: %s", result)
		}
		if !strings.Contains(result, "第二封") {
			t.Errorf("expected '第二封', got: %s", result)
		}
	})

	t.Run("unread filter", func(t *testing.T) {
		result, _, err := HandleReadMail(context.Background(), 0, true, 20)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// After previous read (which marks all as read), there may be none unread.
		// But list_mails_after_send didn't mark them as read (HandleReadMail with id=0
		// does NOT mark as read — only single-mail read does). So they should still
		// be unread.
		if !strings.Contains(result, "列表测试") && !strings.Contains(result, "没有未读邮件") {
			t.Errorf("expected unread mails or empty unread, got: %s", result)
		}
	})

	t.Run("read single mail", func(t *testing.T) {
		sendResult, _, err := HandleSendMail(context.Background(), me, "单封读取", "这是单封邮件的内容全文")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var mid int64
		fmt.Sscanf(sendResult, "✅ 邮件已发送 (#%d)", &mid)

		result, _, err := HandleReadMail(context.Background(), mid, false, 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "单封读取") {
			t.Errorf("expected '单封读取', got: %s", result)
		}
		if !strings.Contains(result, "这是单封邮件的内容全文") {
			t.Errorf("expected full body, got: %s", result)
		}
	})

	t.Run("non existent mail", func(t *testing.T) {
		_, _, err := HandleReadMail(context.Background(), 99999, false, 10)
		if err == nil {
			t.Error("expected error for non-existent mail")
		}
	})

	t.Run("limit", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			HandleSendMail(context.Background(), me, fmt.Sprintf("限制测试 %d", i), "正文")
		}
		result, _, err := HandleReadMail(context.Background(), 0, false, 2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "2 封") {
			t.Errorf("expected '2 封', got: %s", result)
		}
	})
}

// ─── HandleMailSearch ─────────────────────────────────────────────────────────

func TestHandleMailSearch(t *testing.T) {
	newTestDB(t)
	me := currentName()

	HandleSendMail(context.Background(), me, "JWT认证实现", "关于基于RS256的JWT实现方案讨论")
	HandleSendMail(context.Background(), me, "Bug修复报告", "修复了登录超时的bug，当token过期时正确处理401")
	HandleSendMail(context.Background(), me, "中文搜索测试", "FTS5对中文的处理方式是按字分词，需要验证搜索效果")
	HandleSendMail(context.Background(), me, "Mixed测试", "This is a mixed language test 中英文混合内容")

	t.Run("basic search", func(t *testing.T) {
		result, _, err := HandleMailSearch(context.Background(), "JWT", 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "JWT") {
			t.Errorf("expected 'JWT' in results, got: %s", result)
		}
	})

	t.Run("chinese search", func(t *testing.T) {
		result, _, err := HandleMailSearch(context.Background(), "分词", 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "中文搜索测试") {
			t.Errorf("expected '中文搜索测试' in results, got: %s", result)
		}
	})

	t.Run("not found", func(t *testing.T) {
		result, _, err := HandleMailSearch(context.Background(), "不存在的关键词xyz999", 10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "没有找到") {
			t.Errorf("expected '没有找到', got: %s", result)
		}
	})

	t.Run("empty query", func(t *testing.T) {
		_, _, err := HandleMailSearch(context.Background(), "", 10)
		if err == nil {
			t.Error("expected error for empty query")
		}
	})

	t.Run("limit results", func(t *testing.T) {
		result, _, err := HandleMailSearch(context.Background(), "测试", 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "(1 封)") {
			t.Errorf("expected '(1 封)', got: %s", result)
		}
	})
}

// ─── HandleMaintainers ────────────────────────────────────────────────────────

func TestHandleMaintainers(t *testing.T) {
	newTestDB(t)

	t.Run("list maintainers", func(t *testing.T) {
		result, _, err := HandleMaintainers(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "维护者列表") {
			t.Errorf("expected '维护者列表', got: %s", result)
		}
		if !strings.Contains(result, "Newton") {
			t.Errorf("expected 'Newton' in maintainers, got: %s", result)
		}
		if !strings.Contains(result, "Bohr") {
			t.Errorf("expected 'Bohr' in maintainers, got: %s", result)
		}
	})
}

// ─── Integration: Full Mail Lifecycle ─────────────────────────────────────────

func TestMailLifecycle(t *testing.T) {
	newTestDB(t)
	me := currentName()

	// 1. Send mail to self
	sendResult, _, err := HandleSendMail(context.Background(), me, "集成测试邮件", "这是集成测试的邮件正文。")
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}
	var mid int64
	fmt.Sscanf(sendResult, "✅ 邮件已发送 (#%d)", &mid)
	if mid == 0 {
		t.Fatal("failed to extract mail ID")
	}

	// 2. Read the specific mail (marks as read)
	result, _, err := HandleReadMail(context.Background(), mid, false, 10)
	if err != nil {
		t.Fatalf("read single failed: %v", err)
	}
	if !strings.Contains(result, "这是集成测试的邮件正文") {
		t.Errorf("expected full body: %s", result)
	}

	// 3. Search
	result, _, err = HandleMailSearch(context.Background(), "集成测试", 10)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if !strings.Contains(result, "集成测试邮件") {
		t.Errorf("expected mail in search: %s", result)
	}

	// 4. After reading, unread should be empty
	result, _, err = HandleReadMail(context.Background(), 0, true, 10)
	if err != nil {
		t.Fatalf("read unread after marking failed: %v", err)
	}
	if !strings.Contains(result, "没有未读邮件") {
		t.Errorf("expected no unread mail, got: %s", result)
	}
}
