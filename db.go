package main

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

var (
	ModelID   = int64(0)
	DBPath    = filepath.Join(ConfigDir, "sqlite.db")
	SessionID = int64(0)
)

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
            model_id INTEGER NOT NULL DEFAULT 0,
		reasoning_content TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		// 项目技能关联表
		`CREATE TABLE IF NOT EXISTS project_skills (
			project_path TEXT NOT NULL,
			skill_id INTEGER NOT NULL,
			is_enabled BOOLEAN DEFAULT 1,
			enabled_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_used DATETIME,
			PRIMARY KEY (project_path, skill_id),
			FOREIGN KEY (skill_id) REFERENCES skills(id) ON DELETE CASCADE
		)`,

		// 创建索引
		`CREATE INDEX IF NOT EXISTS idx_skills_category ON skills(category)`,
		`CREATE INDEX IF NOT EXISTS idx_skills_priority ON skills(priority DESC)`,

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
			project_path TEXT NOT NULL,
			tool_id INTEGER NOT NULL,
			used_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			success BOOLEAN DEFAULT 1,
			error_msg TEXT,
			FOREIGN KEY (tool_id) REFERENCES tools(id) ON DELETE CASCADE
		)`,

		// 工具相关索引
		`CREATE INDEX IF NOT EXISTS idx_tools_category ON tools(category)`,
		`CREATE INDEX IF NOT EXISTS idx_tools_usage ON tools(usage_count DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_tool_usage_tool ON tool_usage(tool_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tool_usage_time ON tool_usage(used_at DESC)`,
	}

	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("执行SQL失败: %v\nSQL: %s", err, query)
		}
	}

	queries = []string{
		// 增加model_id到消息表（兼容已存在的数据库）
		// 增加reasoning_content到消息表（兼容已存在的数据库）
	}

	for _, query := range queries {
		_, _ = db.Exec(query) // it's OK err != nil
	}

	return nil
}

func OpenDB(elem ...string) (db *sql.DB, err error) {
	dbPath := DBPath
	if len(elem) != 0 {
		dbPath = filepath.Join(elem...)
	}
	return sql.Open("sqlite", dbPath)
}

// CreateOrGetSessionID 获取或创建会话ID
func CreateOrGetSessionID() (sessionID int64, err error) {
	db, err := OpenDB()
	if err != nil {
		return
	}
	defer db.Close()

	err = createTables(db)
	if err != nil {
		return
	}

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

// LoadPrompts load the prompts
// prompts are system messages - they are the rule
func LoadPrompts(ctx context.Context) ([]Message, error) {
	return []Message{{
		Role:    "system",
		Content: GetSystemPrompt(),
	}}, nil
}

// LoadSkills Load the skills.
// Skills is like the knowledge.
func LoadSkills(ctx context.Context) ([]Message, error) {
	return []Message{}, nil
}

// LoadHistory Load the history.
// History is not messages but the messages enhanced. History is like experiences.
func LoadHistory(ctx context.Context) ([]Message, error) {
	return []Message{}, nil
}

// ==================== Skills 相关方法 ====================

// CreateSkill 创建新技能
func CreateSkill(name, description, content, category string, priority int, isGlobal bool) (int64, error) {
	db, err := OpenDB()
	if err != nil {
		return 0, err
	}
	defer db.Close()
	result, err := db.Exec(`
		INSERT INTO skills (name, description, content, category, priority, is_global)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		name, description, content, category, priority, isGlobal)
	if err != nil {
		return 0, fmt.Errorf("创建技能失败: %w", err)
	}
	return result.LastInsertId()
}

// GetSkill 根据ID获取技能
func GetSkill(id int64) (*Skill, error) {
	db, err := OpenDB()
	if err != nil {
		return nil, err
	}

	defer db.Close()
	var skill Skill
	err = db.QueryRow(`
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
func GetSkillByName(name string) (*Skill, error) {
	db, err := OpenDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var skill Skill
	err = db.QueryRow(`
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
func ListSkills(category string) ([]Skill, error) {
	db, err := OpenDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var rows *sql.Rows

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
func EnableSkill(skillID int64) error {
	db, err := OpenDB()
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec(`
		INSERT OR REPLACE INTO project_skills (project_path, skill_id, is_enabled, enabled_at)
		VALUES (?, ?, 1, CURRENT_TIMESTAMP)`, ProjectRoot, skillID)
	if err != nil {
		return fmt.Errorf("启用技能失败: %w", err)
	}
	return nil
}

// DisableSkill 为项目禁用技能
func DisableSkill(skillID int64) error {
	db, err := OpenDB()
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec(`
		UPDATE project_skills SET is_enabled = 0 WHERE project_path = ? AND skill_id = ?`,
		ProjectRoot, skillID)
	if err != nil {
		return fmt.Errorf("禁用技能失败: %w", err)
	}
	return nil
}

// GetEnabledSkills 获取项目启用的技能
func GetEnabledSkills() ([]Skill, error) {
	db, err := OpenDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`
		SELECT s.id, s.name, s.description, s.content, s.category, s.priority, 
		       s.is_global, s.usage_count, s.created_at, s.updated_at
		FROM skills s
		JOIN project_skills ps ON s.id = ps.skill_id
		WHERE ps.project_path = ? AND ps.is_enabled = 1
		ORDER BY s.priority DESC, s.name`, ProjectRoot)
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
func RecordSkillUsage(skillID int64, projectPath string) error {
	db, err := OpenDB()
	if err != nil {
		return err
	}
	defer db.Close()
	// 更新技能使用次数
	_, err = db.Exec("UPDATE skills SET usage_count = usage_count + 1 WHERE id = ?", skillID)
	if err != nil {
		return fmt.Errorf("更新技能使用次数失败: %w", err)
	}

	// 更新最后使用时间
	_, err = db.Exec(`
		UPDATE project_skills SET last_used = CURRENT_TIMESTAMP 
		WHERE project_path = ? AND skill_id = ?`, projectPath, skillID)
	if err != nil {
		return fmt.Errorf("更新最后使用时间失败: %w", err)
	}

	return nil
}

// ==================== Tools 相关方法 ====================

// GetOrCreateTool 获取或创建工具
func GetOrCreateTool(name, description, category string) (int64, error) {
	db, err := OpenDB()
	if err != nil {
		return 0, err
	}
	defer db.Close()
	var id int64
	err = db.QueryRow("SELECT id FROM tools WHERE name = ?", name).Scan(&id)
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
func GetTool(id int64) (*ToolDesc, error) {
	db, err := OpenDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var tool ToolDesc
	err = db.QueryRow(`
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
func GetToolByName(name string) (*ToolDesc, error) {
	db, err := OpenDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var tool ToolDesc
	err = db.QueryRow(`
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
func ListTools(category string) ([]ToolDesc, error) {
	db, err := OpenDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var rows *sql.Rows

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

	var tools []ToolDesc
	for rows.Next() {
		var tool ToolDesc
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
func RecordToolUsage(toolID int64, success bool, errorMsg string) error {
	db, err := OpenDB()
	if err != nil {
		return err
	}
	defer db.Close()
	// 更新工具使用次数
	_, err = db.Exec("UPDATE tools SET usage_count = usage_count + 1 WHERE id = ?", toolID)
	if err != nil {
		return fmt.Errorf("更新工具使用次数失败: %w", err)
	}

	// 记录使用详情
	_, err = db.Exec(`
		INSERT INTO tool_usage (project_path, tool_id, success, error_msg)
		VALUES (?, ?, ?, ?)`, ProjectRoot, toolID, success, errorMsg)
	if err != nil {
		return fmt.Errorf("记录工具使用详情失败: %w", err)
	}

	return nil
}

// GetToolUsageStats 获取工具使用统计
func GetToolUsageStats(days int) ([]ToolUsageStat, error) {
	db, err := OpenDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var rows *sql.Rows

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
		rows, err = db.Query(query+" GROUP BY t.id ORDER BY t.usage_count DESC", days)
	} else {
		rows, err = db.Query(query + " GROUP BY t.id ORDER BY t.usage_count DESC")
	}

	if err != nil {
		return nil, fmt.Errorf("查询工具统计失败: %w", err)
	}
	defer rows.Close()

	var stats []ToolUsageStat

	for rows.Next() {
		var stat ToolUsageStat
		var lastUsedStr sql.NullString
		if err := rows.Scan(&stat.Name, &stat.UsageCount, &stat.SuccessRate, &lastUsedStr); err != nil {
			return nil, fmt.Errorf("扫描工具统计失败: %w", err)
		}
		if lastUsedStr.Valid && lastUsedStr.String != "" {
			if t, err := time.Parse("2006-01-02 15:04:05", lastUsedStr.String); err == nil {
				stat.LastUsed = t
			}
		}
		stats = append(stats, stat)
	}
	return stats, nil
}

// GetProjectToolUsage 获取项目工具使用情况
func GetProjectToolUsage(days int) ([]ToolUsageStat, error,
) {
	db, err := OpenDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var rows *sql.Rows

	query := `
		SELECT 
			t.name,
			COUNT(tu.id) as usage_count,
			MAX(tu.used_at) as last_used
		FROM tools t
		JOIN tool_usage tu ON t.id = tu.tool_id
		WHERE tu.project_path = ?
	`

	if days > 0 {
		query += " AND tu.used_at >= datetime('now', '-' || ? || ' days')"
		rows, err = db.Query(query+" GROUP BY t.id ORDER BY usage_count DESC", ProjectRoot, days)
	} else {
		rows, err = db.Query(query+" GROUP BY t.id ORDER BY usage_count DESC", ProjectRoot)
	}

	if err != nil {
		return nil, fmt.Errorf("查询项目工具使用失败: %w", err)
	}
	defer rows.Close()

	var stats []ToolUsageStat

	for rows.Next() {
		var stat ToolUsageStat
		var lastUsedStr sql.NullString
		if err := rows.Scan(&stat.Name, &stat.UsageCount, &lastUsedStr); err != nil {
			return nil, fmt.Errorf("扫描项目工具使用失败: %w", err)
		}
		if lastUsedStr.Valid && lastUsedStr.String != "" {
			if t, err := time.Parse("2006-01-02 15:04:05", lastUsedStr.String); err == nil {
				stat.LastUsed = t
			}
		}
		stats = append(stats, stat)
	}
	return stats, nil
}
