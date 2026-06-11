// Package config 提供 dscli 配置管理，包括全局配置目录 (~/.dscli)
// 和项目级 .dscli/ 目录的初始化工。
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// gitignoreContent 是 .dscli/.gitignore 的默认内容。
// 仅忽略生成产物（运行时锁文件、技能缓存索引），
// 保留用户创建的内容（SKILL.md、prompt、dscli.md 等）。
const gitignoreContent = `# dscli 运行时与缓存文件（自动生成，勿手动编辑）
locks
skills/skills.yaml
`

// EnsureProjectGitignore 确保项目级 .dscli/.gitignore 存在。
//
// projectRoot 是项目的根目录（通常为 context.ProjectRoot）。
//
// 策略：
//   - 仅当 .dscli/ 目录已存在时工作（不因创建 .gitignore 而反创建父目录）
//   - 如果 .gitignore 已存在，跳过（保留用户自定义修改）
//   - 幂等，可安全重复调用
func EnsureProjectGitignore(projectRoot string) error {
	dscliDir := filepath.Join(projectRoot, ".dscli")

	// 1. 仅当 .dscli/ 已存在时操作
	dscliInfo, err := os.Stat(dscliDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 目录不存在，不创建
		}
		return fmt.Errorf("stat .dscli: %w", err)
	}
	if !dscliInfo.IsDir() {
		return nil // 不是目录（异常情况），不操作
	}

	// 2. 检查 .gitignore 是否已存在
	gitignorePath := filepath.Join(dscliDir, ".gitignore")
	if _, err := os.Stat(gitignorePath); err == nil {
		return nil // 已存在，保留用户修改
	}

	// 3. 创建 .gitignore
	if err := os.WriteFile(gitignorePath, []byte(gitignoreContent), 0o644); err != nil {
		return fmt.Errorf("write .dscli/.gitignore: %w", err)
	}
	return nil
}
