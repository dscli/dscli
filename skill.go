package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strings"
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

// formatSkillContent 格式化技能内容
func formatSkillContent(content string) string {
	data, err := parseSkillContent(content)
	if err != nil {
		return fmt.Sprintf("解析技能内容失败: %v", err)
	}

	var builder strings.Builder

	// 格式化标题
	if title, ok := data["title"].(string); ok && title != "" {
		builder.WriteString(fmt.Sprintf("标题: %s\n", title))
	}

	// 格式化描述
	if description, ok := data["description"].(string); ok && description != "" {
		builder.WriteString(fmt.Sprintf("描述: %s\n", description))
	}

	// 格式化章节
	if sections, ok := data["sections"].([]any); ok {
		for i, section := range sections {
			if sectionMap, ok := section.(map[string]any); ok {
				builder.WriteString(fmt.Sprintf("\n## %d. ", i+1))
				if title, ok := sectionMap["title"].(string); ok {
					builder.WriteString(title)
				}
				builder.WriteString("\n")

				if content, ok := sectionMap["content"].(string); ok {
					builder.WriteString(content)
					builder.WriteString("\n")
				}
			}
		}
	}

	// 格式化示例
	if examples, ok := data["examples"].([]any); ok && len(examples) > 0 {
		builder.WriteString("\n示例:\n")
		for i, example := range examples {
			builder.WriteString(fmt.Sprintf("%d. %v\n", i+1, example))
		}
	}

	return builder.String()
}

// formatSkillDetails 格式化技能详细信息
func formatSkillDetails(skill *Skill, detailed bool) string {
	var builder strings.Builder

	if detailed {
		builder.WriteString(strings.Repeat("=", 80) + "\n")
		builder.WriteString(fmt.Sprintf("技能: %s\n", skill.Name))
		builder.WriteString(strings.Repeat("=", 80) + "\n")

		builder.WriteString(fmt.Sprintf("ID:               %d\n", skill.ID))
		builder.WriteString(fmt.Sprintf("名称:             %s\n", skill.Name))
		builder.WriteString(fmt.Sprintf("描述:             %s\n", skill.Description))
		builder.WriteString(fmt.Sprintf("分类:             %s\n", skill.Category))
		builder.WriteString(fmt.Sprintf("优先级:           %d\n", skill.Priority))
		builder.WriteString(fmt.Sprintf("全局技能:         %v\n", skill.IsGlobal))
		builder.WriteString(fmt.Sprintf("使用次数:         %d\n", skill.UsageCount))
		builder.WriteString(fmt.Sprintf("创建时间:         %s\n", skill.CreatedAt.Format("2006-01-02 15:04:05")))
		builder.WriteString(fmt.Sprintf("更新时间:         %s\n", skill.UpdatedAt.Format("2006-01-02 15:04:05")))

		builder.WriteString(strings.Repeat("-", 80) + "\n")
		builder.WriteString(formatSkillContent(skill.Content))
		builder.WriteString(strings.Repeat("=", 80) + "\n")
	} else {
		builder.WriteString(fmt.Sprintf("#%d [%s] %s\n", skill.ID, skill.Category, skill.Name))
		builder.WriteString(fmt.Sprintf("  描述: %s\n", skill.Description))
		builder.WriteString(fmt.Sprintf("  优先级: %d | 全局: %v | 使用次数: %d\n",
			skill.Priority, skill.IsGlobal, skill.UsageCount))
		builder.WriteString(fmt.Sprintf("  创建: %s | 更新: %s\n",
			skill.CreatedAt.Format("2006-01-02 15:04:05"),
			skill.UpdatedAt.Format("2006-01-02 15:04:05")))
		builder.WriteString("\n")
	}

	return builder.String()
}

// PrintSkill 打印技能信息
func PrintSkill(skill Skill, detailed bool) {
	fmt.Print(formatSkillDetails(&skill, detailed))
}

// LoadSkills 加载技能到系统提示词中
// 注意：此功能暂时未实现，保留接口
func LoadSkills(ctx context.Context) ([]Message, error) {
	return []Message{}, nil
}

// GetSkill 根据ID获取技能
func GetSkill(id int64) (*Skill, error) {
	db, err := OpenDB()
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
	db, err := OpenDB()
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
func CreateSkill(skill *Skill) error {
	db, err := OpenDB()
	if err != nil {
		return fmt.Errorf("打开数据库失败: %w", err)
	}
	defer db.Close()

	result, err := db.Exec(`
		INSERT INTO skills (name, description, content, category, priority, is_global)
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

// RecordSkillUsage 记录技能使用
func RecordSkillUsage(skillID int64, projectPath string) error {
	db, err := OpenDB()
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
		Println("更新项目技能关联失败:", err)
	}

	return nil
}

// handleSkillTool 处理Skill工具调用
func handleSkillTool(ctx context.Context, args ToolArgs) (string, error) {
	// 获取参数
	skillIDVal := ToolArgsValue[any](args, "skill_id", nil)
	skillName := ToolArgsValue[string](args, "skill_name", "")

	var skillID int64
	var err error

	// 处理skill_id参数
	if skillIDVal != nil {
		switch v := skillIDVal.(type) {
		case int:
			skillID = int64(v)
		case int64:
			skillID = v
		case float64: // JSON数字可能被解析为float64
			// 检查是否为整数
			if v != math.Trunc(v) {
				return "", fmt.Errorf("skill_id必须是整数，当前值: %v", v)
			}

			// 检查是否在int64范围内
			if v < math.MinInt64 || v > math.MaxInt64 {
				return "", fmt.Errorf("skill_id超出范围: %v", v)
			}

			skillID = int64(v)

			// 再次检查转换后的值是否为正数
			if skillID <= 0 {
				return "", fmt.Errorf("skill_id必须是正整数，当前值: %v", v)
			}
		default:
			return "", fmt.Errorf("skill_id必须是整数类型，当前类型: %T", skillIDVal)
		}
	}

	// 清理skill_name
	skillName = strings.TrimSpace(skillName)

	// 验证参数组合
	if skillID == 0 && skillName == "" {
		return "", fmt.Errorf("必须提供skill_id或skill_name参数")
	}

	if skillID > 0 && skillName != "" {
		Println("提示：同时提供了skill_id和skill_name，优先使用skill_id")
	}

	// 验证skill_name长度
	if skillName != "" {
		if len(skillName) < 2 {
			return "", fmt.Errorf("skill_name太短，至少2个字符")
		}
		if len(skillName) > 100 {
			return "", fmt.Errorf("skill_name太长，最多100个字符")
		}
	}

	if skillName == "" && skillID == 0 {
		return "", fmt.Errorf("skill_name不能为空字符串")
	}

	var skill *Skill

	if skillID > 0 {
		skill, err = GetSkill(skillID)
	} else {
		skill, err = GetSkillByName(skillName)
	}

	if err != nil {
		return "", fmt.Errorf("获取技能失败: %w", err)
	}

	if skill == nil {
		if skillID > 0 {
			return "", fmt.Errorf("找不到ID为 %d 的技能，请检查技能ID是否正确", skillID)
		} else {
			// 提供更友好的建议
			suggestion := ""
			if strings.Contains(skillName, " ") {
				suggestion = "（提示：技能名称中不要包含空格）"
			}
			return "", fmt.Errorf("找不到名称为 '%s' 的技能%s，请检查技能名称是否正确", skillName, suggestion)
		}
	}

	// 记录技能使用
	if err := RecordSkillUsage(skill.ID, ProjectRoot); err != nil {
		// 只记录日志，不中断操作
		Println("警告：记录技能使用失败:", err)
	}

	// 格式化输出
	return formatSkillDetails(skill, true), nil
}

func init() {
	// 注册Skill工具
	RegisterTool(ToolDef{
		Name:        "skill",
		DisplayName: "获取技能",
		Description: `根据ID或名称获取技能内容。技能包含最佳实践、技巧、规范等知识。

使用示例：
1. 通过ID获取：skill(skill_id=1)
2. 通过名称获取：skill(skill_name="Go最佳实践")

注意事项：
- skill_id和skill_name至少提供一个
- 如果同时提供，优先使用skill_id
- 技能名称区分大小写
- skill_id必须是正整数`,
		Parameters: map[string]any{
			"skill_id": map[string]any{
				"type":        "integer",
				"description": "技能ID（正整数）",
			},
			"skill_name": map[string]any{
				"type":        "string",
				"description": "技能名称（区分大小写）",
			},
		},
		Category: "skill",
		Handler:  handleSkillTool,
	})
}
