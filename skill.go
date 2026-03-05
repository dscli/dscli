package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// parseSkillContent 解析技能内容
func parseSkillContent(content string) (map[string]any, error) {
	var data map[string]any
	err := json.Unmarshal([]byte(content), &data)
	if err != nil {
		return nil, fmt.Errorf("解析技能内容失败: %w", err)
	}
	return data, nil
}

// formatSkillContent 格式化技能内容显示
func formatSkillContent(content string) string {
	data, err := parseSkillContent(content)
	if err != nil {
		return fmt.Sprintf("（内容格式错误: %v）", err)
	}

	var builder strings.Builder
	builder.WriteString("技能内容:\n")

	// 显示触发词
	if triggers, ok := data["trigger"].([]any); ok && len(triggers) > 0 {
		builder.WriteString("  触发词: ")
		for i, trigger := range triggers {
			if i > 0 {
				builder.WriteString(", ")
			}
			builder.WriteString(fmt.Sprintf("%v", trigger))
		}
		builder.WriteString("\n")
	}

	// 显示规则
	if rules, ok := data["rules"].([]any); ok && len(rules) > 0 {
		builder.WriteString("  规则:\n")
		for i, rule := range rules {
			builder.WriteString(fmt.Sprintf("    %d. %v\n", i+1, rule))
		}
	}

	// 显示示例
	if examples, ok := data["examples"].([]any); ok && len(examples) > 0 {
		builder.WriteString("  示例:\n")
		for i, example := range examples {
			builder.WriteString(fmt.Sprintf("    %d. %v\n", i+1, example))
		}
	}

	return builder.String()
}

// PrintSkill 打印技能信息
func PrintSkill(skill Skill, detailed bool) {
	if detailed {
		// 详细显示模式
		Println(strings.Repeat("=", 80))
		Printf("技能: %s\n", skill.Name)
		Println(strings.Repeat("=", 80))

		Printf("ID:               %d\n", skill.ID)
		Printf("名称:             %s\n", skill.Name)
		Printf("描述:             %s\n", skill.Description)
		Printf("分类:             %s\n", skill.Category)
		Printf("优先级:           %d\n", skill.Priority)
		Printf("全局技能:         %v\n", skill.IsGlobal)
		Printf("使用次数:         %d\n", skill.UsageCount)
		Printf("创建时间:         %s\n", skill.CreatedAt.Format("2006-01-02 15:04:05"))
		Printf("更新时间:         %s\n", skill.UpdatedAt.Format("2006-01-02 15:04:05"))

		Println(strings.Repeat("-", 80))
		Println(formatSkillContent(skill.Content))
		Println(strings.Repeat("=", 80))
	} else {
		// 简洁显示模式
		Printf("#%d [%s] %s\n", skill.ID, skill.Category, skill.Name)
		Printf("  描述: %s\n", skill.Description)
		Printf("  优先级: %d | 全局: %v | 使用次数: %d\n",
			skill.Priority, skill.IsGlobal, skill.UsageCount)
		Printf("  创建: %s | 更新: %s\n",
			skill.CreatedAt.Format("2006-01-02 15:04:05"),
			skill.UpdatedAt.Format("2006-01-02 15:04:05"))
		Println()
	}
}

func init() {
	RegisterTableSchema(
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
			is_enabled BOOLEAN DEFAULT 1,
			enabled_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_used DATETIME,
			PRIMARY KEY (project_path, skill_id),
			FOREIGN KEY (skill_id) REFERENCES skills(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_skills_category ON skills(category)`,
		`CREATE INDEX IF NOT EXISTS idx_skills_priority ON skills(priority DESC)`,
	)

	// 创建skills命令
	skillsCmd := AddRootCommand(&cobra.Command{
		Use:   "skills",
		Short: "管理技能",
		Long:  `管理技能系统，包括增删改查等操作。`,
	})

	// list命令
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "列出所有技能",
		Long: `列出所有技能，支持按分类和优先级排序。

示例:
  dscli skills list
  dscli skills list --category go
  dscli skills list --priority high`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("list命令暂未实现")
		},
	}
	listCmd.Flags().String("category", "", "按分类筛选")
	listCmd.Flags().String("priority", "", "按优先级筛选（high/medium/low）")

	// show命令
	showCmd := &cobra.Command{
		Use:   "show <id>",
		Short: "显示指定技能的详细信息",
		Long: `显示指定技能的详细信息。

示例:
  dscli skills show 1
  dscli skills show 2`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("show命令暂未实现")
		},
	}

	// create命令
	createCmd := &cobra.Command{
		Use:   "create",
		Short: "创建新技能",
		Long: `创建新技能。

可以通过以下方式提供内容：
1. 使用 --name, --description, --content 参数
2. 使用 --file 参数从JSON文件读取技能配置

示例:
  dscli skills create --name "Go测试规范" --description "Go语言测试最佳实践" --content '{"trigger": ["test", "测试"], "rules": ["规则1"], "examples": ["示例1"]}'
  dscli skills create --file skill.json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("create命令暂未实现")
		},
	}
	createCmd.Flags().String("name", "", "技能名称（必需）")
	createCmd.Flags().String("description", "", "技能描述（必需）")
	createCmd.Flags().String("content", "", "技能内容（JSON格式）")
	createCmd.Flags().String("file", "", "从JSON文件读取技能配置")
	createCmd.Flags().String("category", "", "技能分类")
	createCmd.Flags().Int("priority", 50, "技能优先级（1-100）")
	createCmd.Flags().Bool("global", false, "是否为全局技能")

	// update命令
	updateCmd := &cobra.Command{
		Use:   "update <id>",
		Short: "更新指定技能",
		Long: `更新指定技能。

可以通过以下方式更新：
1. 使用 --name 更新名称
2. 使用 --description 更新描述
3. 使用 --content 更新内容
4. 使用 --category 更新分类
5. 使用 --priority 更新优先级
6. 使用 --global 更新全局状态

示例:
  dscli skills update 1 --name "新名称"
  dscli skills update 2 --priority 90 --global true`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("update命令暂未实现")
		},
	}
	updateCmd.Flags().String("name", "", "更新技能名称")
	updateCmd.Flags().String("description", "", "更新技能描述")
	updateCmd.Flags().String("content", "", "更新技能内容（JSON格式）")
	updateCmd.Flags().String("category", "", "更新技能分类")
	updateCmd.Flags().Int("priority", 0, "更新技能优先级（1-100）")
	updateCmd.Flags().Bool("global", false, "更新是否为全局技能")

	// delete命令
	deleteCmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "删除指定技能",
		Long: `删除指定技能。

示例:
  dscli skills delete 1
  dscli skills delete 2`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("delete命令暂未实现")
		},
	}

	// enable命令 - 为项目启用技能
	enableCmd := &cobra.Command{
		Use:   "enable <skill-id>",
		Short: "为当前项目启用技能",
		Long: `为当前项目启用指定技能。

示例:
  dscli skills enable 1
  dscli skills enable 2`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("enable命令暂未实现")
		},
	}

	// disable命令 - 为项目禁用技能
	disableCmd := &cobra.Command{
		Use:   "disable <skill-id>",
		Short: "为当前项目禁用技能",
		Long: `为当前项目禁用指定技能。

示例:
  dscli skills disable 1
  dscli skills disable 2`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("disable命令暂未实现")
		},
	}

	// search命令 - 搜索技能
	searchCmd := &cobra.Command{
		Use:   "search <query>",
		Short: "搜索技能",
		Long: `根据关键词搜索技能。

示例:
  dscli skills search "测试"
  dscli skills search "Go"
  dscli skills search "规范"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("search命令暂未实现")
		},
	}

	// import命令 - 从文件导入技能
	importCmd := &cobra.Command{
		Use:   "import <file>",
		Short: "从JSON文件导入技能",
		Long: `从JSON文件导入技能配置。

示例:
  dscli skills import skills.json
  dscli skills import path/to/skills.json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("import命令暂未实现")
		},
	}

	// export命令 - 导出技能到文件
	exportCmd := &cobra.Command{
		Use:   "export <file>",
		Short: "导出技能到JSON文件",
		Long: `导出所有技能到JSON文件。

示例:
  dscli skills export skills.json
  dscli skills export backup/skills.json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("export命令暂未实现")
		},
	}

	// stats命令 - 显示技能统计信息
	statsCmd := &cobra.Command{
		Use:   "stats",
		Short: "显示技能统计信息",
		Long: `显示技能系统的统计信息。

示例:
  dscli skills stats`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("stats命令暂未实现")
		},
	}

	// 添加所有子命令
	skillsCmd.AddCommand(
		listCmd,
		showCmd,
		createCmd,
		updateCmd,
		deleteCmd,
		enableCmd,
		disableCmd,
		searchCmd,
		importCmd,
		exportCmd,
		statsCmd,
	)
}

func LoadSkills(ctx context.Context) ([]Message, error) {
	return []Message{}, nil
}

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
