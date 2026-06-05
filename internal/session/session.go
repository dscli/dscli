package session

import (
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/sqlite"
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
	ID           int64
	ProjectPath  string
	CreatedAt    string
	MaintainerCN string // e.g. "玻尔" or empty
	MaintainerEN string // e.g. "Bohr" or empty
	MaintainerID int64  // ai_names.id, 0 if none
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
		       COALESCE(a.name_cn, ''),
		       COALESCE(a.name_en, ''),
		       COALESCE(a.id, 0)
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
		if err := rows.Scan(&r.ID, &r.ProjectPath, &r.CreatedAt, &r.MaintainerCN, &r.MaintainerEN, &r.MaintainerID); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// AssignMaintainer assigns a maintainer (ai_names.id) to a session (sessions.id).
// Uses UPSERT — replaces any existing assignment for that session.
func AssignMaintainer(sessionID, nameID int64) error {
	db, err := sqlite.OpenDB()
	if err != nil {
		return err
	}
	defer db.Close()

	// Verify session exists.
	var sid int64
	if err := db.QueryRow("SELECT id FROM sessions WHERE id = ?", sessionID).Scan(&sid); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("project %d: 不存在", sessionID)
		}
		return err
	}

	// Verify name exists.
	var nid int64
	if err := db.QueryRow("SELECT id FROM ai_names WHERE id = ?", nameID).Scan(&nid); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("maintainer %d: 不存在", nameID)
		}
		return err
	}

	_, err = db.Exec(`
		INSERT INTO session_names (session_id, name_id)
		VALUES (?, ?)
		ON CONFLICT(session_id) DO UPDATE SET
			name_id = excluded.name_id,
			assigned_at = CURRENT_TIMESTAMP`,
		sessionID, nameID)
	return err
}

// UpdateProjectPath updates the project_path for a given session.
// Returns an error if the session does not exist or the path is empty.
func UpdateProjectPath(sessionID int64, newPath string) error {
	if newPath == "" {
		return fmt.Errorf("project path 不能为空")
	}

	db, err := sqlite.OpenDB()
	if err != nil {
		return err
	}
	defer db.Close()

	// Verify session exists.
	var sid int64
	if err := db.QueryRow("SELECT id FROM sessions WHERE id = ?", sessionID).Scan(&sid); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("project %d: 不存在", sessionID)
		}
		return err
	}

	_, err = db.Exec(`UPDATE sessions SET project_path = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		newPath, sessionID)
	return err
}
