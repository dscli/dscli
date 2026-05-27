package session

import (
	"database/sql"
	"sync"
	"sync/atomic"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/sqlite"
)

func init() {
	sqlite.RegisterTableSchema(
		// 会话表
		`CREATE TABLE IF NOT EXISTS sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_path TEXT UNIQUE NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`)
}

var (
	currentSessionID atomic.Int64
	sessionOnce      sync.Once
)

// GetCurrentSessionID 获取当前会话ID（线程安全，仅初始化一次）
func GetCurrentSessionID(ctx context.Context) (sessionID int64) {
	sessionOnce.Do(func() {
		id, err := CreateOrGetSessionID(ctx)
		if err != nil {
			panic(err)
		}
		currentSessionID.Store(id)
	})
	return currentSessionID.Load()
}

// ResetSessionID clears the cached session ID so the next call to
// GetCurrentSessionID will re-resolve against the current database.
// Intended for tests that switch database paths.
func ResetSessionID() {
	sessionOnce = sync.Once{}
	currentSessionID.Store(0)
}

// CreateOrGetSessionID 获取或创建会话ID
func CreateOrGetSessionID(ctx context.Context) (sessionID int64, err error) {
	projectRoot := context.ProjectRoot
	if projectRoot == "" {
		panic("project root is empty, please set")
	}
	db, err := sqlite.OpenDB()
	if err != nil {
		return sessionID, err
	}
	defer db.Close()

	var id int64
	err = db.QueryRow("SELECT id FROM sessions WHERE project_path = ?",
		projectRoot).Scan(&id)
	if err != nil {
		if err != sql.ErrNoRows {
			return sessionID, err
		}
	} else if id > 0 {
		sessionID = id
		return sessionID, err
	}

	result, err := db.Exec("INSERT INTO sessions (project_path) VALUES (?)",
		projectRoot)
	if err != nil {
		return sessionID, err
	}

	id, err = result.LastInsertId()
	if err != nil {
		return sessionID, err
	}
	sessionID = id
	return sessionID, err
}

// ProjectRow represents a row from the sessions table.
type ProjectRow struct {
	ID          int64
	ProjectPath string
	CreatedAt   string
	Maintainer  string // e.g. "牛顿(Newton)" or empty
}

// ListProjects returns all sessions with their maintainer, ordered by ID.
func ListProjects() ([]ProjectRow, error) {
	db, err := sqlite.OpenDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT s.id, s.project_path, s.created_at,
		       COALESCE(a.name_cn || '(' || a.name_en || ')', '')
		FROM sessions s
		LEFT JOIN session_names sn ON s.id = sn.session_id
		LEFT JOIN ai_names a ON sn.name_id = a.id
		ORDER BY s.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ProjectRow
	for rows.Next() {
		var r ProjectRow
		if err := rows.Scan(&r.ID, &r.ProjectPath, &r.CreatedAt, &r.Maintainer); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}
