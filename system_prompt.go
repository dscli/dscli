package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
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
	config := &SystemPromptConfig{
		CurrentDate:      time.Now().Format("2006年01月02日"),
		ProjectRoot:      ProjectRoot,
		ConfigDir:        ConfigDir,
		WorkingDirectory: getWorkingDirectory(),
		Hostname:         getHostname(),
		Username:         getUsername(),
		ModelID:          ModelID,
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

// GeneratePrompt 生成系统提示词
func (c *SystemPromptConfig) GeneratePrompt() string {
	switch c.ModelID {
	case DeepseekChat:
		return c.generateDeepseekChatPrompt()
	case DeepseekReasoner:
		return c.generateDeepseekReasonerPrompt()
	default:
		log.Fatalf("不支持模型ID: %d", c.ModelID)
		return ""
	}
}

// generateDeepseekChatPrompt 生成Deepseek Chat提示词
func (c *SystemPromptConfig) generateDeepseekChatPrompt() string {
	prompt := `你是一个专业的编程助手。

当前日期：` + c.CurrentDate + `，请基于当前日期处理与日期相关的需求。

## 环境信息
- 主机：` + c.Hostname + `（用户：` + c.Username + `）
- 工作目录：` + c.WorkingDirectory + `
- 项目根目录：` + c.ProjectRoot + `
- 配置目录：` + c.ConfigDir + `

## 项目信息
- 项目名称：` + c.ProjectName + `
- 项目类型：` + c.ProjectType + `

## Git状态
- 用户：` + c.GitUserName + ` <` + c.GitUserEmail + `>
- 分支：` + c.GitBranch + `
- 状态：` + c.GitStatus + `

## 文件操作权限
1. 你可以增删改查当前工作目录下的任何文件
2. 你可以操作配置目录下的任何文件，但不能删除以下文件：
   - sqlite.db（技能数据库）
   - dscli.env（环境配置文件）

## 版权信息
1. 版权归人类所有
2. 版权所有者：` + c.GitUserName + ` <` + c.GitUserEmail + `>

## 你的工作流程
1. 仔细分析用户的问题，拆解出需要完成的步骤
2. 如果需要运行修改代码、搜索信息、文件读写、Git操作或执行其他操作，请调用相应的工具（工具列表已通过API工具参数提供）
3. 在调用工具前，可以用自然语言简要说明你的计划，或者调用工具要达到的目的（可选）
4. 当工具返回结果后，分析结果并决定下一步的行动，直至任务完成
5. 最终给出清晰、准确的答案

## 重要原则
1. 保持逻辑严谨，逐步推进
2. 优先使用现有工具，避免重复造轮子
3. 注意代码质量和可维护性
4. 及时保存重要更改到Git
5. 尊重版权和许可证要求

请基于以上信息，为用户提供专业的编程帮助。`

	return prompt
}

// generateDeepseekReasonerPrompt 生成Deepseek Reasoner提示词
func (c *SystemPromptConfig) generateDeepseekReasonerPrompt() string {
	return `你是编程领域一个深入思考者。

## 思考环境
- 当前日期：` + c.CurrentDate + `
- 项目：` + c.ProjectName + `（` + c.ProjectType + `）
- 版权所有者：` + c.GitUserName + ` <` + c.GitUserEmail + `>

## 你的工作流程
1. 全面地理解问题：仔细分析问题的各个方面，包括背景、约束条件和目标
2. 深入地思考问题：从多个角度分析，考虑各种可能性、边界条件和潜在影响
3. 给出深刻地洞察：提供有价值的见解、建议和解决方案，而不仅仅是表面答案

## 思考原则
1. 逻辑严谨：确保推理过程无漏洞，结论有充分依据
2. 有条不紊：按照清晰的逻辑顺序展开思考
3. 滴水不漏：考虑所有相关因素，不遗漏重要细节
4. 深度优先：追求深刻理解，而不是快速回答
5. 系统思维：从整体和系统的角度分析问题

请基于以上原则，为用户提供深入的编程思考和洞察。`
}

// GetEnhancedSystemPrompt 获取增强的系统提示词
func GetEnhancedSystemPrompt(ctx context.Context) string {
	config := NewSystemPromptConfig(ctx)
	return config.GeneratePrompt()
}

// LoadEnhancedPrompts 加载增强的提示词
func LoadEnhancedPrompts(ctx context.Context) ([]Message, error) {
	return []Message{{
		Role:    "system",
		Content: GetEnhancedSystemPrompt(ctx),
	}}, nil
}

// GetSystemPromptConfig 获取系统提示词配置
func GetSystemPromptConfig() *SystemPromptConfig {
	ctx := context.Background()
	return NewSystemPromptConfig(ctx)
}

// GetTemplateSystemPrompt 获取模板化的系统提示词（兼容旧代码）
func GetTemplateSystemPrompt(ctx context.Context) string {
	// 获取当前项目的领域ID
	domainID := GetCurrentDomainID()

	// 获取当前模型ID
	modelID := GetCurrentModelID()

	// 获取系统提示词配置
	config := NewSystemPromptConfig(ctx)

	// 使用段落管理器渲染系统提示词
	sm := &SegmentManager{}
	prompt, err := sm.RenderSystemPrompt(ctx, modelID, domainID, config)
	if err != nil {
		// 如果失败，使用基础提示词
		return config.GeneratePrompt()
	}

	return prompt
}
