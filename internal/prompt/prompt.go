package prompt

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"gitcode.com/dscli/dscli/internal/config"
	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/skills"
)

//go:embed chat.md
var chatTemplate string

//go:embed reasoner.md
var reasonerTemplate string

// promptTemplate 系统提示词模板
type promptTemplate struct {
	// 模板内容
	Template string
	// 模板名称（用于缓存）
	Name string
}

// promptConfig 模板数据
type promptConfig struct {
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

// GetPromptPath 获取提示词文件路径
// global: true表示全局配置，false表示项目配置
func GetPromptPath(model string, global bool) (string, error) {
	var promptDir string
	if global {
		promptDir = filepath.Join(config.ConfigDir, "prompt")
	} else {
		if context.ProjectRoot == "" {
			return "", fmt.Errorf("不在项目目录中")
		}
		promptDir = filepath.Join(context.ProjectRoot, ".dscli", "prompt")
	}

	err := os.MkdirAll(promptDir, 0o755)
	if err != nil {
		return "", fmt.Errorf("创建提示词目录失败 %s: %w", promptDir, err)
	}

	return filepath.Join(promptDir, fmt.Sprintf("%s.md", model)), nil
}

// readPromptFile 读取提示词文件
func readPromptFile(p string) string {
	if p == "" {
		return ""
	}

	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}

	content := strings.TrimSpace(string(b))
	if content == "" {
		return ""
	}

	return content
}

// GetPromptTemplate 获取当前生效的提示词模板
// 优先级：项目配置 > 全局配置 > 内嵌默认模板
func GetPromptTemplate(model string) string {
	// 先尝试项目配置
	p, err := GetPromptPath(model, false)
	if err == nil {
		prompt := readPromptFile(p)
		if prompt != "" {
			return prompt
		}
	}

	// 再尝试全局配置
	p, err = GetPromptPath(model, true)
	if err == nil {
		prompt := readPromptFile(p)
		if prompt != "" {
			return prompt
		}
	}

	// 最后返回内嵌默认模板
	return GetDefaultPromptTemplate(model)
}

// GetDefaultPromptTemplate 获取内嵌的默认提示词模板
func GetDefaultPromptTemplate(model string) string {
	if model == "chat" {
		return chatTemplate
	}
	return reasonerTemplate
}

// newPromptTemplate 获取指定模型的模板
// 对未知 modelID 默认返回 chat 模板，避免 nil pointer。
func newPromptTemplate(modelID int64) *promptTemplate {
	switch modelID {
	case context.DeepseekReasoner:
		return &promptTemplate{
			Name:     context.ModelDeepseekReasoner,
			Template: GetPromptTemplate("reasoner"),
		}
	default:
		// DeepseekChat (0) 或未知 modelID 统一使用 chat 模板
		return &promptTemplate{
			Name:     context.ModelDeepseekChat,
			Template: GetPromptTemplate("chat"),
		}
	}
}

// Render 渲染模板
func (t *promptTemplate) Render(data *promptConfig) (string, error) {
	tmpl, err := template.New(t.Name).Parse(t.Template)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return strings.TrimSpace(buf.String()), nil
}

// GeneratePromptWithTemplate 使用模板生成提示词
func (c *promptConfig) GeneratePromptWithTemplate() string {
	tmpl := newPromptTemplate(c.ModelID)
	if tmpl == nil {
		// 防御性编程：理论上 newPromptTemplate 不再返回 nil，
		// 但保留此检查以防未来重构引入 bug
		panic("prompt: newPromptTemplate returned nil — this is a bug")
	}

	prompt, err := tmpl.Render(c)
	if err != nil {
		panic(err)
	}

	return prompt
}

// GetSystemPrompt 获取增强的系统提示词
func GetSystemPrompt(ctx context.Context) string {
	config := newPromptConfig(ctx)
	return config.GeneratePromptWithTemplate()
}

// newPromptConfig 创建系统提示词配置
func newPromptConfig(ctx context.Context) *promptConfig {
	projectRoot := context.ProjectRoot
	modelID := context.ContextValue(ctx, context.CurrentModelIDKey, int64(0))
	config := &promptConfig{
		CurrentDate:      time.Now().Format("2006年01月02日"),
		ProjectRoot:      projectRoot,
		ConfigDir:        config.ConfigDir,
		WorkingDirectory: getWorkingDirectory(),
		Hostname:         getHostname(),
		Username:         getUsername(),
		ModelID:          modelID,
	}

	// 获取Git信息
	config.loadGitInfo()
	config.ProjectName = filepath.Base(config.ProjectRoot)
	config.ProjectType = config.detectProjectType()
	return config
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

// getWorkingDirectory 获取当前工作目录
func getWorkingDirectory() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "未知"
	}
	return cwd
}

// loadGitInfo 加载Git信息
func (c *promptConfig) loadGitInfo() {
	// 获取Git用户名
	if output, err := exec.Command("git", "config", "user.name").Output(); err == nil {
		c.GitUserName = strings.TrimSpace(string(output))
	} else {
		c.GitUserName = "未知"
	}

	// 获取Git邮箱
	if output, err := exec.Command("git", "config", "user.email").Output(); err == nil {
		c.GitUserEmail = strings.TrimSpace(string(output))
	} else {
		c.GitUserEmail = "未知"
	}

	// 获取当前分支
	if output, err := exec.Command("git", "branch", "--show-current").Output(); err == nil {
		c.GitBranch = strings.TrimSpace(string(output))
		if c.GitBranch == "" {
			c.GitBranch = "（无活动分支）"
		}
	} else {
		c.GitBranch = "非Git仓库"
	}

	// 获取Git状态（简化版）
	if output, err := exec.Command("git", "status", "--porcelain").Output(); err == nil {
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		if len(lines) > 0 && lines[0] != "" {
			c.GitStatus = fmt.Sprintf("有%d个文件变更", len(lines))
		} else {
			c.GitStatus = "工作区干净"
		}
	} else {
		c.GitStatus = "无法获取状态"
	}
}

// detectProjectType 检测项目类型
func (c *promptConfig) detectProjectType() string {
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

// LoadPrompts loads the system prompt combined with skill prompts.
func LoadPrompts(ctx context.Context) ([]Message, error) {
	systemPrompt := GetSystemPrompt(ctx)
	skillPrompt := skills.BuildSkillPrompt(ctx)

	content := systemPrompt
	if skillPrompt != "" {
		content += "\n\n"
		content += skillPrompt
	}

	notePrompt := BuildNotePrompt(ctx)
	if notePrompt != "" {
		content += "\n\n"
		content += notePrompt
	}

	return []Message{{
		Role:    "system",
		Content: content,
	}}, nil
}
