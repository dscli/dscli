package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	icontext "gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/skills"
	"github.com/spf13/cobra"
)

func init() {
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
	skillCmd.AddCommand(addCmd)
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
		return filepath.Join(icontext.ProjectRoot, ".dscli", "skills"), nil
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
