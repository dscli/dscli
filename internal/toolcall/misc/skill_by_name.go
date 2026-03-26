package misc

import (
	"fmt"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/outfmt"
)

// handleSkillByName 处理Skill工具调用
func handleSkillByName(ctx context.Context, args ToolArgs) (content string, user string, err error) {
	// 获取参数
	skillName := ToolArgsValue(args, "skill_name", "")
	if skillName == "" {
		err = fmt.Errorf("skill name can not be empty")
		return
	}
	outfmt.Printf("getting skill by name [%s]\n", skillName)
	skill, err := GetSkillByName(skillName)
	if err != nil {
		err = fmt.Errorf("no skill found for %s: %w", skillName, err)
		return
	}

	if skill == nil {
		err = fmt.Errorf("no skill found for %s", skillName)
		return
	}
	projectRoot := context.ProjectRoot
	// 异步记录技能使用
	SafeAsyncRecordUsage(skill.ID, projectRoot)
	// 格式化输出

	content = skill.Content
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
