package toolcall

import (
	"database/sql"
	"fmt"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/sqlite"
)

func init() {
	sqlite.RegisterTableSchema(
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
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		// 项目技能关联表
		`CREATE TABLE IF NOT EXISTS project_skills (
			project_path TEXT NOT NULL,
			skill_id INTEGER NOT NULL,
			PRIMARY KEY (project_path, skill_id),
			FOREIGN KEY (skill_id) REFERENCES skills(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_skills_category ON skills(category)`,
		`CREATE INDEX IF NOT EXISTS idx_skills_priority ON skills(priority DESC)`,
	)
}

func GetProjectSkills(ctx context.Context) (skills []*Skill, err error) {
	projectRoot := context.ProjectRoot
	db, err := sqlite.OpenDB()
	if err != nil {
		return
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx,
		`SELECT id, name, description, category FROM skills WHERE id IN
 (SELECT skill_id FROM project_skills WHERE project_PATH = ?)`, projectRoot)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		skill := &Skill{}
		err = rows.Scan(&skill.ID, &skill.Name, &skill.Description, &skill.Category)
		if err != nil {
			return
		}
		skills = append(skills, skill)
	}
	return
}

// GetSkillByID 根据ID获取技能
func GetSkillByID(id int64) (*Skill, error) {
	db, err := sqlite.OpenDB()
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	defer db.Close()

	var skill Skill
	err = db.QueryRow(`
		SELECT id, name, description, content, category, priority, is_global, usage_count, created_at, updated_at
		FROM skills WHERE id = ?
	`, id).Scan(
		&skill.ID, &skill.Name, &skill.Description, &skill.Content, &skill.Category,
		&skill.Priority, &skill.IsGlobal, &skill.UsageCount, &skill.CreatedAt, &skill.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询技能失败: %w", err)
	}

	return &skill, nil
}

// GetSkillByName 根据名称获取技能
func GetSkillByName(name string) (*Skill, error) {
	db, err := sqlite.OpenDB()
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	defer db.Close()

	var skill Skill
	err = db.QueryRow(`
		SELECT id, name, description, content, category, priority, is_global, usage_count, created_at, updated_at
		FROM skills WHERE name = ?
	`, name).Scan(
		&skill.ID, &skill.Name, &skill.Description, &skill.Content, &skill.Category,
		&skill.Priority, &skill.IsGlobal, &skill.UsageCount, &skill.CreatedAt, &skill.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询技能失败: %w", err)
	}

	return &skill, nil
}

// CreateSkill 创建新技能
func CreateSkill(ctx context.Context, skill *Skill) error {
	db, err := sqlite.OpenDB()
	if err != nil {
		return fmt.Errorf("打开数据库失败: %w", err)
	}
	defer db.Close()

	result, err := db.ExecContext(ctx, `INSERT OR REPLACE INTO skills (name, description, content, category, priority, is_global)
		VALUES (?, ?, ?, ?, ?, ?)
	`, skill.Name, skill.Description, skill.Content, skill.Category, skill.Priority, skill.IsGlobal)
	if err != nil {
		return fmt.Errorf("创建技能失败: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("获取技能ID失败: %w", err)
	}

	skill.ID = id
	return nil
}

func CreateProjectSkill(ctx context.Context, id int64) (err error) {
	projectRoot := context.ProjectRoot
	db, err := sqlite.OpenDB()
	if err != nil {
		return
	}

	_, err = db.ExecContext(ctx,
		"INSERT OR REPLACE INTO project_skills (skill_id, project_path) VALUES(?, ?)", id, projectRoot)
	if err != nil {
		return
	}
	return
}

// RecordSkillUsage 记录技能使用
func RecordSkillUsage(skillID int64, projectPath string) error {
	db, err := sqlite.OpenDB()
	if err != nil {
		return fmt.Errorf("打开数据库失败: %w", err)
	}
	defer db.Close()

	// 更新技能使用次数
	_, err = db.Exec(`
		UPDATE skills 
		SET usage_count = usage_count + 1, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, skillID)
	if err != nil {
		return fmt.Errorf("更新技能使用次数失败: %w", err)
	}

	// 更新项目技能关联表的最后使用时间
	_, err = db.Exec(`
		UPDATE project_skills 
		SET last_used = CURRENT_TIMESTAMP
		WHERE skill_id = ? AND project_path = ?
	`, skillID, projectPath)
	if err != nil {
		// 如果更新失败，只记录日志，不中断操作
		outfmt.Println("更新项目技能关联失败:", err)
	}

	return nil
}
