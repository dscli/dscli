package toolcall

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"gitcode.com/dscli/dscli/internal/skills"
)

// LoadSkills 加载技能到系统提示词中。
// 从本地技能存储（项目 .dscli/skills）和全局技能存储（~/.dscli/skills）加载，
// 生成 system message 注入到对话中。
//
// 注入两部分信息：
//  1. 可用技能列表（名称、描述、关键词）
//  2. 如何通过 skill_by_name 工具和 shell/python 工具使用技能
func LoadSkills(ctx context.Context) (messages []Message, err error) {
	localStore, err := skills.LocalStore()
	if err != nil {
		return nil, fmt.Errorf("failed to load local skill store: %w", err)
	}

	globalStore, err := skills.GlobalStore()
	if err != nil {
		return nil, fmt.Errorf("failed to load global skill store: %w", err)
	}

	// 合并技能：本地优先，同名时覆盖全局
	allSkills := make(map[string]skills.Skill)
	for name, skill := range globalStore.Skills {
		allSkills[name] = skill
	}
	for name, skill := range localStore.Skills {
		allSkills[name] = skill
	}

	if len(allSkills) == 0 {
		return nil, nil // 没有技能时不注入
	}

	// 按名称排序以保证输出稳定
	names := make([]string, 0, len(allSkills))
	for name := range allSkills {
		names = append(names, name)
	}
	sort.Strings(names)

	var builder strings.Builder
	builder.WriteString("## 可用技能\n\n")
	builder.WriteString("以下技能可通过 `skill_by_name` 工具获取详细内容，")
	builder.WriteString("技能中的脚本可通过 `shell` 或 `python` 工具执行。\n\n")
	builder.WriteString("| 名称 | 描述 | 关键词 |\n")
	builder.WriteString("|------|------|--------|\n")

	for _, name := range names {
		skill := allSkills[name]
		keywords := "-"
		if len(skill.Keywords) > 0 {
			keywords = strings.Join(skill.Keywords, ", ")
		}
		fmt.Fprintf(&builder, "| %s | %s | %s |\n",
			skill.Name,
			truncateSkillDesc(skill.Description, 80),
			keywords,
		)
	}

	builder.WriteString("\n### 使用技能的方法\n\n")
	builder.WriteString("1. 首先使用 `skill_by_name` 工具获取技能完整内容\n")
	builder.WriteString("2. 从技能内容中提取相关脚本代码\n")
	builder.WriteString("3. 使用 `shell` 或 `python` 工具执行脚本\n")

	messages = []Message{{
		Role:    "system",
		Content: builder.String(),
	}}

	return messages, nil
}

// truncateSkillDesc 截断过长的技能描述
func truncateSkillDesc(desc string, maxLen int) string {
	runes := []rune(desc)
	if len(runes) <= maxLen {
		return desc
	}
	return string(runes[:maxLen-3]) + "..."
}
