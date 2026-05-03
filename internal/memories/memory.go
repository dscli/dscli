// Package memories implements persistent memory tools (mem_save, mem_update, mem_search,
// mem_delete, mem_get_observation, mem_stats) backed by SQLite FTS5 for full-text search.
//
// Architecture:
//   - memories table: stores observations with title, content, type
//   - memories_fts: FTS5 virtual table for full-text search (standalone, no content=)
//   - Chinese text is tokenized via internal/tokenizer (gse CutSearch) before indexing
package memories

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/session"
	"gitcode.com/dscli/dscli/internal/sqlite"
	"gitcode.com/dscli/dscli/internal/tokenizer"
)

// ─── SQLite Schema (registered via init) ──────────────────────────────────────

func init() {
	sqlite.RegisterTableSchema(
		`CREATE TABLE IF NOT EXISTS memories (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id INTEGER NOT NULL,
			title      TEXT    NOT NULL,
			content    TEXT    NOT NULL,
			type       TEXT    NOT NULL DEFAULT 'manual',
			created_at DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
			FOREIGN KEY (session_id) REFERENCES sessions(id)
		)`,
		// Standalone FTS table (no content= option).
		// CJK characters are space-separated before insert so each char
		// becomes an independent token, enabling substring search.
		`CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
			title, content, type
		)`,
	)
	sqlite.RegisterIndexSchema(
		`CREATE INDEX IF NOT EXISTS idx_memories_type ON memories(type)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_created ON memories(created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_memories_updated ON memories(updated_at DESC)`,
	)
	// 升级脚本：为旧数据库添加 session_id 列（RegisterUpgradeSchema 静默忽略重复错误）
	sqlite.RegisterUpgradeSchema(
		`ALTER TABLE memories ADD COLUMN session_id INTEGER NOT NULL DEFAULT 0`,
		`CREATE INDEX IF NOT EXISTS idx_memories_session_id ON memories(session_id)`,
	)
}

// ─── Shared Helpers ───────────────────────────────────────────────────────────

// openDB opens the shared dscli database.
func openDB() (*sql.DB, error) {
	return sqlite.OpenDB()
}

// truncate shortens s to max runes, appending "..." if truncated.
func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "..."
}

// memoryRow is a single memory record.
type memoryRow struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Type      string `json:"type"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// searchRow is a memoryRow with FTS rank.
type searchRow struct {
	memoryRow
	Rank float64 `json:"rank"`
}

// ─── FTS Sync Helpers ─────────────────────────────────────────────────────────

// insertFTS inserts a row into the FTS index with Chinese tokenization.
func insertFTS(db *sql.DB, id int64, title, content, typ string) error {
	_, err := db.Exec(
		`INSERT INTO memories_fts(rowid, title, content, type) VALUES (?, ?, ?, ?)`,
		id, tokenizer.Tokenize(title), tokenizer.Tokenize(content), typ,
	)
	return err
}

// deleteFTS removes a row from the FTS index by rowid.
func deleteFTS(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM memories_fts WHERE rowid = ?`, id)
	return err
}

// ─── Handlers ─────────────────────────────────────────────────────────────────

// HandleMemSave saves a new memory observation.
func HandleMemSave(ctx context.Context, title string, body string, typ string) (result string, suggest string, err error) {
	if title == "" || body == "" {
		err = fmt.Errorf("title 和 content 为必填项")
		return
	}

	db, err := openDB()
	if err != nil {
		err = fmt.Errorf("打开数据库失败: %w", err)
		return
	}
	defer db.Close()

	sessionID := session.GetCurrentSessionID(ctx)

	// Truncate content if too long (>50000 chars)
	if len(body) > 50000 {
		body = body[:50000] + "... [截断]"
	}

	res, err := db.Exec(
		`INSERT INTO memories (session_id, title, content, type) VALUES (?, ?, ?, ?)`,
		sessionID, title, body, typ,
	)
	if err != nil {
		err = fmt.Errorf("保存记忆失败: %w", err)
		return
	}

	id, _ := res.LastInsertId()

	// Sync FTS index with space-separated CJK content
	if ftsErr := insertFTS(db, id, title, body, typ); ftsErr != nil {
		err = fmt.Errorf("创建全文索引失败: %w", ftsErr)
		return
	}

	outfmt.Printf("Memory saved: #%d %q (%s)\n", id, title, typ)

	result = fmt.Sprintf("✅ 记忆已保存: #%d\n标题: %s\n类型: %s\n时间: %s",
		id, title, typ, time.Now().Format("2006-01-02 15:04:05"))
	return
}

// HandleMemUpdate updates an existing memory by ID.
func HandleMemUpdate(ctx context.Context, id int64, title string, body string, typ string) (result string, suggest string, err error) {
	db, err := openDB()
	if err != nil {
		err = fmt.Errorf("打开数据库失败: %w", err)
		return
	}
	defer db.Close()

	sessionID := session.GetCurrentSessionID(ctx)

	// Verify the memory exists and belongs to current project
	var existing memoryRow
	err = db.QueryRow(
		`SELECT id, title, content, type, created_at, updated_at FROM memories
		 WHERE id = ? AND session_id = ?`, id, sessionID,
	).Scan(&existing.ID, &existing.Title, &existing.Content, &existing.Type,
		&existing.CreatedAt, &existing.UpdatedAt)
	if err == sql.ErrNoRows {
		err = fmt.Errorf("记忆 #%d 不存在或不属于当前项目", id)
		return
	}
	if err != nil {
		err = fmt.Errorf("查询记忆失败: %w", err)
		return
	}

	var sets []string
	var vals []any

	newTitle := existing.Title
	newBody := existing.Content
	newType := existing.Type

	if title != "" {
		sets = append(sets, "title = ?")
		vals = append(vals, title)
		newTitle = title
	}
	if body != "" {
		if len(body) > 50000 {
			body = body[:50000] + "... [截断]"
		}
		sets = append(sets, "content = ?")
		vals = append(vals, body)
		newBody = body
	}
	if typ != "" {
		sets = append(sets, "type = ?")
		vals = append(vals, typ)
		newType = typ
	}

	sets = append(sets, "updated_at = datetime('now')")
	vals = append(vals, id, sessionID)

	sql := fmt.Sprintf("UPDATE memories SET %s WHERE id = ? AND session_id = ?", strings.Join(sets, ", "))
	_, err = db.Exec(sql, vals...)
	if err != nil {
		err = fmt.Errorf("更新记忆失败: %w", err)
		return
	}

	// Rebuild FTS entry: delete old, insert new with tokenized content
	if ftsErr := deleteFTS(db, id); ftsErr != nil {
		err = fmt.Errorf("删除旧全文索引失败: %w", ftsErr)
		return
	}
	if ftsErr := insertFTS(db, id, newTitle, newBody, newType); ftsErr != nil {
		err = fmt.Errorf("重建全文索引失败: %w", ftsErr)
		return
	}

	outfmt.Printf("Memory updated: #%d %q\n", id, existing.Title)
	result = fmt.Sprintf("✅ 记忆已更新: #%d\n原标题: %s", id, existing.Title)
	return
}

// HandleMemSearch searches memories using FTS5 full-text search.
func HandleMemSearch(ctx context.Context, query string, typ string, limit int) (result string, suggest string, err error) {
	db, err := openDB()
	if err != nil {
		err = fmt.Errorf("打开数据库失败: %w", err)
		return
	}
	defer db.Close()

	sessionID := session.GetCurrentSessionID(ctx)

	ftsQuery := tokenizer.SanitizeFTS(query)

	sqlQ := `
		SELECT m.id, m.title, m.content, m.type, m.created_at, m.updated_at, fts.rank
		FROM memories_fts fts
		JOIN memories m ON m.id = fts.rowid
		WHERE memories_fts MATCH ?
		  AND m.session_id = ?
	`
	vals := []any{ftsQuery, sessionID}

	if typ != "" {
		sqlQ += " AND m.type = ?"
		vals = append(vals, typ)
	}

	sqlQ += " ORDER BY fts.rank LIMIT ?"
	vals = append(vals, limit)

	rows, err := db.Query(sqlQ, vals...)
	if err != nil {
		err = fmt.Errorf("搜索失败: %w（提示：尝试用更简单的关键词）", err)
		return
	}
	defer rows.Close()

	var results []searchRow
	for rows.Next() {
		var r searchRow
		if err = rows.Scan(&r.ID, &r.Title, &r.Content, &r.Type,
			&r.CreatedAt, &r.UpdatedAt, &r.Rank); err != nil {
			return "", "", fmt.Errorf("扫描结果失败: %w", err)
		}
		results = append(results, r)
	}
	if err = rows.Err(); err != nil {
		return "", "", fmt.Errorf("搜索结果遍历失败: %w", err)
	}

	if len(results) == 0 {
		result = fmt.Sprintf("🔍 未找到匹配的记忆: %q", query)
		return
	}

	var b strings.Builder
	fmt.Fprintf(&b, "🔍 找到 %d 条记忆:\n\n", len(results))
	for i, r := range results {
		preview := truncate(r.Content, 300)
		hasMore := len(r.Content) > 300
		fmt.Fprintf(&b, "[%d] #%d [%s] %s\n", i+1, r.ID, r.Type, r.Title)
		fmt.Fprintf(&b, "    %s", preview)
		if hasMore {
			fmt.Fprintf(&b, " [预览]")
		}
		fmt.Fprintf(&b, "\n    %s | 相关性: %.2f\n\n", r.CreatedAt, r.Rank)
	}
	if len(results) > 0 {
		hasTruncated := false
		for _, r := range results {
			if len(r.Content) > 300 {
				hasTruncated = true
				break
			}
		}
		if hasTruncated {
			b.WriteString("---\n以上为预览（300字符）。使用 mem_get_observation 工具可查看完整内容。\n")
		}
	}

	result = b.String()
	return
}

// HandleMemDelete deletes a memory by ID.
func HandleMemDelete(ctx context.Context, id int64) (result string, suggest string, err error) {
	db, err := openDB()
	if err != nil {
		err = fmt.Errorf("打开数据库失败: %w", err)
		return
	}
	defer db.Close()

	sessionID := session.GetCurrentSessionID(ctx)

	// Verify existence and ownership first for a meaningful error message
	var title string
	err = db.QueryRow(`SELECT title FROM memories WHERE id = ? AND session_id = ?`, id, sessionID).Scan(&title)
	if err == sql.ErrNoRows {
		err = fmt.Errorf("记忆 #%d 不存在或不属于当前项目", id)
		return
	}
	if err != nil {
		err = fmt.Errorf("查询记忆失败: %w", err)
		return
	}

	res, err := db.Exec(`DELETE FROM memories WHERE id = ? AND session_id = ?`, id, sessionID)
	if err != nil {
		err = fmt.Errorf("删除记忆失败: %w", err)
		return
	}

	rows, _ := res.RowsAffected()
	if rows == 0 {
		err = fmt.Errorf("记忆 #%d 不存在或不属于当前项目", id)
		return
	}

	// Remove from FTS index
	if err = deleteFTS(db, id); err != nil {
		// Non-fatal: the memories row is already gone
		outfmt.Debug("memory: FTS cleanup for #%d failed: %v\n", id, err)
	}

	outfmt.Printf("Memory deleted: #%d %q\n", id, title)
	result = fmt.Sprintf("✅ 记忆已删除: #%d %q", id, title)
	return
}

// HandleMemGetObservation retrieves full memory content by ID.
// Unlike mem_search which returns truncated previews, this returns the complete content.
func HandleMemGetObservation(ctx context.Context, id int64) (result string, suggest string, err error) {

	db, err := openDB()
	if err != nil {
		err = fmt.Errorf("打开数据库失败: %w", err)
		return
	}
	defer db.Close()

	sessionID := session.GetCurrentSessionID(ctx)

	var m memoryRow
	err = db.QueryRow(
		`SELECT id, title, content, type, created_at, updated_at FROM memories WHERE id = ? AND session_id = ?`, id, sessionID,
	).Scan(&m.ID, &m.Title, &m.Content, &m.Type, &m.CreatedAt, &m.UpdatedAt)
	if err == sql.ErrNoRows {
		err = fmt.Errorf("记忆 #%d 不存在或不属于当前项目", id)
		return
	}
	if err != nil {
		err = fmt.Errorf("查询记忆失败: %w", err)
		return
	}

	result = fmt.Sprintf("#%d [%s] %s\n\n%s\n\n创建: %s | 更新: %s",
		m.ID, m.Type, m.Title, m.Content, m.CreatedAt, m.UpdatedAt)
	return
}

// HandleMemStats returns memory system statistics.
func HandleMemStats(ctx context.Context) (result string, suggest string, err error) {
	db, err := openDB()
	if err != nil {
		err = fmt.Errorf("打开数据库失败: %w", err)
		return
	}
	defer db.Close()

	sessionID := session.GetCurrentSessionID(ctx)

	var total int64
	err = db.QueryRow(`SELECT COUNT(*) FROM memories WHERE session_id = ?`, sessionID).Scan(&total)
	if err != nil {
		err = fmt.Errorf("统计失败: %w", err)
		return
	}

	if total == 0 {
		result = "📊 记忆系统为空，还没有任何记忆。"
		return
	}

	// Type distribution
	rows, err := db.Query(`SELECT type, COUNT(*) FROM memories WHERE session_id = ? GROUP BY type ORDER BY COUNT(*) DESC`, sessionID)
	if err != nil {
		err = fmt.Errorf("类型统计失败: %w", err)
		return
	}
	defer rows.Close()

	var b strings.Builder
	fmt.Fprintf(&b, "📊 记忆统计: %d 条\n\n类型分布:\n", total)
	for rows.Next() {
		var typ string
		var count int64
		if err = rows.Scan(&typ, &count); err != nil {
			return "", "", fmt.Errorf("扫描失败: %w", err)
		}
		fmt.Fprintf(&b, "  %-15s %d\n", typ, count)
	}
	if err = rows.Err(); err != nil {
		return "", "", fmt.Errorf("遍历失败: %w", err)
	}

	// Latest entry
	var latest memoryRow
	err = db.QueryRow(
		`SELECT id, title, type, created_at FROM memories WHERE session_id = ? ORDER BY created_at DESC LIMIT 1`, sessionID,
	).Scan(&latest.ID, &latest.Title, &latest.Type, &latest.CreatedAt)
	if err == nil {
		fmt.Fprintf(&b, "\n最新记忆: #%d [%s] %q (%s)", latest.ID, latest.Type, latest.Title, latest.CreatedAt)
	}

	result = b.String()
	return
}