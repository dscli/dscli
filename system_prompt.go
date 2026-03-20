package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gitcode.com/dscli/dscli/internal/context"
)

// SystemPromptConfig 系统提示词配置
type SystemPromptConfig struct {
	// 基础信息
	CurrentDate string
	ProjectRoot string
	ConfigDir   string

	// Git信息
	GitUserName  string
	GitUserEmail string
	GitBranch    string
	GitStatus    string

	// 项目信息
	ProjectName string
	ProjectType string

	// 环境信息
	WorkingDirectory string
	Hostname         string
	Username         string

	// 模型特定配置
	ModelID int64
}

// NewSystemPromptConfig 创建系统提示词配置
func NewSystemPromptConfig(ctx context.Context) *SystemPromptConfig {
	modelID := context.ContextValue(ctx, context.CurrentModelIDKey, int64(0))
	config := &SystemPromptConfig{
		CurrentDate:      time.Now().Format("2006年01月02日"),
		ProjectRoot:      ProjectRoot,
		ConfigDir:        ConfigDir,
		WorkingDirectory: getWorkingDirectory(),
		Hostname:         getHostname(),
		Username:         getUsername(),
		ModelID:          modelID,
	}

	// 获取Git信息
	config.loadGitInfo()

	// 获取项目信息
	config.loadProjectInfo()

	return config
}

// getWorkingDirectory 获取工作目录
func getWorkingDirectory() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "未知"
	}
	return cwd
}

// getHostname 获取主机名
func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "未知"
	}
	return hostname
}

// getUsername 获取用户名
func getUsername() string {
	return os.Getenv("USER")
}

// loadGitInfo 加载Git信息
func (c *SystemPromptConfig) loadGitInfo() {
	// 获取Git用户名
	if output, err := exec.Command("git", "config", "user.name").Output(); err == nil {
		c.GitUserName = strings.TrimSpace(string(output))
	}

	// 获取Git邮箱
	if output, err := exec.Command("git", "config", "user.email").Output(); err == nil {
		c.GitUserEmail = strings.TrimSpace(string(output))
	}

	// 获取当前分支
	if output, err := exec.Command("git", "branch", "--show-current").Output(); err == nil {
		c.GitBranch = strings.TrimSpace(string(output))
	}

	// 获取Git状态（简化版）
	if output, err := exec.Command("git", "status", "--porcelain").Output(); err == nil {
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		if len(lines) > 0 && lines[0] != "" {
			c.GitStatus = fmt.Sprintf("有%d个文件变更", len(lines))
		} else {
			c.GitStatus = "工作区干净"
		}
	}
}

// loadProjectInfo 加载项目信息
func (c *SystemPromptConfig) loadProjectInfo() {
	// 从项目根目录获取项目名称
	c.ProjectName = filepath.Base(c.ProjectRoot)

	// 检测项目类型
	c.ProjectType = c.detectProjectType()
}

// detectProjectType 检测项目类型
func (c *SystemPromptConfig) detectProjectType() string {
	// 检查是否有go.mod文件
	if _, err := os.Stat(filepath.Join(c.ProjectRoot, "go.mod")); err == nil {
		return "Go项目"
	}

	// 检查是否有package.json文件
	if _, err := os.Stat(filepath.Join(c.ProjectRoot, "package.json")); err == nil {
		return "Node.js项目"
	}

	// 检查是否有requirements.txt文件
	if _, err := os.Stat(filepath.Join(c.ProjectRoot, "requirements.txt")); err == nil {
		return "Python项目"
	}

	// 检查是否有Cargo.toml文件
	if _, err := os.Stat(filepath.Join(c.ProjectRoot, "Cargo.toml")); err == nil {
		return "Rust项目"
	}

	// 检查是否有Makefile文件
	if _, err := os.Stat(filepath.Join(c.ProjectRoot, "Makefile")); err == nil {
		return "C/C++项目"
	}

	return "通用项目"
}

// GeneratePrompt 生成系统提示词（已弃用，请使用GeneratePromptWithTemplate）
func (c *SystemPromptConfig) GeneratePrompt() string {
	// 已弃用：重定向到模板版本以保持向后兼容
	return c.GeneratePromptWithTemplate()
}

// GetEnhancedSystemPrompt 获取增强的系统提示词
func GetEnhancedSystemPrompt(ctx context.Context) string {
	config := NewSystemPromptConfig(ctx)
	return config.GeneratePromptWithTemplate()
}

// LoadEnhancedPrompts 加载增强的提示词
func LoadEnhancedPrompts(ctx context.Context) ([]Message, error) {
	return []Message{{
		Role:    "system",
		Content: GetEnhancedSystemPrompt(ctx),
	}}, nil
}

// GetSystemPromptConfig 获取系统提示词配置
func GetSystemPromptConfig(ctx context.Context) *SystemPromptConfig {
	return NewSystemPromptConfig(ctx)
}

// GetTemplateSystemPrompt 获取模板化的系统提示词（兼容旧代码）
func GetTemplateSystemPrompt(ctx context.Context) string {
	// 获取当前项目的领域ID
	domainID := GetCurrentDomainID(ctx)

	// 获取当前模型ID
	modelID := GetCurrentModelID(ctx)

	// 获取系统提示词配置
	config := NewSystemPromptConfig(ctx)

	// 使用段落管理器渲染系统提示词
	sm := &SegmentManager{}
	prompt, err := sm.RenderSystemPrompt(ctx, modelID, domainID, config)
	if err != nil || prompt == "" {
		// 如果失败或为空，使用模板化的系统提示词
		return GetEnhancedSystemPromptWithTemplate(ctx)
	}

	return prompt
}
