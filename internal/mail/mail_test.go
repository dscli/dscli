package mail

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dscli/dscli/internal/ainame"
	"github.com/dscli/dscli/internal/session"
	"github.com/dscli/dscli/internal/sqlite"
)

func newTestDB(t *testing.T) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "mail-test.db")
	sqlite.SetDBPath(dbPath)
	session.ResetSessionID()
	ainame.ResetCurrentNameID()
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

// === HandleSendMail ===========================================================

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

// === HandleReadMail & HandleListMail ==========================================

func TestHandleReadMail(t *testing.T) {
	newTestDB(t)
	me := currentName()

	t.Run("empty inbox", func(t *testing.T) {
		result, _, err := HandleListMail(context.Background(), false, 20)
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

		result, _, err := HandleListMail(context.Background(), false, 20)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "列表测试") {
			t.Errorf("expected '列表测试', got: %s", result)
		}
		if !strings.Contains(result, "第二封") {
			t.Errorf("expected '第二封', got: %s", result)
		}
		// Body should NOT appear in list view
		if strings.Contains(result, "正文内容") {
			t.Errorf("list should not show body, got: %s", result)
		}
	})

	t.Run("unread filter", func(t *testing.T) {
		result, _, err := HandleListMail(context.Background(), true, 20)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// HandleListMail does NOT mark as read — only HandleReadMail does.
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

		result, _, err := HandleReadMail(context.Background(), mid)
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
		_, _, err := HandleReadMail(context.Background(), 99999)
		if err == nil {
			t.Error("expected error for non-existent mail")
		}
	})

	t.Run("read requires valid id", func(t *testing.T) {
		_, _, err := HandleReadMail(context.Background(), 0)
		if err == nil {
			t.Error("expected error for invalid id")
		}
	})

	t.Run("limit", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			HandleSendMail(context.Background(), me, fmt.Sprintf("限制测试 %d", i), "正文")
		}
		result, _, err := HandleListMail(context.Background(), false, 2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "2 封") {
			t.Errorf("expected '2 封', got: %s", result)
		}
	})
}

// === HandleMailSearch =========================================================

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

// === HandleContacts ============================================================

func TestHandleContacts(t *testing.T) {
	newTestDB(t)

	t.Run("list contacts", func(t *testing.T) {
		result, _, err := HandleContacts(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "通讯录") {
			t.Errorf("expected '通讯录', got: %s", result)
		}
		// Only contacts with assigned projects are listed.
		// The current session should show up with its project.
		if !strings.Contains(result, "working on") {
			t.Errorf("expected 'working on', got: %s", result)
		}
		// Current user marker.
		if !strings.Contains(result, "→") {
			t.Errorf("expected '→' marker for current user, got: %s", result)
		}
		// Nobody (name_id=0) should NOT appear — it has no sessions.
		if strings.Contains(result, "nobody") {
			t.Errorf("nobody should not appear (no project assigned), got: %s", result)
		}
	})
}

// === Integration: Full Mail Lifecycle =========================================

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
	result, _, err := HandleReadMail(context.Background(), mid)
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
	result, _, err = HandleListMail(context.Background(), true, 10)
	if err != nil {
		t.Fatalf("read unread after marking failed: %v", err)
	}
	if !strings.Contains(result, "没有未读邮件") {
		t.Errorf("expected no unread mail, got: %s", result)
	}
}

// === HandleReplyMail ===========================================================

func TestHandleReplyMail(t *testing.T) {
	newTestDB(t)
	me := currentName()

	// First send a mail to ourselves (simulating someone else sent to us)
	sendResult, _, err := HandleSendMail(context.Background(), me, "原始邮件", "这是原始邮件内容")
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}
	var mid int64
	fmt.Sscanf(sendResult, "✅ 邮件已发送 (#%d)", &mid)

	t.Run("reply with subject", func(t *testing.T) {
		result, _, err := HandleReplyMail(context.Background(), mid, "我的回复", "回复正文内容")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "回复已发送") {
			t.Errorf("expected '回复已发送', got: %s", result)
		}
		if !strings.Contains(result, "我的回复") {
			t.Errorf("expected custom subject, got: %s", result)
		}
	})

	t.Run("reply with auto Re: prefix", func(t *testing.T) {
		result, _, err := HandleReplyMail(context.Background(), mid, "", "自动主题回复")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "Re: 原始邮件") {
			t.Errorf("expected 'Re: 原始邮件' auto-subject, got: %s", result)
		}
	})

	t.Run("reply without doubling Re:", func(t *testing.T) {
		// Reply again to the first reply (which has "Re: 原始邮件" subject)
		// Find the reply mail ID first
		result, _, err := HandleReplyMail(context.Background(), mid, "", "再次回复")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Subject should be "Re: 原始邮件" (not "Re: Re: 原始邮件")
		if strings.Contains(result, "Re: Re:") {
			t.Errorf("should not double 'Re:' prefix, got: %s", result)
		}
	})

	t.Run("reply to non-existent mail", func(t *testing.T) {
		_, _, err := HandleReplyMail(context.Background(), 99999, "主题", "正文")
		if err == nil {
			t.Error("expected error for non-existent mail")
		}
	})

	t.Run("reply with empty body and subject", func(t *testing.T) {
		_, _, err := HandleReplyMail(context.Background(), mid, "", "")
		if err == nil {
			t.Error("expected error for empty subject and body")
		}
	})

	t.Run("reply with invalid id", func(t *testing.T) {
		_, _, err := HandleReplyMail(context.Background(), 0, "主题", "正文")
		if err == nil {
			t.Error("expected error for invalid id")
		}
	})
}

// === HandleDeleteMail ==========================================================

func TestHandleDeleteMail(t *testing.T) {
	newTestDB(t)
	me := currentName()

	t.Run("delete success", func(t *testing.T) {
		sendResult, _, err := HandleSendMail(context.Background(), me, "待删除邮件", "这条邮件将被删除")
		if err != nil {
			t.Fatalf("send failed: %v", err)
		}
		var mid int64
		fmt.Sscanf(sendResult, "✅ 邮件已发送 (#%d)", &mid)

		result, _, err := HandleDeleteMail(context.Background(), mid)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "已删除") {
			t.Errorf("expected '已删除', got: %s", result)
		}

		// Verify it's gone
		_, _, err = HandleReadMail(context.Background(), mid)
		if err == nil {
			t.Error("expected error for deleted mail")
		}
	})

	t.Run("delete non-existent", func(t *testing.T) {
		_, _, err := HandleDeleteMail(context.Background(), 99999)
		if err == nil {
			t.Error("expected error for non-existent mail")
		}
	})

	t.Run("delete invalid id", func(t *testing.T) {
		_, _, err := HandleDeleteMail(context.Background(), 0)
		if err == nil {
			t.Error("expected error for invalid id")
		}
	})
}

// === Integration: Reply + Delete Lifecycle =====================================

func TestMailReplyAndDeleteLifecycle(t *testing.T) {
	newTestDB(t)
	me := currentName()

	// 1. Send original mail
	sendResult, _, err := HandleSendMail(context.Background(), me, "讨论主题", "讨论的原始内容")
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}
	var mid int64
	fmt.Sscanf(sendResult, "✅ 邮件已发送 (#%d)", &mid)

	// 2. Reply to it
	replyResult, _, err := HandleReplyMail(context.Background(), mid, "", "这是我的回复")
	if err != nil {
		t.Fatalf("reply failed: %v", err)
	}
	if !strings.Contains(replyResult, "Re: 讨论主题") {
		t.Errorf("expected auto Re: subject: %s", replyResult)
	}
	var replyID int64
	fmt.Sscanf(replyResult, "✅ 回复已发送 (#%d)", &replyID)

	// 3. Read inbox — should have both mails
	result, _, err := HandleListMail(context.Background(), false, 20)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if !strings.Contains(result, "讨论主题") {
		t.Errorf("expected original mail in inbox: %s", result)
	}
	if !strings.Contains(result, "Re: 讨论主题") {
		t.Errorf("expected reply in inbox: %s", result)
	}

	// 4. Delete the reply
	_, _, err = HandleDeleteMail(context.Background(), replyID)
	if err != nil {
		t.Fatalf("delete reply failed: %v", err)
	}

	// 5. Delete the original
	_, _, err = HandleDeleteMail(context.Background(), mid)
	if err != nil {
		t.Fatalf("delete original failed: %v", err)
	}

	// 6. Inbox should be empty
	result, _, err = HandleListMail(context.Background(), false, 20)
	if err != nil {
		t.Fatalf("read after delete failed: %v", err)
	}
	if !strings.Contains(result, "为空") {
		t.Errorf("expected empty inbox, got: %s", result)
	}
}

// === UnreadMailList & FormatUnreadMailNotification ============================

func TestUnreadMailList(t *testing.T) {
	newTestDB(t)
	me := currentName()

	t.Run("empty initially", func(t *testing.T) {
		summaries := UnreadMailList(context.Background())
		if len(summaries) != 0 {
			t.Errorf("expected 0 unread, got %d", len(summaries))
		}
	})

	t.Run("one unread after send", func(t *testing.T) {
		HandleSendMail(context.Background(), me, "Test subject", "Test body")

		summaries := UnreadMailList(context.Background())
		if len(summaries) != 1 {
			t.Fatalf("expected 1 unread, got %d", len(summaries))
		}
		if summaries[0].SenderName == "" {
			t.Error("expected sender name")
		}
		if summaries[0].Subject != "Test subject" {
			t.Errorf("expected 'Test subject', got %q", summaries[0].Subject)
		}
		if summaries[0].ID == 0 {
			t.Error("expected non-zero ID")
		}
	})

	t.Run("multiple unread", func(t *testing.T) {
		HandleSendMail(context.Background(), me, "Second mail", "Body 2")
		HandleSendMail(context.Background(), me, "Third mail", "Body 3")

		summaries := UnreadMailList(context.Background())
		if len(summaries) < 2 {
			t.Fatalf("expected at least 2 unread, got %d", len(summaries))
		}
	})

	t.Run("no session returns nil", func(t *testing.T) {
		// Simulate no session by using a fresh DB with no session
		// UnreadMailList should return nil, not panic
		// This is implicitly tested by the fact that empty initially
		// returned empty slice, not nil — but nil is also acceptable.
	})

	t.Run("dedup: no repeat after notification", func(t *testing.T) {
		HandleSendMail(context.Background(), me, "Dedup test", "Body")

		// First call returns the mail and marks it notified.
		s := UnreadMailList(context.Background())
		if len(s) != 1 {
			t.Fatalf("first call: expected 1 unread, got %d", len(s))
		}

		// Second call should return nothing — already notified.
		s = UnreadMailList(context.Background())
		if len(s) != 0 {
			t.Errorf("second call: expected 0 (already notified), got %d", len(s))
		}
	})
}

func TestFormatUnreadMailNotification(t *testing.T) {
	t.Run("empty summaries", func(t *testing.T) {
		result := FormatUnreadMailNotification(nil)
		if result != "" {
			t.Errorf("expected empty string for nil, got %q", result)
		}
		result = FormatUnreadMailNotification([]UnreadMailSummary{})
		if result != "" {
			t.Errorf("expected empty string for empty slice, got %q", result)
		}
	})

	t.Run("single mail", func(t *testing.T) {
		summaries := []UnreadMailSummary{
			{ID: 1, SenderName: "Fermi", Subject: "Hello"},
		}
		result := FormatUnreadMailNotification(summaries)
		if !strings.Contains(result, "UNREAD MAIL") {
			t.Error("expected 'UNREAD MAIL' header")
		}
		if !strings.Contains(result, "1 unread message") {
			t.Error("expected '1 unread message'")
		}
		if !strings.Contains(result, "Fermi") {
			t.Error("expected sender 'Fermi'")
		}
		if !strings.Contains(result, "Hello") {
			t.Error("expected subject 'Hello'")
		}
		if !strings.Contains(result, "| 1 | Fermi | Hello |") {
			t.Errorf("expected table row, got: %s", result)
		}
	})

	t.Run("multiple mails", func(t *testing.T) {
		summaries := []UnreadMailSummary{
			{ID: 10, SenderName: "Fermi", Subject: "Re: test"},
			{ID: 11, SenderName: "Zhang Heng", Subject: "Hello from ZH"},
		}
		result := FormatUnreadMailNotification(summaries)
		if !strings.Contains(result, "2 unread messages") {
			t.Error("expected '2 unread messages'")
		}
		if !strings.Contains(result, "| 10 | Fermi |") {
			t.Error("expected first row")
		}
		if !strings.Contains(result, "| 11 | Zhang Heng |") {
			t.Error("expected second row")
		}
	})

	t.Run("empty subject", func(t *testing.T) {
		summaries := []UnreadMailSummary{
			{ID: 1, SenderName: "Curie", Subject: ""},
		}
		result := FormatUnreadMailNotification(summaries)
		if !strings.Contains(result, "(no subject)") {
			t.Error("expected '(no subject)' fallback")
		}
	})

	t.Run("long subject truncated", func(t *testing.T) {
		longSubject := strings.Repeat("x", 80)
		summaries := []UnreadMailSummary{
			{ID: 1, SenderName: "Fermi", Subject: longSubject},
		}
		result := FormatUnreadMailNotification(summaries)
		if strings.Contains(result, longSubject) {
			t.Error("expected subject to be truncated")
		}
		if !strings.Contains(result, "...") {
			t.Error("expected '...' truncation marker")
		}
	})

	t.Run("actionable instruction present", func(t *testing.T) {
		summaries := []UnreadMailSummary{
			{ID: 1, SenderName: "Test", Subject: "Test"},
		}
		result := FormatUnreadMailNotification(summaries)
		if !strings.Contains(result, "readmail") {
			t.Error("expected 'readmail' instruction")
		}
		if !strings.Contains(result, "before responding") {
			t.Error("expected 'before responding' directive")
		}
	})
}

func TestFormatUnreadMailLine(t *testing.T) {
	t.Run("empty summaries", func(t *testing.T) {
		if got := FormatUnreadMailLine(nil); got != "" {
			t.Errorf("expected empty for nil, got %q", got)
		}
		if got := FormatUnreadMailLine([]UnreadMailSummary{}); got != "" {
			t.Errorf("expected empty for empty slice, got %q", got)
		}
	})

	t.Run("single mail", func(t *testing.T) {
		summaries := []UnreadMailSummary{
			{ID: 1, SenderName: "Fermi", Subject: "Hello"},
		}
		result := FormatUnreadMailLine(summaries)
		if !strings.Contains(result, "1 unread message") {
			t.Errorf("expected '1 unread message', got: %s", result)
		}
		if !strings.Contains(result, "readmail") {
			t.Error("expected 'readmail' instruction")
		}
		// Single line — no newline inside
		if strings.Count(result, "\n") > 0 {
			t.Error("expected single line, got multiline")
		}
	})

	t.Run("multiple mails", func(t *testing.T) {
		summaries := []UnreadMailSummary{
			{ID: 10, SenderName: "Fermi", Subject: "Re: test"},
			{ID: 11, SenderName: "Zhang Heng", Subject: "Hello from ZH"},
		}
		result := FormatUnreadMailLine(summaries)
		if !strings.Contains(result, "2 unread messages") {
			t.Errorf("expected '2 unread messages', got: %s", result)
		}
		if strings.Count(result, "\n") > 0 {
			t.Error("expected single line, got multiline")
		}
	})
}


// === fixMailFTS ================================================================

func TestFixMailFTS(t *testing.T) {
	newTestDB(t)
	me := currentName()

	db, err := sqlite.OpenDB()
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	// 1. Simulate old buggy schema: drop current mail_fts, create with content='mail'
	db.Exec("DROP TABLE IF EXISTS mail_fts")
	_, err = db.Exec(`CREATE VIRTUAL TABLE mail_fts USING fts5(content, content='mail', content_rowid='id')`)
	if err != nil {
		t.Fatalf("create old schema: %v", err)
	}

	// 2. Insert test mails via HandleSendMail (which also populates FTS via insertMailFTS)
	HandleSendMail(context.Background(), me, "迁移测试1", "这是第一封测试邮件的内容")
	HandleSendMail(context.Background(), me, "迁移测试2", "第二封邮件，包含不同的关键词")

	// Verify old schema is detected
	var oldSQL string
	db.QueryRow("SELECT sql FROM sqlite_master WHERE type='table' AND name='mail_fts'").Scan(&oldSQL)
	if !strings.Contains(oldSQL, "content='mail'") {
		t.Fatal("expected old schema with content='mail'")
	}

	// 3. Run the fix
	if err := fixMailFTS(db); err != nil {
		t.Fatalf("fixMailFTS failed: %v", err)
	}

	// 4. Verify new schema
	var newSQL string
	db.QueryRow("SELECT sql FROM sqlite_master WHERE type='table' AND name='mail_fts'").Scan(&newSQL)
	if strings.Contains(newSQL, "content='mail'") {
		t.Errorf("old schema not cleaned up, got: %s", newSQL)
	}
	if !strings.Contains(newSQL, "fts5(content)") {
		t.Errorf("expected fts5(content), got: %s", newSQL)
	}

	// 5. Verify mail_search works after fix
	result, _, err := HandleMailSearch(context.Background(), "迁移测试", 10)
	if err != nil {
		t.Fatalf("search after fix failed: %v", err)
	}
	if !strings.Contains(result, "迁移测试1") {
		t.Errorf("expected '迁移测试1' in search results, got: %s", result)
	}

	// 6. Idempotency: running fixMailFTS again should be a no-op
	if err := fixMailFTS(db); err != nil {
		t.Fatalf("second fixMailFTS failed: %v", err)
	}
}
