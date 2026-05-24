// Package mail implements an inter-AI messaging system backed by SQLite.
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Architecture
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// The mail system is split into two layers:
//
//   - internal/mail          — Core domain logic (this package)
//   - internal/toolcall/mail — LLM tool registration & argument parsing
//
// Mail enables explicit communication between AI maintainers. Each AI is
// identified by a name_id (from ai_names). Senders are determined from the
// current session via ainame.GetNameID(). Recipients are looked up by
// case-insensitive name or email.
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Schema
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
//	mail       — id, sender_name_id, recipient_name_id, subject, body, is_read, created_at
//	mail_fts   — FTS5 external content table over mail(subject, body)
//
// FTS sync is managed explicitly in Go (not via SQL triggers), so that
// Chinese content is tokenized with gse before insertion — ensuring the
// same tokenization on both index and query sides.
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Handlers
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
//	HandleSendMail     — Send a mail to another maintainer by name
//	HandleReadMail     — Read mails for the current maintainer
//	HandleMailSearch   — FTS5 search across mails
//	HandleMaintainers  — List all known maintainer names
package mail

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"gitcode.com/dscli/dscli/internal/ainame"
	"gitcode.com/dscli/dscli/internal/session"
	"gitcode.com/dscli/dscli/internal/sqlite"
	"gitcode.com/dscli/dscli/internal/tokenizer"
)

func init() {
	sqlite.RegisterTableSchema(
		`CREATE TABLE IF NOT EXISTS mail (
			id                INTEGER PRIMARY KEY AUTOINCREMENT,
			sender_name_id    INTEGER NOT NULL DEFAULT 0,
			recipient_name_id INTEGER NOT NULL DEFAULT 0,
			subject           TEXT NOT NULL DEFAULT '',
			body              TEXT NOT NULL DEFAULT '',
			is_read           INTEGER NOT NULL DEFAULT 0,
			created_at        DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (sender_name_id)    REFERENCES ai_names(id),
			FOREIGN KEY (recipient_name_id) REFERENCES ai_names(id)
		)`,
		// External content FTS5 — sync managed manually in Go.
		`CREATE VIRTUAL TABLE IF NOT EXISTS mail_fts USING fts5(
			content, content='mail', content_rowid='id'
		)`,
	)

	sqlite.RegisterIndexSchema(
		`CREATE INDEX IF NOT EXISTS idx_mail_recipient ON mail(recipient_name_id, is_read, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_mail_sender    ON mail(sender_name_id, created_at DESC)`,
	)
}

// ─── Types ─────────────────────────────────────────────────────────────────────

// MailRow represents a single mail message.
type MailRow struct {
	ID              int64  `json:"id"`
	SenderName      string `json:"sender_name"`
	SenderEmail     string `json:"sender_email"`
	RecipientName   string `json:"recipient_name"`
	RecipientEmail  string `json:"recipient_email"`
	Subject         string `json:"subject"`
	Body            string `json:"body"`
	IsRead          bool   `json:"is_read"`
	CreatedAt       string `json:"created_at"`
}

// MaintainerRow represents a maintainer from ai_names.
type MaintainerRow struct {
	ID       int64  `json:"id"`
	NameCN   string `json:"name_cn"`
	NameEN   string `json:"name_en"`
	BirdFrog string `json:"bird_frog"`
	Email    string `json:"email"`
}

// ─── FTS Sync ──────────────────────────────────────────────────────────────────

// insertMailFTS tokenizes subject+body and inserts into mail_fts.
func insertMailFTS(db *sqlite.DB, id int64, subject, body string) error {
	tokens := tokenizer.Tokenize(subject + " " + body)
	if tokens == "" {
		// At minimum, insert the ID so the FTS row exists.
		tokens = fmt.Sprintf("mail%d", id)
	}
	_, err := db.Exec("INSERT INTO mail_fts(rowid, content) VALUES (?, ?)", id, tokens)
	return err
}

// deleteMailFTS removes a mail from the FTS index.
func deleteMailFTS(db *sqlite.DB, id int64) error {
	_, err := db.Exec("INSERT INTO mail_fts(mail_fts, rowid, content) VALUES('delete', ?, ?)", id, "")
	return err
}

// updateMailFTS removes old FTS entry and inserts new one.
func updateMailFTS(db *sqlite.DB, id int64, subject, body string) error {
	if err := deleteMailFTS(db, id); err != nil {
		return err
	}
	return insertMailFTS(db, id, subject, body)
}

// ─── Handlers ──────────────────────────────────────────────────────────────────

// HandleSendMail sends a mail to a recipient identified by name or email.
func HandleSendMail(ctx context.Context, recipient, subject, body string) (result, warning string, err error) {
	if recipient == "" {
		err = fmt.Errorf("recipient is required")
		return
	}
	if subject == "" && body == "" {
		err = fmt.Errorf("subject or body is required")
		return
	}

	sessionID := session.GetCurrentSessionID(ctx)
	senderNameID := ainame.GetNameID(sessionID)

	db, err := sqlite.OpenDB()
	if err != nil {
		return "", "", fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Look up recipient by name_en or email (case insensitive).
	var recipientNameID int64
	var recipientNameEN, recipientEmail string
	err = db.QueryRow(
		`SELECT id, name_en, email FROM ai_names
		 WHERE LOWER(name_en) = LOWER(?) OR LOWER(email) = LOWER(?)
		 LIMIT 1`,
		recipient, recipient,
	).Scan(&recipientNameID, &recipientNameEN, &recipientEmail)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", "", fmt.Errorf("未知的接收者: %s（请使用 maintainers 查看可用名字）", recipient)
		}
		return "", "", fmt.Errorf("查询接收者失败: %w", err)
	}

	if recipientNameID == senderNameID && senderNameID != 0 {
		warning = "你正在给自己发邮件 — 也许你想发给其他人？"
	}

	result2, err := db.Exec(
		`INSERT INTO mail (sender_name_id, recipient_name_id, subject, body) VALUES (?, ?, ?, ?)`,
		senderNameID, recipientNameID, subject, body,
	)
	if err != nil {
		return "", "", fmt.Errorf("发送邮件失败: %w", err)
	}

	mailID, _ := result2.LastInsertId()

	// Sync to FTS with tokenized content
	if ftsErr := insertMailFTS(db, mailID, subject, body); ftsErr != nil {
		// Non-fatal — mail is saved, FTS will catch up on next search (or manual rebuild)
		warning = fmt.Sprintf("%s\n⚠️ FTS 索引失败: %v", warning, ftsErr)
	}

	var senderNameEN string
	_ = db.QueryRow("SELECT name_en FROM ai_names WHERE id = ?", senderNameID).Scan(&senderNameEN)
	if senderNameEN == "" {
		senderNameEN = "nobody"
	}

	result = fmt.Sprintf("✅ 邮件已发送 (#%d)\n发件人: %s\n收件人: %s <%s>\n主题: %s",
		mailID, senderNameEN, recipientNameEN, recipientEmail, subject)
	return result, warning, nil
}

// HandleReadMail reads mail for the current maintainer.
// mailID > 0: read specific mail; mailID == 0: list mails (with optional unreadOnly filter).
func HandleReadMail(ctx context.Context, mailID int64, unreadOnly bool, limit int) (result, warning string, err error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	sessionID := session.GetCurrentSessionID(ctx)
	nameID := ainame.GetNameID(sessionID)

	db, err := sqlite.OpenDB()
	if err != nil {
		return "", "", fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Read a single mail by ID
	if mailID > 0 {
		var row MailRow
		var isRead int
		err = db.QueryRow(
			`SELECT m.id, s.name_en, s.email, r.name_en, r.email, m.subject, m.body, m.is_read, m.created_at
			 FROM mail m
			 JOIN ai_names s ON s.id = m.sender_name_id
			 JOIN ai_names r ON r.id = m.recipient_name_id
			 WHERE m.id = ? AND m.recipient_name_id = ?`,
			mailID, nameID,
		).Scan(&row.ID, &row.SenderName, &row.SenderEmail,
			&row.RecipientName, &row.RecipientEmail,
			&row.Subject, &row.Body, &isRead, &row.CreatedAt)
		if err != nil {
			if err == sql.ErrNoRows {
				return "", "", fmt.Errorf("邮件 #%d 不存在或不属于你", mailID)
			}
			return "", "", fmt.Errorf("读取邮件失败: %w", err)
		}
		row.IsRead = isRead != 0

		// Mark as read (no FTS update needed — subject/body unchanged)
		_, _ = db.Exec("UPDATE mail SET is_read = 1 WHERE id = ?", mailID)

		result = formatMailRow(row)
		return result, warning, nil
	}

	// List mails
	query := `SELECT m.id, s.name_en, s.email, r.name_en, r.email, m.subject, m.body, m.is_read, m.created_at
		 FROM mail m
		 JOIN ai_names s ON s.id = m.sender_name_id
		 JOIN ai_names r ON r.id = m.recipient_name_id
		 WHERE m.recipient_name_id = ?`
	var args []any
	args = append(args, nameID)

	if unreadOnly {
		query += " AND m.is_read = 0"
	}

	query += " ORDER BY m.created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.Query(query, args...)
	if err != nil {
		return "", "", fmt.Errorf("查询邮件失败: %w", err)
	}
	defer rows.Close()

	var mails []MailRow
	for rows.Next() {
		var row MailRow
		var isRead int
		if err := rows.Scan(&row.ID, &row.SenderName, &row.SenderEmail,
			&row.RecipientName, &row.RecipientEmail,
			&row.Subject, &row.Body, &isRead, &row.CreatedAt); err != nil {
			continue
		}
		row.IsRead = isRead != 0
		mails = append(mails, row)
	}

	if len(mails) == 0 {
		if unreadOnly {
			return "📭 没有未读邮件。", "", nil
		}
		return "📭 收件箱为空。", "", nil
	}

	var sb strings.Builder
	status := "收件箱"
	if unreadOnly {
		status = "未读邮件"
	}
	sb.WriteString(fmt.Sprintf("📬 **%s** (%d 封)\n\n", status, len(mails)))

	for _, m := range mails {
		readMark := " "
		if !m.IsRead {
			readMark = "●"
		}
		shortSubject := m.Subject
		if len(shortSubject) > 60 {
			shortSubject = shortSubject[:60] + "..."
		}
		if shortSubject == "" {
			shortSubject = "(无主题)"
		}
		shortBody := m.Body
		if len(shortBody) > 120 {
			shortBody = shortBody[:120] + "..."
		}

		sb.WriteString(fmt.Sprintf("%s **#%d** | %s → %s | %s\n",
			readMark, m.ID, m.SenderName, m.RecipientName, m.CreatedAt))
		sb.WriteString(fmt.Sprintf("  主题: %s\n", shortSubject))
		if shortBody != "" {
			sb.WriteString(fmt.Sprintf("  内容: %s\n", shortBody))
		}
		sb.WriteString("\n")
	}

	return sb.String(), "", nil
}

// HandleMailSearch searches mails using FTS5.
func HandleMailSearch(ctx context.Context, query string, limit int) (result, warning string, err error) {
	if query == "" {
		err = fmt.Errorf("query is required")
		return
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}

	db, err := sqlite.OpenDB()
	if err != nil {
		return "", "", fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Sanitize query using the same tokenizer as indexing
	sanitized := tokenizer.SanitizeFTS(query)
	if sanitized == "" {
		return fmt.Sprintf("⚠️ 搜索词 \"%s\" 不包含有效内容，请尝试其他关键词。", query), "", nil
	}

	rows, err := db.Query(
		`SELECT m.id, s.name_en, s.email, r.name_en, r.email, m.subject, m.body, m.is_read, m.created_at
		 FROM mail_fts f
		 JOIN mail m ON m.id = f.rowid
		 JOIN ai_names s ON s.id = m.sender_name_id
		 JOIN ai_names r ON r.id = m.recipient_name_id
		 WHERE mail_fts MATCH ?
		 ORDER BY rank
		 LIMIT ?`,
		sanitized, limit,
	)
	if err != nil {
		return "", "", fmt.Errorf("搜索邮件失败: %w", err)
	}
	defer rows.Close()

	var mails []MailRow
	for rows.Next() {
		var row MailRow
		var isRead int
		if err := rows.Scan(&row.ID, &row.SenderName, &row.SenderEmail,
			&row.RecipientName, &row.RecipientEmail,
			&row.Subject, &row.Body, &isRead, &row.CreatedAt); err != nil {
			continue
		}
		row.IsRead = isRead != 0
		mails = append(mails, row)
	}

	if len(mails) == 0 {
		return fmt.Sprintf("🔍 没有找到与 \"%s\" 相关的邮件。", query), "", nil
	}

	// Get total unread count for the current maintainer
	sessionID := session.GetCurrentSessionID(ctx)
	nameID := ainame.GetNameID(sessionID)
	var unreadCount int
	_ = db.QueryRow("SELECT COUNT(*) FROM mail WHERE recipient_name_id = ? AND is_read = 0", nameID).Scan(&unreadCount)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔍 **搜索结果**: \"%s\" (%d 封)", query, len(mails)))
	if unreadCount > 0 {
		sb.WriteString(fmt.Sprintf(" | 📬 未读: %d", unreadCount))
	}
	sb.WriteString("\n\n")

	for _, m := range mails {
		shortSubject := m.Subject
		if len(shortSubject) > 60 {
			shortSubject = shortSubject[:60] + "..."
		}
		if shortSubject == "" {
			shortSubject = "(无主题)"
		}
		shortBody := m.Body
		if len(shortBody) > 100 {
			shortBody = shortBody[:100] + "..."
		}

		sb.WriteString(fmt.Sprintf("**#%d** | %s → %s | %s\n", m.ID, m.SenderName, m.RecipientName, m.CreatedAt))
		sb.WriteString(fmt.Sprintf("  主题: %s\n", shortSubject))
		if shortBody != "" {
			sb.WriteString(fmt.Sprintf("  内容: %s\n", shortBody))
		}
		sb.WriteString("\n")
	}

	return sb.String(), "", nil
}

// HandleMaintainers lists all maintainers from ai_names.
func HandleMaintainers(ctx context.Context) (result, warning string, err error) {
	db, err := sqlite.OpenDB()
	if err != nil {
		return "", "", fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	rows, err := db.Query(
		`SELECT id, name_cn, name_en, bird_frog, email FROM ai_names ORDER BY id`,
	)
	if err != nil {
		return "", "", fmt.Errorf("查询维护者失败: %w", err)
	}
	defer rows.Close()

	sessionID := session.GetCurrentSessionID(ctx)
	currentNameID := ainame.GetNameID(sessionID)

	var maintainers []MaintainerRow
	for rows.Next() {
		var m MaintainerRow
		if err := rows.Scan(&m.ID, &m.NameCN, &m.NameEN, &m.BirdFrog, &m.Email); err != nil {
			continue
		}
		maintainers = append(maintainers, m)
	}

	if len(maintainers) == 0 {
		return "没有找到维护者。", "", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("👥 **维护者列表** (%d 人)\n\n", len(maintainers)))

	var unreadCount int
	_ = db.QueryRow("SELECT COUNT(*) FROM mail WHERE recipient_name_id = ? AND is_read = 0", currentNameID).Scan(&unreadCount)

	for _, m := range maintainers {
		marker := " "
		if m.ID == currentNameID {
			marker = "→"
		}
		birdFrog := "🐸"
		if m.BirdFrog == "bird" {
			birdFrog = "🐦"
		}
		sb.WriteString(fmt.Sprintf("%s %s **%s** (%s) <%s>\n",
			marker, birdFrog, m.NameEN, m.NameCN, m.Email))
	}

	if unreadCount > 0 {
		sb.WriteString(fmt.Sprintf("\n📬 你有 %d 封未读邮件，使用 readmail 查看。\n", unreadCount))
	}

	return sb.String(), "", nil
}

// ─── Helpers ───────────────────────────────────────────────────────────────────

func formatMailRow(row MailRow) string {
	readStatus := "已读"
	if !row.IsRead {
		readStatus = "未读"
	}

	return fmt.Sprintf(`📧 **邮件 #%d** [%s]
发件人: %s <%s>
收件人: %s <%s>
时间: %s
主题: %s

%s`,
		row.ID, readStatus,
		row.SenderName, row.SenderEmail,
		row.RecipientName, row.RecipientEmail,
		row.CreatedAt,
		row.Subject,
		row.Body,
	)
}
