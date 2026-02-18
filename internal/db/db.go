package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Message 表示一条对话消息，支持工具调用
type Message struct {
	Role       string
	Content    string
	ToolCallID string          // 仅当 role="tool" 时有效
	ToolCalls  json.RawMessage // 仅当 role="assistant" 且包含工具调用时有效，存储 ToolCall 数组的 JSON
	CreatedAt  time.Time
}

// Session 表示一个对话会话
type Session struct {
	ID          int64
	ProjectPath string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// DB 封装数据库操作
type DB struct {
	*sql.DB
	path string
}

// New 创建或打开数据库（统一位置 ~/.dscli/sqlite.db）
func New() (*DB, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("获取用户主目录失败: %w", err)
	}
	dbPath := filepath.Join(home, ".dscli", "sqlite.db")

	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("创建数据库目录失败: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}

	// 创建表（增加 tool_call_id 和 tool_calls 字段）
	createTablesSQL := `
	CREATE TABLE IF NOT EXISTS sessions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		project_path TEXT UNIQUE NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id INTEGER NOT NULL,
		role TEXT NOT NULL,
		content TEXT NOT NULL,
		tool_call_id TEXT,
		tool_calls TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (session_id) REFERENCES sessions(id)
	);
	CREATE INDEX IF NOT EXISTS idx_messages_session_id ON messages(session_id);
	`
	if _, err := db.Exec(createTablesSQL); err != nil {
		return nil, fmt.Errorf("创建表失败: %w", err)
	}

	return &DB{DB: db, path: dbPath}, nil
}

// GetOrCreateSession 根据项目路径获取或创建会话
func (db *DB) GetOrCreateSession(projectPath string) (int64, error) {
	var id int64
	err := db.QueryRow("SELECT id FROM sessions WHERE project_path = ?", projectPath).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, fmt.Errorf("查询会话失败: %w", err)
	}

	result, err := db.Exec("INSERT INTO sessions (project_path) VALUES (?)", projectPath)
	if err != nil {
		return 0, fmt.Errorf("创建会话失败: %w", err)
	}
	id, err = result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("获取新会话ID失败: %w", err)
	}
	return id, nil
}

// LoadHistory 加载指定会话的所有历史消息，按时间升序返回
func (db *DB) LoadHistory(sessionID int64) ([]Message, error) {
	rows, err := db.Query(`
		SELECT role, content, tool_call_id, tool_calls, created_at
		FROM messages
		WHERE session_id = ?
		ORDER BY created_at ASC`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("查询历史消息失败: %w", err)
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		var toolCallID sql.NullString
		var toolCalls sql.NullString
		if err := rows.Scan(&m.Role, &m.Content, &toolCallID, &toolCalls, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("扫描消息失败: %w", err)
		}
		if toolCallID.Valid {
			m.ToolCallID = toolCallID.String
		}
		if toolCalls.Valid {
			m.ToolCalls = json.RawMessage(toolCalls.String)
		}
		messages = append(messages, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历消息失败: %w", err)
	}
	return messages, nil
}

// SaveMessagesBatch 批量保存消息（事务）
func (db *DB) SaveMessagesBatch(sessionID int64, msgs []Message) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO messages (session_id, role, content, tool_call_id, tool_calls)
		VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("准备语句失败: %w", err)
	}
	defer stmt.Close()

	for _, m := range msgs {
		var toolCallID, toolCalls sql.NullString
		if m.ToolCallID != "" {
			toolCallID.String = m.ToolCallID
			toolCallID.Valid = true
		}
		if len(m.ToolCalls) > 0 {
			toolCalls.String = string(m.ToolCalls)
			toolCalls.Valid = true
		}
		if _, err := stmt.Exec(sessionID, m.Role, m.Content, toolCallID, toolCalls); err != nil {
			return fmt.Errorf("插入消息失败: %w", err)
		}
	}

	// 更新会话的更新时间
	if _, err := tx.Exec("UPDATE sessions SET updated_at = CURRENT_TIMESTAMP WHERE id = ?", sessionID); err != nil {
		return fmt.Errorf("更新会话时间失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	return nil
}

// Close 关闭数据库连接
func (db *DB) Close() error {
	return db.DB.Close()
}
