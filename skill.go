package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/skills"
	"github.com/spf13/cobra"
)

var skillCmd *cobra.Command

func init() {
	skillCmd = AddRootCommand(&cobra.Command{
		Use:   "skill",
		Short: "技能管理 - 安装、显示和查询技能",
		Long: `skill 命令用于管理和使用技能。

技能是预定义的代码片段或模板，可以快速复用。
支持本地技能（项目目录）和全局技能（用户配置目录）。`,
	})

	// show 子命令
	AddCommand(skillCmd, &cobra.Command{
		Use:   "show <skill name>",
		Short: "显示指定名称的技能",
		Long: `显示指定名称的技能。

技能名称可以是本地技能或全局技能。
本地技能优先于全局技能。`,
		Args: cobra.ExactArgs(1),
		RunE: skillShowRunE,
	})

	// query 子命令
	AddCommand(skillCmd, &cobra.Command{
		Use:   "query <query>",
		Short: "查询匹配关键词的技能",
		Long: `查询匹配关键词的技能。

查询会匹配技能的关键词，返回所有匹配的技能摘要。
关键词匹配是大小写不敏感的。`,
		Args: cobra.ExactArgs(1),
		RunE: skillQueryRunE,
	})

	// list 子命令
	AddCommand(skillCmd, &cobra.Command{
		Use:   "list",
		Short: "列出所有可用技能",
		Long: `列出所有可用技能。

显示本地和全局技能列表，按名称排序。
本地技能优先于全局技能（同名时只显示本地技能）。`,
		Args: cobra.NoArgs,
		RunE: skillListRunE,
	})

	// set-auto-inject 子命令
	setAutoInjectCmd := AddCommand(skillCmd, &cobra.Command{
		Use:   "set-auto-inject <name> <true|false>",
		Short: "设置技能的自动注入属性",
		Long: `设置技能的 auto_inject 属性。

当 auto_inject 为 true 时，技能内容会自动注入到每次对话的上下文中，无需 LLM 主动获取。
当为 false 时，技能仅在 skill 列表中展示，LLM 需要时才通过 skill_by_name 获取。

默认修改本地技能；使用 --global/-g 标志修改全局技能。`,
		Args: cobra.ExactArgs(2),
		RunE: skillSetAutoInjectRunE,
	})

	setAutoInjectCmd.Flags().BoolP("global", "g", false, "修改全局技能而非本地技能")

	AddCommand(skillCmd, &cobra.Command{
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
		RunE: skillValidateRunE,
	})

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
	AddCommand(skillCmd, removeCmd)

	addCmd := &cobra.Command{
		Use:   "add <source>",
		Short: "安装技能到本地或全局技能目录",
		Long: `安装技能到指定目标。

source 为技能目录路径（包含 SKILL.md 的目录）。

目标选项（互斥）：
  --target=global        安装到全局技能目录（~/.dscli/skills）
  --target=local         安装到当前项目技能目录（.dscli/skills）
  --target=<project>     安装到指定项目的技能目录（<project>/.dscli/skills）

默认行为：安装到当前项目技能目录。

示例：
  dscli skill add ~/src/agent-skills/skills/ascend-docker --target=global
  dscli skill add ~/src/agent-skills/skills/ascend-docker --target=local
  dscli skill add ascend-docker --target=~/src/gitcode.com/dscli/dscli`,
		Args: cobra.ExactArgs(1),
		RunE: runSkillAdd,
	}

	addCmd.Flags().String("target", "local", "安装目标：global、local 或项目路径")
	AddCommand(skillCmd, addCmd)

}

func skillValidateRunE(cmd *cobra.Command, args []string) error {
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
		return nil
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
	return nil
}

// skillListRunE 列出所有技能
func skillListRunE(cmd *cobra.Command, args []string) error {
	skillInfos, err := skills.ListAll()
	if err != nil {
		return fmt.Errorf("列出技能失败: %w", err)
	}

	if len(skillInfos) == 0 {
		fmt.Fprint(os.Stderr, "没有找到任何技能。\n")
		return nil
	}

	// 转换为map数组，以便使用FormatOutput
	var skillMaps []map[string]string
	for _, info := range skillInfos {
		autoInject := "-"
		if info.AutoInject {
			autoInject = "是"
		}
		skillMaps = append(skillMaps, map[string]string{
			"name":        info.Name,
			"scope":       info.Scope,
			"auto_inject": autoInject,
		})
	}

	// 使用FormatOutput进行格式化输出
	headers := []string{"名称", "范围", "自动注入"}
	rowFunc := func(data any) []string {
		switch info := data.(type) {
		case map[string]string:
			return []string{info["name"], info["scope"], info["auto_inject"]}
		default:
			return []string{"", "", ""}
		}
	}

	// 使用默认的table格式
	err = FormatOutput(skillMaps, "table", headers, rowFunc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "格式化输出失败: %v\n", err)
		os.Exit(1)
	}

	return nil
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
		skillsDir = filepath.Join(context.ProjectRoot, ".dscli", "skills")
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
func isStaleCacheEntry(skillName, skillsDir string) bool {
	yamlPath := filepath.Join(skillsDir, "skills.yaml")
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return false
	}
	// Skill entries in skills.yaml appear as "  <name>:" top-level keys.
	// A simple substring match is sufficient for this diagnostic check.
	return strings.Contains(string(data), "\n  "+skillName+":")
}

func runSkillAdd(cmd *cobra.Command, args []string) error {
	sourcePath := args[0]
	target, _ := cmd.Flags().GetString("target")

	// 1. 解析源路径为绝对路径（支持 ~ 展开）
	absSource, err := resolvePath(sourcePath)
	if err != nil {
		return fmt.Errorf("解析源路径失败: %w", err)
	}

	// 2. 检查源目录是否存在（os.Stat 会跟随符号链接）
	srcInfo, err := os.Stat(absSource)
	if err != nil {
		return fmt.Errorf("源路径不存在: %w", err)
	}
	if !srcInfo.IsDir() {
		return fmt.Errorf("源路径不是目录: %s", absSource)
	}

	// 2.5 解析符号链接，获取真实路径（filepath.WalkDir 不跟随根符号链接）
	if absSource, err = filepath.EvalSymlinks(absSource); err != nil {
		return fmt.Errorf("解析源路径符号链接失败: %w", err)
	}

	// 3. 查找 SKILL.md
	skillMDPath := filepath.Join(absSource, "SKILL.md")
	if _, err := os.Stat(skillMDPath); os.IsNotExist(err) {
		return fmt.Errorf("源目录中未找到 SKILL.md: %s", absSource)
	}

	// 4. 解析技能名称
	var skill skills.Skill
	if err := skills.ParseSkill(skillMDPath, &skill); err != nil {
		return fmt.Errorf("解析 SKILL.md 失败: %w", err)
	}
	if skill.Name == "" {
		return fmt.Errorf("SKILL.md 中缺少 name 字段")
	}

	// 5. 确定目标目录
	targetDir, err := resolveSkillTarget(target)
	if err != nil {
		return err
	}

	// 6. 确保目标目录存在
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("创建目标目录失败: %w", err)
	}

	// 7. 目标技能目录
	destDir := filepath.Join(targetDir, skill.Name)

	// 8. 检查是否已存在
	if info, err := os.Stat(destDir); err == nil {
		if info.IsDir() {
			return fmt.Errorf("skill %q already exists at %s, remove it first with 'dscli skill remove %s'",
				skill.Name, destDir, skill.Name)
		}
		return fmt.Errorf("path %s exists but is not a directory", destDir)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check target directory failed: %w", err)
	}

	// 9. 复制目录（失败时清理残留）
	if err := copyDir(absSource, destDir); err != nil {
		os.RemoveAll(destDir) // 清理不完全的复制
		return fmt.Errorf("copy skill directory failed: %w", err)
	}

	fmt.Printf("✅ 技能 %q 已安装到 %s\n", skill.Name, destDir)
	fmt.Printf("   skills.yaml 将在下次加载时自动更新。\n")
	return nil
}

// resolveSkillTarget 解析 --target 参数，返回目标技能目录。
// 支持 "global"、"local"、或具体项目路径（含 ~ 展开）。
func resolveSkillTarget(target string) (string, error) {
	switch target {
	case "global":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("获取用户主目录失败: %w", err)
		}
		return filepath.Join(home, ".dscli", "skills"), nil
	case "local":
		return filepath.Join(context.ProjectRoot, ".dscli", "skills"), nil
	default:
		// 作为项目路径处理（支持 ~ 展开）
		absTarget, err := resolvePath(target)
		if err != nil {
			return "", fmt.Errorf("解析目标路径失败: %w", err)
		}
		return filepath.Join(absTarget, ".dscli", "skills"), nil
	}
}

// resolvePath 解析路径：展开 ~，转换为绝对路径。
func resolvePath(p string) (string, error) {
	if strings.HasPrefix(p, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("获取用户主目录失败: %w", err)
		}
		p = filepath.Join(home, p[1:])
	}
	return filepath.Abs(p)
}

// copyDir 递归复制目录。支持符号链接：目录符号链接仅创建目录本身，
// 普通文件和指向文件的符号链接按内容复制。
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// 计算相对路径
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(dst, relPath)

		// WalkDir 使用 Lstat 判断类型，对符号链接返回非目录。
		// 需要用 os.Stat 跟随符号链接来正确判断目标类型。
		fi, statErr := os.Stat(path)
		if statErr == nil && fi.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}

		// 符号链接到目录：WalkDir 不会递归进入，只创建目录本身
		if d.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}

		// 检查是否为符号链接（Lstat 模式）
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("获取 %s 文件信息失败: %w", path, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			// 保留符号链接：读取目标 → 在目标位置创建相同指向的符号链接
			linkTarget, readErr := os.Readlink(path)
			if readErr != nil {
				return fmt.Errorf("读取符号链接 %s 失败: %w", path, readErr)
			}
			return os.Symlink(linkTarget, targetPath)
		}

		// 普通文件：按内容复制
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("读取 %s 失败: %w", path, err)
		}

		if err := os.WriteFile(targetPath, data, info.Mode().Perm()); err != nil {
			return fmt.Errorf("写入 %s 失败: %w", targetPath, err)
		}

		return nil
	})
}

func skillShowRunE(cmd *cobra.Command, args []string) error {
	skillName := args[0]
	content, err := skills.Use(skillName)
	if err != nil {
		return fmt.Errorf("使用技能失败: %w", err)
	}

	outfmt.Print(content)
	return nil
}

func skillQueryRunE(cmd *cobra.Command, args []string) error {
	query := args[0]
	result, err := skills.Query(query)
	if err != nil {
		return fmt.Errorf("查询技能失败: %w", err)
	}

	outfmt.Print(result)
	return nil
}

func skillSetAutoInjectRunE(cmd *cobra.Command, args []string) error {
	name := args[0]
	val := args[1]
	var autoInject bool
	switch val {
	case "true":
		autoInject = true
	case "false":
		autoInject = false
	default:
		return fmt.Errorf("无效的值 %q，必须为 true 或 false", val)
	}

	global, _ := cmd.Flags().GetBool("global")

	if err := skills.SetAutoInject(name, autoInject, global); err != nil {
		return fmt.Errorf("设置 auto_inject 失败: %w", err)
	}

	scope := "本地"
	if global {
		scope = "全局"
	}
	fmt.Printf("已将 %s 技能 %q 的 auto_inject 设置为 %v\n", scope, name, autoInject)
	return nil
}