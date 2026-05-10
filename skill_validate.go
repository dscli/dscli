package main

import (
	"fmt"
	"os"

	"gitcode.com/dscli/dscli/internal/skills"
	"github.com/spf13/cobra"
)

func init() {
	validateCmd := &cobra.Command{
		Use:   "validate <path|skill-name>",
		Short: "校验技能目录是否符合 Agent Skills 规范",
		Long: `校验指定的技能目录或技能名称是否符合 Agent Skills 规范。

接受技能目录路径或技能名称。给定名称时，从本地和全局 store 中查找该技能
并校验其目录。

检查项包括：
  - 目录存在且可访问
  - SKILL.md 文件存在
  - YAML frontmatter 格式正确
  - name 和 description 必填字段
  - 名称格式：小写、仅字母/数字/连字符、最多 64 字符、无首尾或连续连字符
  - 目录名与技能名一致（NFKC 规范化后）
  - description 不超过 1024 字符
  - compatibility 若存在须为字符串且不超过 500 字符
  - 仅允许规范定义的 6 个 frontmatter 字段

退出码为 1 时表示校验失败（存在一个或多个错误）。

示例：
  dscli skill validate ./my-skill
  dscli skill validate go-fix
  dscli skill validate ~/src/agent-skills/skills/go-fix`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			arg := args[0]

			// 1. Try as directory path first
			if info, err := os.Stat(arg); err == nil && info.IsDir() {
				errs := skills.ValidateSkillDir(arg)
				if len(errs) == 0 {
					fmt.Println("✅ 技能目录校验通过")
					return nil
				}
				fmt.Println("❌ 技能目录校验失败:")
				for _, e := range errs {
					fmt.Printf("   - %s\n", e)
				}
				return fmt.Errorf("发现 %d 个问题", len(errs))
			}

			// 2. Try as skill name — resolve to directory via local/global store
			dir := skills.ResolveSkillDir(arg)
			if dir == "" {
				return fmt.Errorf("not a valid directory or skill name: %q", arg)
			}

			errs := skills.ValidateSkillDir(dir)
			if len(errs) == 0 {
				fmt.Printf("✅ 技能 %q 校验通过 (%s)\n", arg, dir)
				return nil
			}
			fmt.Printf("❌ 技能 %q 校验失败 (%s):\n", arg, dir)
			for _, e := range errs {
				fmt.Printf("   - %s\n", e)
			}
			return fmt.Errorf("发现 %d 个问题", len(errs))
		},
	}
	skillCmd.AddCommand(validateCmd)
}
