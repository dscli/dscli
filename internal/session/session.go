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

var currentSessionID atomic.Int64
var sessionOnce sync.Once

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

// CreateOrGetSessionID 获取或创建会话ID
func CreateOrGetSessionID(ctx context.Context) (sessionID int64, err error) {
	projectRoot := context.ProjectRoot
	if projectRoot == "" {
		panic("project root is empty, please set")
	}
	db, err := sqlite.OpenDB()
	if err != nil {
		return
	}
	defer db.Close()

	var id int64
	err = db.QueryRow("SELECT id FROM sessions WHERE project_path = ?",
		projectRoot).Scan(&id)
	if err != nil {
		if err != sql.ErrNoRows {
			return
		}
	} else if id > 0 {
		sessionID = id
		return
	}

	result, err := db.Exec("INSERT INTO sessions (project_path) VALUES (?)",
		projectRoot)
	if err != nil {
		return
	}

	id, err = result.LastInsertId()
	if err != nil {
		return
	}
	sessionID = id
	return
}
