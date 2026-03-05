package main

import "database/sql"

var SessionID = int64(0)

func init() {
	RegisterTableSchema(
		// 会话表
		`CREATE TABLE IF NOT EXISTS sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_path TEXT UNIQUE NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`)
}

// CreateOrGetSessionID 获取或创建会话ID
func CreateOrGetSessionID() (sessionID int64, err error) {
	db, err := OpenDB()
	if err != nil {
		return
	}
	defer db.Close()

	var id int64
	err = db.QueryRow("SELECT id FROM sessions WHERE project_path = ?",
		ProjectRoot).Scan(&id)
	if err != nil {
		if err != sql.ErrNoRows {
			return
		}
	} else if id > 0 {
		sessionID = id
		return
	}

	result, err := db.Exec("INSERT INTO sessions (project_path) VALUES (?)",
		ProjectRoot)
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
