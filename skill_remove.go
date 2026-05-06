package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

	scope := "本地"
	if global {
		scope = "全局"
	}

	// 技能路径
	skillDir := filepath.Join(skillsDir, skillName)

	// 检查目录是否存在
	info, err := os.Stat(skillDir)
	if os.IsNotExist(err) {
		// 目录不存在，检查是否为缓存中的僵尸条目（目录已删除但 skills.yaml 未更新）
		if isStaleCacheEntry(skillName, skillsDir) {
			// 删除 skills.yaml 强制重建，清除僵尸条目
			yamlPath := filepath.Join(skillsDir, "skills.yaml")
			if removeErr := os.Remove(yamlPath); removeErr != nil && !os.IsNotExist(removeErr) {
				return fmt.Errorf("清理缓存文件失败: %w", removeErr)
			}
			fmt.Printf("✅ 已清理 %s 技能缓存中的僵尸条目 %q\n", scope, skillName)
			fmt.Printf("   skills.yaml 已删除，下次加载时将自动重建。\n")
			return nil
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

// isStaleCacheEntry checks if a skill is listed in skills.yaml but its
// directory is missing on disk — a "zombie" entry from manual deletion.
// It reads the yaml directly (bypassing the store) to avoid triggering
// Load's auto-clean which would remove the evidence before we can check.
func isStaleCacheEntry(skillName string, skillsDir string) bool {
	yamlPath := filepath.Join(skillsDir, "skills.yaml")
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return false
	}
	// Skill entries in skills.yaml appear as "  <name>:" top-level keys.
	// A simple substring match is sufficient for this diagnostic check.
	return strings.Contains(string(data), "\n  "+skillName+":")
}
