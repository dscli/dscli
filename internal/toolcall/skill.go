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
//
// LoadSkills 加载技能到系统提示词中。
// 从本地技能存储（项目 .dscli/skills）和全局技能存储（~/.dscli/skills）加载，
// 生成 system message 注入到对话中。
//
// 注入策略分两层：
//  1. auto_inject 技能：完整内容直接注入，LLM 无需主动获取
//  2. 手动技能：仅注入列表（名称、描述、关键词），LLM 按需通过 skill_by_name 获取
//
// 使用 `skill_search` 工具可按关键词搜索手动技能。
// LoadSkills loads skills into system prompt messages.
//
// It loads from local (.dscli/skills) and global (~/.dscli/skills) stores,
// merging with local priority. Two injection strategies:
//  1. auto_inject skills: full content injected directly, no LLM fetch needed
//  2. manual skills: name/description list only, LLM fetches via skill_by_name
//
// Use skill_search tool for keyword-based discovery of manual skills.
// Store loading errors are gracefully degraded to empty stores
// so that skill failures never block conversation.
func LoadSkills(ctx context.Context) (messages []Message, err error) {
	localStore, localErr := skills.LocalStore()
	if localErr != nil {
		// Degrade gracefully: continue with empty local store
		localStore = &skills.Store{}
	}

	globalStore, globalErr := skills.GlobalStore()
	if globalErr != nil {
		// Degrade gracefully: continue with empty global store
		globalStore = &skills.Store{}
	}

	// Merge skills: local takes priority over global
	allSkills := make(map[string]skills.Skill)
	for name, skill := range globalStore.Skills {
		allSkills[name] = skill
	}
	for name, skill := range localStore.Skills {
		allSkills[name] = skill
	}

	if len(allSkills) == 0 {
		return []Message{}, nil // No skills, no injection
	}

	// Sort for stable output
	names := make([]string, 0, len(allSkills))
	for name := range allSkills {
		names = append(names, name)
	}
	sort.Strings(names)

	// Separate auto_inject skills from manual ones
	var autoSkills, manualSkills []skills.Skill
	for _, name := range names {
		skill := allSkills[name]
		if skill.AutoInject {
			autoSkills = append(autoSkills, skill)
		} else {
			manualSkills = append(manualSkills, skill)
		}
	}

	// Cap manual skills listed to avoid token waste (names are enough to prompt fetch)
	const maxManualListed = 20
	hasMore := len(manualSkills) > maxManualListed
	if hasMore {
		manualSkills = manualSkills[:maxManualListed]
	}

	var builder strings.Builder

	// === Part 1: Auto-inject skills (full content) ===
	for _, skill := range autoSkills {
		builder.WriteString("---\n")
		fmt.Fprintf(&builder, "## Skill: %s (auto-loaded)\n\n", skill.Name)
		builder.WriteString(skill.Content)
		builder.WriteString("\n\n")
	}

	// === Part 2: Manual skill list ===
	if len(manualSkills) > 0 {
		builder.WriteString("## Available Skills\n\n")
		builder.WriteString("Fetch full content via `skill_by_name` tool, ")
		builder.WriteString("then execute scripts via `shell` or `python` tools.\n")
		builder.WriteString("Not sure which skill to use? Try `skill_search` with keywords.\n\n")
		builder.WriteString("| Name | Description | Keywords |\n")
		builder.WriteString("|------|-------------|----------|\n")

		for _, skill := range manualSkills {
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

		if hasMore {
			fmt.Fprintf(&builder, "| ... | _(%d more skills, use skill_search to discover)_ | ... |\n",
				len(names)-len(autoSkills)-maxManualListed)
		}
	}

	// === Part 3: Usage instructions with example ===
	builder.WriteString("\n### How to Use a Skill\n\n")
	builder.WriteString("1. Call `skill_by_name(skill_name=\"...\")` to fetch the full skill content\n")
	builder.WriteString("2. Read the SKILL.md content for instructions. Scripts are listed under `## Scripts` with paths.\n")
	builder.WriteString("3. To read a resource, use the path shown (e.g. `~/.dscli/skills/<name>/scripts/foo.sh`)\n")
	builder.WriteString("4. Execute scripts via `shell` or `python` tool:\n")
	builder.WriteString("   - Shell script: `bash <skill_path>/scripts/foo.sh <args>`\n")
	builder.WriteString("   - Python script: `python3 <skill_path>/scripts/bar.py <args>`\n")
	builder.WriteString("5. Reference documents and templates can be read with `read_file` or `cat` via shell\n\n")
	builder.WriteString("**Example** — user says \"run tests\", you see `test-skill` in the list:\n")
	builder.WriteString("```\n")
	builder.WriteString("Step 1: skill_by_name(skill_name=\"test-skill\")\n")
	builder.WriteString("Step 2: Find the script path in the returned content (e.g. ~/.dscli/skills/test-skill/scripts/test.sh)\n")
	builder.WriteString("Step 3: Execute via shell tool: bash ~/.dscli/skills/test-skill/scripts/test.sh\n")
	builder.WriteString("```\n")
	builder.WriteString("**IMPORTANT**: Never guess or fabricate scripts. Always fetch the skill first.\n")

	messages = []Message{{
		Role:    "system",
		Content: builder.String(),
	}}

	return messages, nil
}

// truncateSkillDesc truncates a skill description to maxLen runes, appending "..." if needed.
func truncateSkillDesc(desc string, maxLen int) string {
	if maxLen < 3 {
		maxLen = 3 // guard against negative slice index
	}
	runes := []rune(desc)
	if len(runes) <= maxLen {
		return desc
	}
	return string(runes[:maxLen-3]) + "..."
}
