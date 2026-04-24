package misc

import (
	"context"
	"fmt"

	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/skills"
)

// handleSkillByName 处理Skill工具调用
func handleSkillByName(ctx context.Context, args ToolArgs) (content string, user string, err error) {
	// 获取参数
	skillName := ToolArgsValue(args, "skill_name", "")
	if skillName == "" {
		err = fmt.Errorf("skill name can not be empty")
		return
	}
	outfmt.Printf("获取技能 [%s]\n", skillName)

	// 使用 markdown 技能系统获取技能内容
	skillContent, err := skills.Use(skillName)
	if err != nil {
		err = fmt.Errorf("获取技能 %s 失败: %w", skillName, err)
		return
	}

	content = skillContent
	return
}

func init() {
	// 注册Skill工具
	RegisterTool(ToolDef{
		Name:        "skill_by_name",
		DisplayName: "获取技能",
		Description: `根据Skill名称获取技能内容。技能包含最佳实践、技巧、规范等知识。

使用示例：
1. 通过名称获取：skill_by_name(skill_name="Go最佳实践")

注意事项：
- skill_name长度2-100字符，区分大小写`,
		Strict: true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"skill_name": map[string]any{
					"type":        "string",
					"description": "技能名称（区分大小写）",
					"pattern":     TitleLikePattern(128),
				},
			},
			"required":             []string{"skill_name"},
			"additionalProperties": false,
		},
		Category: "skill",
		Handler:  handleSkillByName,
	})
}
