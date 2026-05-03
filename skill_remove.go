package main

import (
	"fmt"
	"os"
	"path/filepath"

	icontext "gitcode.com/dscli/dscli/internal/context"
	"github.com/spf13/cobra"
)

func init() {
	removeCmd := &cobra.Command{
		Use:   "remove <skill-name>",
		Short: "移除指定技能",
		Long: `从本地或全局技能目录移除指定技能。

默认从本地（项目）技能目录移除；使用 --global/-g 从全局目录移除。

注意：移除后 skills.yaml 不会立即更新，下次加载时会自动重建。
如需立即生效，可手动删除 skills.yaml 文件或重新运行 skill list。

示例：
  dscli skill remove ascend-docker
  dscli skill remove ascend-docker --global`,
		Args: cobra.ExactArgs(1),
		RunE: runSkillRemove,
	}

	removeCmd.Flags().BoolP("global", "g", false, "从全局技能目录移除（默认从本地）")
	skillCmd.AddCommand(removeCmd)
}

func runSkillRemove(cmd *cobra.Command, args []string) error {
	skillName := args[0]
	global, _ := cmd.Flags().GetBool("global")

	// 确定技能目录
	var skillsDir string
	if global {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("获取用户主目录失败: %w", err)
		}
		skillsDir = filepath.Join(home, ".dscli", "skills")
	} else {
		skillsDir = filepath.Join(icontext.ProjectRoot, ".dscli", "skills")
	}

	// 技能路径
	skillDir := filepath.Join(skillsDir, skillName)

	// 检查是否存在
	info, err := os.Stat(skillDir)
	if os.IsNotExist(err) {
		scope := "本地"
		if global {
			scope = "全局"
		}
		return fmt.Errorf("技能 %q 在 %s 技能目录中不存在", skillName, scope)
	}
	if err != nil {
		return fmt.Errorf("访问技能目录失败: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s 不是目录", skillDir)
	}

	// 确认 SKILL.md 存在（额外安全检查）
	skillMDPath := filepath.Join(skillDir, "SKILL.md")
	if _, err := os.Stat(skillMDPath); os.IsNotExist(err) {
		return fmt.Errorf("技能目录中未找到 SKILL.md: %s", skillDir)
	}

	// 删除技能目录
	if err := os.RemoveAll(skillDir); err != nil {
		return fmt.Errorf("移除技能目录失败: %w", err)
	}

	scope := "本地"
	if global {
		scope = "全局"
	}
	fmt.Printf("✅ 已从 %s 技能目录移除 %q\n", scope, skillName)

	// 如果 skills.yaml 存在，删除之，强制下次 Load 时重建
	yamlPath := filepath.Join(skillsDir, "skills.yaml")
	if _, err := os.Stat(yamlPath); err == nil {
		if err := os.Remove(yamlPath); err != nil {
			fmt.Printf("⚠️  无法删除 skills.yaml（不影响功能，下次加载时会重建）: %v\n", err)
		} else {
			fmt.Printf("   skills.yaml 已删除，下次加载时将自动重建。\n")
		}
	}

	return nil
}
