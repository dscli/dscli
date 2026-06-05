// Package mail implements an inter-AI messaging system backed by SQLite.
//
// Architecture:
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
// Schema:
//
//	mail       — id, sender_name_id, recipient_name_id, subject, body, is_read, created_at
//	mail_fts   — FTS5 external content table over mail(subject, body)
//
// FTS sync is managed explicitly in Go (not via SQL triggers), so that
// Chinese content is tokenized with gse before insertion — ensuring the
// same tokenization on both index and query sides.
//
// Handlers:
//
//	HandleSendMail     — Send a mail to another maintainer by name
//	HandleReadMail     — Read mails for the current maintainer
//	HandleMailSearch   — FTS5 search across mails
//	HandleContacts     — List contacts with assigned projects
package mail

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/dscli/dscli/internal/ainame"
	"github.com/dscli/dscli/internal/sqlite"
	"github.com/dscli/dscli/internal/tokenizer"
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
			notified_at       DATETIME,
			FOREIGN KEY (sender_name_id)    REFERENCES ai_names(id),
			FOREIGN KEY (recipient_name_id) REFERENCES ai_names(id)
		)`,
		// Standalone FTS5 — content is tokenized and inserted manually in Go.
		// Do NOT use content='mail' (external content) because mail has no
		// single "content" column; it has subject and body. The external
		// content reference would cause "no such column: T.content" on rebuild
		// and SQLITE_CORRUPT_VTAB (267) on MATCH queries.
		`CREATE VIRTUAL TABLE IF NOT EXISTS mail_fts USING fts5(content)`,
	)

	sqlite.RegisterIndexSchema(
		`CREATE INDEX IF NOT EXISTS idx_mail_recipient ON mail(recipient_name_id, is_read, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_mail_sender    ON mail(sender_name_id, created_at DESC)`,
	)

	// Add notified_at column for existing databases.
	// Errors (e.g. column already exists on fresh install) are ignored —
	// the upgrade loop treats errors as non-fatal.
	sqlite.RegisterUpgradeSchema(
		`ALTER TABLE mail ADD COLUMN notified_at DATETIME`,
	)

	// Fix FTS5 table for existing databases that have the buggy external
	// content reference (content='mail' → non-existent mail.content column).
	// This hook runs once per binary version change.
	sqlite.RegisterPostInitHook(fixMailFTS)
}

// fixMailFTS drops and recreates the mail_fts table if it has the buggy
// external content reference (content='mail'). The bug: mail has subject
// and body columns, not a single "content" column. This caused
// SQLITE_CORRUPT_VTAB (267) on MATCH queries.
func fixMailFTS(db *sqlite.DB) error {
	var tableSQL string
	err := db.QueryRow("SELECT sql FROM sqlite_master WHERE type='table' AND name='mail_fts'").Scan(&tableSQL)
	if err != nil {
		return nil // table doesn't exist yet (clean install) — nothing to fix
	}
	if !strings.Contains(tableSQL, "content='mail'") {
		return nil // already fixed or new install
	}

	// Drop old FTS table (may be corrupted) and recreate without external content reference.
	if _, err := db.Exec("DROP TABLE IF EXISTS mail_fts"); err != nil {
		return fmt.Errorf("drop old mail_fts: %w", err)
	}
	if _, err := db.Exec("CREATE VIRTUAL TABLE IF NOT EXISTS mail_fts USING fts5(content)"); err != nil {
		return fmt.Errorf("create new mail_fts: %w", err)
	}

	// Reindex all existing mails (best effort — errors are non-fatal).
	// Collect rows first, then insert — inserting while iterating a query
	// on the same connection causes SQLITE_BUSY.
	rows, err := db.Query("SELECT id, subject, body FROM mail")
	if err != nil {
		return nil // no mails to reindex
	}

	type mailRec struct {
		id      int64
		subject string
		body    string
	}
	var mails []mailRec
	for rows.Next() {
		var m mailRec
		if err := rows.Scan(&m.id, &m.subject, &m.body); err != nil {
			continue
		}
		mails = append(mails, m)
	}
	rows.Close()

	for _, m := range mails {
		// Best-effort reindex; individual failures are non-fatal.
		if ftsErr := insertMailFTS(db, m.id, m.subject, m.body); ftsErr != nil {
			_ = ftsErr
		}
	}
	return nil
}

// === Types =====================================================================

// MailRow represents a single mail message.
type MailRow struct {
	ID             int64  `json:"id"`
	SenderName     string `json:"sender_name"`
	SenderEmail    string `json:"sender_email"`
	RecipientName  string `json:"recipient_name"`
	RecipientEmail string `json:"recipient_email"`
	Subject        string `json:"subject"`
	Body           string `json:"body"`
	IsRead         bool   `json:"is_read"`
	CreatedAt      string `json:"created_at"`
}

// MaintainerRow represents a maintainer from ai_names.
type MaintainerRow struct {
	ID       int64  `json:"id"`
	NameCN   string `json:"name_cn"`
	NameEN   string `json:"name_en"`
	BirdFrog string `json:"bird_frog"`
	Email    string `json:"email"`
}

// === FTS Sync ==================================================================

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

// === Handlers ==================================================================

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

	senderNameID := ainame.GetCurrentNameID(ctx)

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
			return "", "", fmt.Errorf("未知的接收者: %s（请使用 contacts 查看可用名字）", recipient)
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

// HandleReadMail reads a single mail by ID for the current maintainer.
// Also marks the mail as read.
func HandleReadMail(ctx context.Context, mailID int64) (result, warning string, err error) {
	if mailID <= 0 {
		return "", "", fmt.Errorf("mail ID is required")
	}

	nameID := ainame.GetCurrentNameID(ctx)

	db, err := sqlite.OpenDB()
	if err != nil {
		return "", "", fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

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

// HandleListMail lists recent mails for the current maintainer, showing
// subject only (no body). Optional unreadOnly filter.
func HandleListMail(ctx context.Context, unreadOnly bool, limit int) (result, warning string, err error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	nameID := ainame.GetCurrentNameID(ctx)

	db, err := sqlite.OpenDB()
	if err != nil {
		return "", "", fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

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
	fmt.Fprintf(&sb, "📬 **%s** (%d 封)\n\n", status, len(mails))

	for _, m := range mails {
		readMark := " "
		if !m.IsRead {
			readMark = "●"
		}
		subject := m.Subject
		if subject == "" {
			subject = "(无主题)"
		}

		fmt.Fprintf(&sb, "%s **#%d** | %s → %s | %s\n",
			readMark, m.ID, m.SenderName, m.RecipientName, localTime(m.CreatedAt))
		fmt.Fprintf(&sb, "  主题: %s\n", subject)
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

	nameID := ainame.GetCurrentNameID(ctx)

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
	var unreadCount int
	_ = db.QueryRow("SELECT COUNT(*) FROM mail WHERE recipient_name_id = ? AND is_read = 0", nameID).Scan(&unreadCount)

	var sb strings.Builder
	fmt.Fprintf(&sb, "🔍 **搜索结果**: \"%s\" (%d 封)", query, len(mails))
	if unreadCount > 0 {
		fmt.Fprintf(&sb, " | 📬 未读: %d", unreadCount)
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

		fmt.Fprintf(&sb, "**#%d** | %s → %s | %s\n", m.ID, m.SenderName, m.RecipientName, localTime(m.CreatedAt))
		fmt.Fprintf(&sb, "  主题: %s\n", shortSubject)
		if shortBody != "" {
			fmt.Fprintf(&sb, "  内容: %s\n", shortBody)
		}
		sb.WriteString("\n")
	}

	return sb.String(), "", nil
}

// HandleContacts lists contacts that have been assigned to at least one project,
// along with their project assignments.
func HandleContacts(ctx context.Context) (result, warning string, err error) {
	currentNameID := ainame.GetCurrentNameID(ctx)

	db, err := sqlite.OpenDB()
	if err != nil {
		return "", "", fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	rows, err := db.Query(
		`SELECT an.id, an.name_cn, an.name_en, an.bird_frog, an.email,
		        s.id, s.project_path
		 FROM ai_names an
		 JOIN session_names sn ON sn.name_id = an.id
		 JOIN sessions s ON s.id = sn.session_id
		 ORDER BY an.id, s.id`,
	)
	if err != nil {
		return "", "", fmt.Errorf("查询联系人失败: %w", err)
	}
	defer rows.Close()

	// Group by ai_name to collect project assignments.
	type projInfo struct {
		id   int64
		name string
	}
	type contactInfo struct {
		row      MaintainerRow
		projects []projInfo
	}
	contactMap := make(map[int64]*contactInfo)
	var contactOrder []int64

	for rows.Next() {
		var m MaintainerRow
		var sID int64
		var projPath string
		if err := rows.Scan(&m.ID, &m.NameCN, &m.NameEN, &m.BirdFrog, &m.Email, &sID, &projPath); err != nil {
			continue
		}
		c, ok := contactMap[m.ID]
		if !ok {
			c = &contactInfo{row: m}
			contactMap[m.ID] = c
			contactOrder = append(contactOrder, m.ID)
		}
		projName := filepath.Base(projPath)
		c.projects = append(c.projects, projInfo{id: sID, name: projName})
	}

	if len(contactOrder) == 0 {
		return "没有找到有项目的联系人。", "", nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "👥 **通讯录** (%d 人)\n\n", len(contactOrder))

	var unreadCount int
	_ = db.QueryRow("SELECT COUNT(*) FROM mail WHERE recipient_name_id = ? AND is_read = 0", currentNameID).Scan(&unreadCount)

	for _, id := range contactOrder {
		c := contactMap[id]
		marker := " "
		if c.row.ID == currentNameID {
			marker = "→"
		}
		birdFrog := "🐸"
		if c.row.BirdFrog == "bird" {
			birdFrog = "🐦"
		}
		var projParts []string
		for _, p := range c.projects {
			projParts = append(projParts, fmt.Sprintf("%s(%d)", p.name, p.id))
		}
		projStr := strings.Join(projParts, ", ")

		fmt.Fprintf(&sb, "%s %s **%s** (%s) <%s>\n    working on %s\n",
			marker, birdFrog, c.row.NameEN, c.row.NameCN, c.row.Email, projStr)
	}

	if unreadCount > 0 {
		fmt.Fprintf(&sb, "\n📬 你有 %d 封未读邮件，使用 readmail 查看。\n", unreadCount)
	}

	return sb.String(), "", nil
}

// HandleReplyMail replies to an existing mail.
// The recipient is automatically set to the original mail's sender.
// If subject is empty, "Re: <original subject>" is used.
func HandleReplyMail(ctx context.Context, replyToID int64, subject, body string) (result, warning string, err error) {
	if replyToID <= 0 {
		err = fmt.Errorf("replyToID is required")
		return
	}
	if subject == "" && body == "" {
		err = fmt.Errorf("subject or body is required")
		return
	}

	senderNameID := ainame.GetCurrentNameID(ctx)

	db, err := sqlite.OpenDB()
	if err != nil {
		return "", "", fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Look up original mail — must be addressed to current user
	var origSenderNameID int64
	var origSubject, origBody string
	err = db.QueryRow(
		`SELECT sender_name_id, subject, body FROM mail
		 WHERE id = ? AND recipient_name_id = ?`,
		replyToID, senderNameID,
	).Scan(&origSenderNameID, &origSubject, &origBody)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", "", fmt.Errorf("邮件 #%d 不存在或不属于你", replyToID)
		}
		return "", "", fmt.Errorf("查询原邮件失败: %w", err)
	}

	// Default subject: Re: <original>
	if subject == "" {
		if strings.HasPrefix(origSubject, "Re: ") {
			subject = origSubject
		} else {
			subject = "Re: " + origSubject
		}
	}

	// Look up original sender's name/email
	var recipientNameEN, recipientEmail string
	_ = db.QueryRow("SELECT name_en, email FROM ai_names WHERE id = ?", origSenderNameID).Scan(&recipientNameEN, &recipientEmail)

	result2, err := db.Exec(
		`INSERT INTO mail (sender_name_id, recipient_name_id, subject, body) VALUES (?, ?, ?, ?)`,
		senderNameID, origSenderNameID, subject, body,
	)
	if err != nil {
		return "", "", fmt.Errorf("回复失败: %w", err)
	}

	mailID, _ := result2.LastInsertId()

	if ftsErr := insertMailFTS(db, mailID, subject, body); ftsErr != nil {
		warning = fmt.Sprintf("⚠️ FTS 索引失败: %v", ftsErr)
	}

	var senderNameEN string
	_ = db.QueryRow("SELECT name_en FROM ai_names WHERE id = ?", senderNameID).Scan(&senderNameEN)

	result = fmt.Sprintf("✅ 回复已发送 (#%d)\n发件人: %s\n收件人: %s <%s>\n回复: #%d\n主题: %s",
		mailID, senderNameEN, recipientNameEN, recipientEmail, replyToID, subject)
	return result, warning, nil
}

// HandleDeleteMail deletes a mail by ID. Only the recipient (current user)
// can delete their own mails.
func HandleDeleteMail(ctx context.Context, mailID int64) (result, warning string, err error) {
	if mailID <= 0 {
		err = fmt.Errorf("mail ID is required")
		return
	}

	nameID := ainame.GetCurrentNameID(ctx)

	db, err := sqlite.OpenDB()
	if err != nil {
		return "", "", fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Verify ownership: only the recipient can delete
	var subject string
	err = db.QueryRow(
		`SELECT subject FROM mail WHERE id = ? AND recipient_name_id = ?`,
		mailID, nameID,
	).Scan(&subject)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", "", fmt.Errorf("邮件 #%d 不存在或不属于你", mailID)
		}
		return "", "", fmt.Errorf("查询邮件失败: %w", err)
	}

	res, err := db.Exec(`DELETE FROM mail WHERE id = ? AND recipient_name_id = ?`, mailID, nameID)
	if err != nil {
		return "", "", fmt.Errorf("删除邮件失败: %w", err)
	}

	rows, _ := res.RowsAffected()
	if rows == 0 {
		return "", "", fmt.Errorf("邮件 #%d 不存在或不属于你", mailID)
	}

	// Remove from FTS index
	if ftsErr := deleteMailFTS(db, mailID); ftsErr != nil {
		warning = fmt.Sprintf("⚠️ FTS 清理失败: %v", ftsErr)
	}

	result = fmt.Sprintf("✅ 邮件已删除: #%d %q", mailID, subject)
	return result, warning, nil
}

// === Helpers ===================================================================

// localTime converts a UTC datetime string ("2006-01-02 15:04:05") to local time.
// If parsing fails, the original string is returned as-is.
func localTime(utcStr string) string {
	t, err := time.Parse("2006-01-02 15:04:05", utcStr)
	if err != nil {
		return utcStr
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

// UnreadMailCount returns the number of unread mails for the current maintainer.
// Returns 0 on any error (missing session, DB error, etc.) — errors are silently
// swallowed because this is a prompt hint; it must never block or error out.
func UnreadMailCount(ctx context.Context) int {
	nameID := ainame.GetCurrentNameID(ctx)
	if nameID == 0 {
		return 0
	}

	db, err := sqlite.OpenDB()
	if err != nil {
		return 0
	}
	defer db.Close()

	var count int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM mail WHERE recipient_name_id = ? AND is_read = 0 AND notified_at IS NULL",
		nameID,
	).Scan(&count); err != nil {
		return 0
	}
	return count
}

// UnreadMailSummary is a lightweight view of an unread mail for notification.
type UnreadMailSummary struct {
	ID         int64
	SenderName string
	Subject    string
}

// UnreadMailList returns unread mail summaries for the current maintainer.
// Returns nil on any error — errors are silently swallowed because this is
// a prompt hint; it must never block or error out.
func UnreadMailList(ctx context.Context) []UnreadMailSummary {
	nameID := ainame.GetCurrentNameID(ctx)
	if nameID == 0 {
		return nil
	}

	db, err := sqlite.OpenDB()
	if err != nil {
		return nil
	}
	defer db.Close()

	rows, err := db.Query(
		`SELECT m.id, s.name_en, m.subject
		 FROM mail m
		 JOIN ai_names s ON s.id = m.sender_name_id
		 WHERE m.recipient_name_id = ? AND m.is_read = 0 AND m.notified_at IS NULL
		 ORDER BY m.created_at DESC
		 LIMIT 10`,
		nameID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var summaries []UnreadMailSummary
	for rows.Next() {
		var s UnreadMailSummary
		if err := rows.Scan(&s.ID, &s.SenderName, &s.Subject); err != nil {
			continue
		}
		summaries = append(summaries, s)
	}

	// Mark returned mails as notified so they won't appear again in future
	// notifications. Errors are silently ignored — notification is best-effort.
	for _, s := range summaries {
		_, _ = db.Exec(`UPDATE mail SET notified_at = CURRENT_TIMESTAMP WHERE id = ?`, s.ID)
	}

	return summaries
}

// FormatUnreadMailNotification formats unread mail summaries into a
// user-message notification. Returns empty string when there are no
// summaries — caller should skip injection in that case.
func FormatUnreadMailNotification(summaries []UnreadMailSummary) string {
	if len(summaries) == 0 {
		return ""
	}

	var sb strings.Builder
	word := "messages"
	if len(summaries) == 1 {
		word = "message"
	}

	sb.WriteString("## ⚠️ UNREAD MAIL — Action Required\n\n")
	fmt.Fprintf(&sb, "You have **%d unread %s**. ", len(summaries), word)
	sb.WriteString("Call `readmail` **before responding** — ")
	sb.WriteString("these may contain decisions or questions that affect your task.\n\n")
	sb.WriteString("| ID | From | Subject |\n")
	sb.WriteString("|----|------|--------|\n")

	for _, s := range summaries {
		subject := s.Subject
		if subject == "" {
			subject = "(no subject)"
		}
		// Keep subject concise for the table
		if len(subject) > 60 {
			subject = subject[:60] + "..."
		}
		fmt.Fprintf(&sb, "| %d | %s | %s |\n", s.ID, s.SenderName, subject)
	}

	return sb.String()
}

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
		localTime(row.CreatedAt),
		row.Subject,
		row.Body,
	)
}
