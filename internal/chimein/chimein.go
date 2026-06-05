package chimein

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/session"
	"github.com/dscli/dscli/internal/sqlite"
)

func init() {
	sqlite.RegisterTableSchema(
		`CREATE TABLE IF NOT EXISTS chimeins (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id INTEGER UNIQUE NOT NULL,
			content TEXT NOT NULL DEFAULT '',
			FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
		)`,
	)
}

// Append 追加内容到当前 session 的 chimein 行。
// 如果该 session 尚不存在对应行则创建，否则在已有内容后追加。
// 追加格式：原内容 + "\n" + newContent + "\n"
func Append(ctx context.Context, newContent string) error {
	sessionID := session.GetCurrentSessionID(ctx)
	db, err := sqlite.OpenDB()
	if err != nil {
		return err
	}
	defer db.Close()

	// 先获取已有内容
	existing, err := getContent(ctx, db, sessionID)
	if err != nil {
		// 不存在则创建
		if err == sql.ErrNoRows {
			content := "\n" + strings.TrimSpace(newContent) + "\n"
			_, insertErr := db.ExecContext(ctx,
				`INSERT INTO chimeins (session_id, content) VALUES (?, ?)`,
				sessionID, content)
			return insertErr
		}
		return err
	}

	// 追加内容
	content := existing + "\n" + strings.TrimSpace(newContent) + "\n"
	res, err := db.ExecContext(ctx,
		`UPDATE chimeins SET content = ? WHERE session_id = ?`,
		content, sessionID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("failed to append chimein content")
	}
	return nil
}

// Get 获取当前 session 的 chimein 内容，读取后自动清空。
// 如果不存在，返回空字符串和 nil error。
func Get(ctx context.Context) (string, error) {
	sessionID := session.GetCurrentSessionID(ctx)
	db, err := sqlite.OpenDB()
	if err != nil {
		return "", err
	}
	defer db.Close()

	content, err := getContent(ctx, db, sessionID)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}

	// 读到即为消费，自动清空。
	if content != "" {
		db.ExecContext(ctx,
			`UPDATE chimeins SET content = '' WHERE session_id = ?`,
			sessionID)
	}

	return content, nil
}

// getContent 内部函数：从指定 db 连接获取 content。
// 未找到时返回 sql.ErrNoRows。
func getContent(ctx context.Context, db *sqlite.DB, sessionID int64) (string, error) {
	var content string
	err := db.QueryRowContext(ctx,
		`SELECT content FROM chimeins WHERE session_id = ?`,
		sessionID).Scan(&content)
	return content, err
}
