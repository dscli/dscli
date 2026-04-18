package prompt

import (
	"bytes"
	_ "embed"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"gitcode.com/dscli/dscli/internal/config"
	"gitcode.com/dscli/dscli/internal/context"
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

func GetPromptPath(model string, global bool) string {
	promptDir := filepath.Join(context.ProjectRoot, ".dscli", "prompt")
	if global {
		promptDir = filepath.Join(config.ConfigDir, "prompt")
	}
	err := os.MkdirAll(promptDir, 0o755)
	if err != nil {
		return ""
	}
	return filepath.Join(promptDir, fmt.Sprintf("%s.md", model))
}

// readPromptFile 读取提示词文件
func readPromptFile(p string) string {
	if p == "" {
		return ""
	}

	b, err := os.ReadFile(p)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("读取提示词文件失败 %s: %v", p, err)
		}
		return ""
	}

	content := strings.TrimSpace(string(b))
	if content == "" {
		log.Printf("提示词文件为空: %s", p)
		return ""
	}

	return content
}

func GetPromptTemplate(model string) string {
	p := GetPromptPath(model, false)
	prompt := readPromptFile(p)
	if prompt != "" {
		return prompt
	}
	p = GetPromptPath(model, true)
	prompt = readPromptFile(p)
	if prompt != "" {
		return prompt
	}

	if model == "chat" {
		return chatTemplate
	}
	return reasonerTemplate
}

// newPromptTemplate 获取指定模型的模板
func newPromptTemplate(modelID int64) *promptTemplate {
	switch modelID {
	case context.DeepseekChat:
		return &promptTemplate{
			Name:     context.ModelDeepseekChat,
			Template: GetPromptTemplate("chat"),
		}
	case context.DeepseekReasoner:
		return &promptTemplate{
			Name:     context.ModelDeepseekReasoner,
			Template: GetPromptTemplate("reasoner"),
		}
	default:
		log.Fatalf("不支持模型ID: %d", modelID)
		return nil
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
	template := newPromptTemplate(c.ModelID)
	data := c

	prompt, err := template.Render(data)
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
// getUsername 获取用户名
func getUsername() string {
	return os.Getenv("USER")
}

// getWorkingDirectory 获取当前工作目录
func getWorkingDirectory() string {
	cwd, err := os.Getwd()
	if err != nil {
		log.Printf("无法获取工作目录: %v", err)
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
		log.Printf("警告: 获取Git用户名失败: %v", err)
		c.GitUserName = "未知"
	}

	// 获取Git邮箱
	if output, err := exec.Command("git", "config", "user.email").Output(); err == nil {
		c.GitUserEmail = strings.TrimSpace(string(output))
	} else {
		log.Printf("警告: 获取Git邮箱失败: %v", err)
		c.GitUserEmail = "未知"
	}

	// 获取当前分支
	if output, err := exec.Command("git", "branch", "--show-current").Output(); err == nil {
		c.GitBranch = strings.TrimSpace(string(output))
		if c.GitBranch == "" {
			c.GitBranch = "（无活动分支）"
		}
	} else {
		log.Printf("警告: 获取Git分支失败: %v", err)
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
		log.Printf("警告: 获取Git状态失败: %v", err)
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
