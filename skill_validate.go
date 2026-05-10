package main

import (
	"fmt"

	"gitcode.com/dscli/dscli/internal/skills"
	"github.com/spf13/cobra"
)

func init() {
	validateCmd := &cobra.Command{
		Use:   "validate <path>",
		Short: "校验技能目录是否符合 Agent Skills 规范",
		Long: `校验指定的技能目录是否符合 Agent Skills 规范。

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
  dscli skill validate ~/src/agent-skills/skills/go-fix`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			errors := skills.ValidateSkillDir(path)
			if len(errors) == 0 {
				fmt.Println("✅ 技能目录校验通过")
				return nil
			}
			fmt.Println("❌ 技能目录校验失败:")
			for _, e := range errors {
				fmt.Printf("   - %s\n", e)
			}
			return fmt.Errorf("发现 %d 个问题", len(errors))
		},
	}
	skillCmd.AddCommand(validateCmd)
}
