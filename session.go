package main

import (
	"database/sql"

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

// CreateOrGetSessionID 获取或创建会话ID
func CreateOrGetSessionID(ctx context.Context) (sessionID int64, err error) {
	projectRoot := context.ContextValue(ctx, context.ProjectRootKey, "")
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
