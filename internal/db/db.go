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

// Skill 表示一个技能
type Skill struct {
	ID          int64
	Name        string
	Description string
	Content     string
	Category    string
	Priority    int
	IsGlobal    bool
	UsageCount  int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ProjectSkill 表示项目与技能的关联

// Tool 表示一个工具
type Tool struct {
	ID          int64
	Name        string
	Description string
	Category    string
	UsageCount  int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ToolUsage 表示工具使用记录
type ToolUsage struct {
	ID          int64
	ProjectHash string
	ToolID      int64
	UsedAt      time.Time
	Success     bool
	ErrorMsg    string
}
type ProjectSkill struct {
	ProjectHash string
	SkillID     int64
	IsEnabled   bool
	EnabledAt   time.Time
	LastUsed    sql.NullTime
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
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("创建数据库目录失败: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}

	// 创建表
	if err := createTables(db); err != nil {
		return nil, fmt.Errorf("创建表失败: %w", err)
	}

	return &DB{DB: db, path: dbPath}, nil
}

// createTables 创建所有需要的表
func createTables(db *sql.DB) error {
	queries := []string{
		// 会话表
		`CREATE TABLE IF NOT EXISTS sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_path TEXT UNIQUE NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		// 消息表
		`CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id INTEGER NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			tool_call_id TEXT,
			tool_calls TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (session_id) REFERENCES sessions(id)
		)`,

		// 技能表
		`CREATE TABLE IF NOT EXISTS skills (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			description TEXT NOT NULL,
			content TEXT NOT NULL,
			category TEXT,
			priority INTEGER DEFAULT 50,
			is_global BOOLEAN DEFAULT 0,
			usage_count INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		// 项目技能关联表
		`CREATE TABLE IF NOT EXISTS project_skills (
			project_hash TEXT NOT NULL,
			skill_id INTEGER NOT NULL,
			is_enabled BOOLEAN DEFAULT 1,
			enabled_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_used DATETIME,
			PRIMARY KEY (project_hash, skill_id),
			FOREIGN KEY (skill_id) REFERENCES skills(id) ON DELETE CASCADE
		)`,

		// 创建索引
		`CREATE INDEX IF NOT EXISTS idx_messages_session_id ON messages(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_skills_category ON skills(category)`,
		`CREATE INDEX IF NOT EXISTS idx_skills_priority ON skills(priority DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_project_skills_enabled ON project_skills(project_hash, is_enabled)`,

		// 工具表
		`CREATE TABLE IF NOT EXISTS tools (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			description TEXT NOT NULL,
			category TEXT,
			usage_count INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		// 工具使用记录表
		`CREATE TABLE IF NOT EXISTS tool_usage (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_hash TEXT NOT NULL,
			tool_id INTEGER NOT NULL,
			used_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			success BOOLEAN DEFAULT 1,
			error_msg TEXT,
			FOREIGN KEY (tool_id) REFERENCES tools(id) ON DELETE CASCADE
		)`,

		// 工具相关索引
		`CREATE INDEX IF NOT EXISTS idx_tools_category ON tools(category)`,
		`CREATE INDEX IF NOT EXISTS idx_tools_usage ON tools(usage_count DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_tool_usage_project ON tool_usage(project_hash)`,
		`CREATE INDEX IF NOT EXISTS idx_tool_usage_tool ON tool_usage(tool_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tool_usage_time ON tool_usage(used_at DESC)`, 
	}

	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("执行SQL失败: %v\nSQL: %s", err, query)
		}
	}
	return nil
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

func (db *DB) LoadLastOne(sessionID int64) (*Message, error) {
	rows, err := db.Query(`
        SELECT role, content, tool_call_id, tool_calls, created_at 
        FROM messages
        WHERE session_id = ?
        ORDER BY id DESC 
        LIMIT 1`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to load last: %w", err)
	}
	defer rows.Close()
	var m Message
	if rows.Next() {
		var toolCallID sql.NullString
		var toolCalls sql.NullString
		if err := rows.Scan(&m.Role, &m.Content, &toolCallID, &toolCalls, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}
		if toolCallID.Valid {
			m.ToolCallID = toolCallID.String
		}
		if toolCalls.Valid {
			m.ToolCalls = json.RawMessage(toolCalls.String)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to loop rows: %w", err)
	}

	return &m, nil
}

// LoadHistory 加载指定会话的所有历史消息，按时间升序返回
func (db *DB) LoadHistory(sessionID int64) ([]Message, error) {
	rows, err := db.Query(`
		SELECT role, content, tool_call_id, tool_calls, created_at
		FROM messages
		WHERE session_id = ?
		ORDER BY id ASC`, sessionID)
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
	n := len(messages)
	idx := n - 128
	if idx > 0 {
		for {
			m := messages[idx]
			if m.ToolCallID == "" && len(m.ToolCalls) == 0 || idx == 0{
				break
			}
			idx -= 1
		}
	} else {
		idx = 0
	}
	return messages[idx:], nil
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

// ==================== Skills 相关方法 ====================

// GetProjectHash 获取项目路径的哈希值
func GetProjectHash(projectPath string) string {
	// 简单实现：使用路径作为哈希（实际可以使用MD5等）
	// 这里为了简单，直接使用路径，实际应该使用哈希函数
	return projectPath
}

// CreateSkill 创建新技能
func (db *DB) CreateSkill(name, description, content, category string, priority int, isGlobal bool) (int64, error) {
	result, err := db.Exec(`
		INSERT INTO skills (name, description, content, category, priority, is_global)
		VALUES (?, ?, ?, ?, ?, ?)`,
		name, description, content, category, priority, isGlobal)
	if err != nil {
		return 0, fmt.Errorf("创建技能失败: %w", err)
	}
	return result.LastInsertId()
}

// GetSkill 根据ID获取技能
func (db *DB) GetSkill(id int64) (*Skill, error) {
	var skill Skill
	err := db.QueryRow(`
		SELECT id, name, description, content, category, priority, is_global, usage_count, created_at, updated_at
		FROM skills WHERE id = ?`, id).Scan(
		&skill.ID, &skill.Name, &skill.Description, &skill.Content, &skill.Category,
		&skill.Priority, &skill.IsGlobal, &skill.UsageCount, &skill.CreatedAt, &skill.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("获取技能失败: %w", err)
	}
	return &skill, nil
}

// GetSkillByName 根据名称获取技能
func (db *DB) GetSkillByName(name string) (*Skill, error) {
	var skill Skill
	err := db.QueryRow(`
		SELECT id, name, description, content, category, priority, is_global, usage_count, created_at, updated_at
		FROM skills WHERE name = ?`, name).Scan(
		&skill.ID, &skill.Name, &skill.Description, &skill.Content, &skill.Category,
		&skill.Priority, &skill.IsGlobal, &skill.UsageCount, &skill.CreatedAt, &skill.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("获取技能失败: %w", err)
	}
	return &skill, nil
}

// ListSkills 列出所有技能（可按分类过滤）
func (db *DB) ListSkills(category string) ([]Skill, error) {
	var rows *sql.Rows
	var err error

	if category == "" {
		rows, err = db.Query(`
			SELECT id, name, description, content, category, priority, is_global, usage_count, created_at, updated_at
			FROM skills ORDER BY priority DESC, name`)
	} else {
		rows, err = db.Query(`
			SELECT id, name, description, content, category, priority, is_global, usage_count, created_at, updated_at
			FROM skills WHERE category = ? ORDER BY priority DESC, name`, category)
	}

	if err != nil {
		return nil, fmt.Errorf("查询技能失败: %w", err)
	}
	defer rows.Close()

	var skills []Skill
	for rows.Next() {
		var skill Skill
		if err := rows.Scan(
			&skill.ID, &skill.Name, &skill.Description, &skill.Content, &skill.Category,
			&skill.Priority, &skill.IsGlobal, &skill.UsageCount, &skill.CreatedAt, &skill.UpdatedAt); err != nil {
			return nil, fmt.Errorf("扫描技能失败: %w", err)
		}
		skills = append(skills, skill)
	}
	return skills, nil
}

// EnableSkill 为项目启用技能
func (db *DB) EnableSkill(projectHash string, skillID int64) error {
	_, err := db.Exec(`
		INSERT OR REPLACE INTO project_skills (project_hash, skill_id, is_enabled, enabled_at)
		VALUES (?, ?, 1, CURRENT_TIMESTAMP)`, projectHash, skillID)
	if err != nil {
		return fmt.Errorf("启用技能失败: %w", err)
	}
	return nil
}

// DisableSkill 为项目禁用技能
func (db *DB) DisableSkill(projectHash string, skillID int64) error {
	_, err := db.Exec(`
		UPDATE project_skills SET is_enabled = 0 WHERE project_hash = ? AND skill_id = ?`,
		projectHash, skillID)
	if err != nil {
		return fmt.Errorf("禁用技能失败: %w", err)
	}
	return nil
}

// GetEnabledSkills 获取项目启用的技能
func (db *DB) GetEnabledSkills(projectHash string) ([]Skill, error) {
	rows, err := db.Query(`
		SELECT s.id, s.name, s.description, s.content, s.category, s.priority, 
		       s.is_global, s.usage_count, s.created_at, s.updated_at
		FROM skills s
		JOIN project_skills ps ON s.id = ps.skill_id
		WHERE ps.project_hash = ? AND ps.is_enabled = 1
		ORDER BY s.priority DESC, s.name`, projectHash)
	if err != nil {
		return nil, fmt.Errorf("查询启用技能失败: %w", err)
	}
	defer rows.Close()

	var skills []Skill
	for rows.Next() {
		var skill Skill
		if err := rows.Scan(
			&skill.ID, &skill.Name, &skill.Description, &skill.Content, &skill.Category,
			&skill.Priority, &skill.IsGlobal, &skill.UsageCount, &skill.CreatedAt, &skill.UpdatedAt); err != nil {
			return nil, fmt.Errorf("扫描技能失败: %w", err)
		}
		skills = append(skills, skill)
	}
	return skills, nil
}

// RecordSkillUsage 记录技能使用
func (db *DB) RecordSkillUsage(skillID int64, projectHash string) error {
	// 更新技能使用次数
	_, err := db.Exec("UPDATE skills SET usage_count = usage_count + 1 WHERE id = ?", skillID)
	if err != nil {
		return fmt.Errorf("更新技能使用次数失败: %w", err)
	}

	// 更新最后使用时间
	_, err = db.Exec(`
		UPDATE project_skills SET last_used = CURRENT_TIMESTAMP 
		WHERE project_hash = ? AND skill_id = ?`, projectHash, skillID)
	if err != nil {
		return fmt.Errorf("更新最后使用时间失败: %w", err)
	}

	return nil
}

// Close 关闭数据库连接
func (db *DB) Close() error {
	return db.DB.Close()
}

// ==================== Tools 相关方法 ====================

// GetOrCreateTool 获取或创建工具
func (db *DB) GetOrCreateTool(name, description, category string) (int64, error) {
	var id int64
	err := db.QueryRow("SELECT id FROM tools WHERE name = ?", name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, fmt.Errorf("查询工具失败: %w", err)
	}

	result, err := db.Exec(`
		INSERT INTO tools (name, description, category)
		VALUES (?, ?, ?)`, name, description, category)
	if err != nil {
		return 0, fmt.Errorf("创建工具失败: %w", err)
	}
	return result.LastInsertId()
}

// GetTool 根据ID获取工具
func (db *DB) GetTool(id int64) (*Tool, error) {
	var tool Tool
	err := db.QueryRow(`
		SELECT id, name, description, category, usage_count, created_at, updated_at
		FROM tools WHERE id = ?`, id).Scan(
		&tool.ID, &tool.Name, &tool.Description, &tool.Category,
		&tool.UsageCount, &tool.CreatedAt, &tool.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("获取工具失败: %w", err)
	}
	return &tool, nil
}

// GetToolByName 根据名称获取工具
func (db *DB) GetToolByName(name string) (*Tool, error) {
	var tool Tool
	err := db.QueryRow(`
		SELECT id, name, description, category, usage_count, created_at, updated_at
		FROM tools WHERE name = ?`, name).Scan(
		&tool.ID, &tool.Name, &tool.Description, &tool.Category,
		&tool.UsageCount, &tool.CreatedAt, &tool.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("获取工具失败: %w", err)
	}
	return &tool, nil
}

// ListTools 列出所有工具（可按分类过滤）
func (db *DB) ListTools(category string) ([]Tool, error) {
	var rows *sql.Rows
	var err error

	if category == "" {
		rows, err = db.Query(`
			SELECT id, name, description, category, usage_count, created_at, updated_at
			FROM tools ORDER BY usage_count DESC, name`)
	} else {
		rows, err = db.Query(`
			SELECT id, name, description, category, usage_count, created_at, updated_at
			FROM tools WHERE category = ? ORDER BY usage_count DESC, name`, category)
	}

	if err != nil {
		return nil, fmt.Errorf("查询工具失败: %w", err)
	}
	defer rows.Close()

	var tools []Tool
	for rows.Next() {
		var tool Tool
		if err := rows.Scan(
			&tool.ID, &tool.Name, &tool.Description, &tool.Category,
			&tool.UsageCount, &tool.CreatedAt, &tool.UpdatedAt); err != nil {
			return nil, fmt.Errorf("扫描工具失败: %w", err)
		}
		tools = append(tools, tool)
	}
	return tools, nil
}

// RecordToolUsage 记录工具使用
func (db *DB) RecordToolUsage(toolID int64, projectHash string, success bool, errorMsg string) error {
	// 更新工具使用次数
	_, err := db.Exec("UPDATE tools SET usage_count = usage_count + 1 WHERE id = ?", toolID)
	if err != nil {
		return fmt.Errorf("更新工具使用次数失败: %w", err)
	}

	// 记录使用详情
	_, err = db.Exec(`
		INSERT INTO tool_usage (project_hash, tool_id, success, error_msg)
		VALUES (?, ?, ?, ?)`, projectHash, toolID, success, errorMsg)
	if err != nil {
		return fmt.Errorf("记录工具使用详情失败: %w", err)
	}

	return nil
}

// GetToolUsageStats 获取工具使用统计
func (db *DB) GetToolUsageStats(days int) ([]struct {
	Name       string
	UsageCount int
	SuccessRate float64
	LastUsed   time.Time
}, error) {
	var rows *sql.Rows
	var err error

	query := `
		SELECT 
			t.name,
			t.usage_count,
			COALESCE(SUM(CASE WHEN tu.success THEN 1 ELSE 0 END) * 100.0 / COUNT(*), 100) as success_rate,
			MAX(tu.used_at) as last_used
		FROM tools t
		LEFT JOIN tool_usage tu ON t.id = tu.tool_id
	`

	if days > 0 {
		query += " WHERE tu.used_at >= datetime('now', '-' || ? || ' days')"
		rows, err = db.Query(query + " GROUP BY t.id ORDER BY t.usage_count DESC", days)
	} else {
		rows, err = db.Query(query + " GROUP BY t.id ORDER BY t.usage_count DESC")
	}

	if err != nil {
		return nil, fmt.Errorf("查询工具统计失败: %w", err)
	}
	defer rows.Close()

	var stats []struct {
		Name       string
		UsageCount int
		SuccessRate float64
		LastUsed   time.Time
	}

	for rows.Next() {
		var stat struct {
			Name       string
			UsageCount int
			SuccessRate float64
			LastUsed   time.Time
		}
		var lastUsed sql.NullTime
		if err := rows.Scan(&stat.Name, &stat.UsageCount, &stat.SuccessRate, &lastUsed); err != nil {
			return nil, fmt.Errorf("扫描工具统计失败: %w", err)
		}
		if lastUsed.Valid {
			stat.LastUsed = lastUsed.Time
		}
		stats = append(stats, stat)
	}
	return stats, nil
}

// GetProjectToolUsage 获取项目工具使用情况
func (db *DB) GetProjectToolUsage(projectHash string, days int) ([]struct {
	Name       string
	UsageCount int
	LastUsed   time.Time
}, error) {
	var rows *sql.Rows
	var err error

	query := `
		SELECT 
			t.name,
			COUNT(tu.id) as usage_count,
			MAX(tu.used_at) as last_used
		FROM tools t
		JOIN tool_usage tu ON t.id = tu.tool_id
		WHERE tu.project_hash = ?
	`

	if days > 0 {
		query += " AND tu.used_at >= datetime('now', '-' || ? || ' days')"
		rows, err = db.Query(query + " GROUP BY t.id ORDER BY usage_count DESC", projectHash, days)
	} else {
		rows, err = db.Query(query + " GROUP BY t.id ORDER BY usage_count DESC", projectHash)
	}

	if err != nil {
		return nil, fmt.Errorf("查询项目工具使用失败: %w", err)
	}
	defer rows.Close()

	var stats []struct {
		Name       string
		UsageCount int
		LastUsed   time.Time
	}

	for rows.Next() {
		var stat struct {
			Name       string
			UsageCount int
			LastUsed   time.Time
		}
		var lastUsed sql.NullTime
		if err := rows.Scan(&stat.Name, &stat.UsageCount, &lastUsed); err != nil {
			return nil, fmt.Errorf("扫描项目工具使用失败: %w", err)
		}
		if lastUsed.Valid {
			stat.LastUsed = lastUsed.Time
		}
		stats = append(stats, stat)
	}
	return stats, nil
}
