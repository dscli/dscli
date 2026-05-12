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
	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/roles"
	"gitcode.com/dscli/dscli/internal/session"
	"gitcode.com/dscli/dscli/internal/skills"
)

//go:embed dev.md
var devTemplate string

//go:embed expert.md
var expertTemplate string

//go:embed review.md
var reviewTemplate string

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

	// 角色（dev/expert/review）
	Role string

	// 模型特定配置
	ModelID int64

	// context（用于数据库查询等）
	ctx context.Context
}

// GetPromptPath 获取提示词文件路径
// global: true表示全局配置，false表示项目配置
func GetPromptPath(role string, global bool) (string, error) {
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

	return filepath.Join(promptDir, fmt.Sprintf("%s.md", role)), nil
}

// PromptInfo 提示词基本信息
type PromptInfo struct {
	Name        string
	Description string
	Source      string // "built-in", "project", "global"
}

// extractDescription 提取 md 第一行作为描述（去掉 # 前缀）
func extractDescription(content string) string {
	lines := strings.SplitN(content, "\n", 2)
	if len(lines) == 0 {
		return ""
	}
	line := strings.TrimSpace(lines[0])
	// 去掉开头的 # 符号
	line = strings.TrimLeft(line, "# ")
	line = strings.TrimSpace(line)
	return line
}

// ListPrompts 列出所有可用提示词（去重，项目优先 > 全局 > 内嵌）
func ListPrompts() []PromptInfo {
	seen := map[string]bool{}
	var result []PromptInfo

	// 1. 内嵌模板（兜底）
	for name, tmpl := range roleTemplateMap {
		if seen[name] {
			continue
		}
		seen[name] = true
		result = append(result, PromptInfo{
			Name:        name,
			Description: extractDescription(tmpl),
			Source:      "built-in",
		})
	}

	// 2. 全局自定义 (~/.dscli/prompt/*.md)
	globalDir := filepath.Join(config.ConfigDir, "prompt")
	if entries, err := os.ReadDir(globalDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".md" {
				continue
			}
			name := strings.TrimSuffix(e.Name(), ".md")
			p := filepath.Join(globalDir, e.Name())
			content := readPromptFile(p)
			desc := extractDescription(content)
			if desc == "" {
				desc = content // fallback
			}
			if seen[name] {
				// 更新描述（全局覆盖内嵌）
				for i := range result {
					if result[i].Name == name {
						result[i].Description = desc
						result[i].Source = "global"
						break
					}
				}
			} else {
				seen[name] = true
				result = append(result, PromptInfo{
					Name:        name,
					Description: desc,
					Source:      "global",
				})
			}
		}
	}

	// 3. 项目自定义 (${PROJECT_ROOT}/.dscli/prompt/*.md) — 最高优先级
	if context.ProjectRoot != "" {
		projectDir := filepath.Join(context.ProjectRoot, ".dscli", "prompt")
		if entries, err := os.ReadDir(projectDir); err == nil {
			for _, e := range entries {
				if e.IsDir() || filepath.Ext(e.Name()) != ".md" {
					continue
				}
				name := strings.TrimSuffix(e.Name(), ".md")
				p := filepath.Join(projectDir, e.Name())
				content := readPromptFile(p)
				desc := extractDescription(content)
				if desc == "" {
					desc = content
				}
				if seen[name] {
					for i := range result {
						if result[i].Name == name {
							result[i].Description = desc
							result[i].Source = "project"
							break
						}
					}
				} else {
					seen[name] = true
					result = append(result, PromptInfo{
						Name:        name,
						Description: desc,
						Source:      "project",
					})
				}
			}
		}
	}

	return result
}

// ResolvePromptEditPath 确定编辑提示词的目标路径
// 优先编辑项目级别（若已存在）；若不在项目中或文件不存在则使用全局
// ResolvePromptEditPath 确定编辑提示词的目标路径
// 优先编辑项目级别（若已存在）；若不在项目中或文件不存在则使用全局
func ResolvePromptEditPath(name string) (string, error) {
	// 先尝试项目级别
	if context.ProjectRoot != "" {
		p, err := GetPromptPath(name, false)
		if err == nil {
			if _, statErr := os.Stat(p); statErr == nil {
				return p, nil // 项目级别已存在
			}
		}
	}

	// 尝试全局级别
	p, err := GetPromptPath(name, true)
	if err == nil {
		if _, statErr := os.Stat(p); statErr == nil {
			return p, nil // 全局级别已存在
		}
	}

	// 都不存在：优先项目级别创建
	if context.ProjectRoot != "" {
		return GetPromptPath(name, false)
	}
	return GetPromptPath(name, true)
}

// ResolvePromptRemovePath 确定删除提示词的目标路径
// 优先项目级别（若存在），否则全局
func ResolvePromptRemovePath(name string) (string, error) {
	if context.ProjectRoot != "" {
		p, err := GetPromptPath(name, false)
		if err == nil {
			if _, statErr := os.Stat(p); statErr == nil {
				return p, nil
			}
		}
	}
	return GetPromptPath(name, true)
}

// GetPromptSourceContent 获取用于初始化新提示词文件的种子内容。
// 非 global 作用域（项目）：先尝试全局文件，再尝试内建模板。
// global 作用域：只尝试内建模板（已知内建名才返回内容）。
// 无可用来源时返回空字符串，调用方创建空文件。
func GetPromptSourceContent(name string, global bool) string {
	if !global {
		// 先尝试全局文件（比内建更近的作用域）
		p, err := GetPromptPath(name, true)
		if err == nil {
			if content := readPromptFile(p); content != "" {
				return content
			}
		}
	}
	// 再尝试内建模板（仅已知内建名）
	if tmpl, ok := roleTemplateMap[name]; ok {
		return tmpl
	}
	return ""
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
// 优先级：项目配置(.dscli/prompt/) > 全局配置 > 内嵌默认模板
// GetPromptTemplate 获取当前生效的提示词模板。
// 优先级：项目配置(.dscli/prompt/) > 全局配置 > 内嵌默认模板。
// 当数据库中存在 role→prompt 映射时，使用映射后的 prompt 名称加载模板；
func GetPromptTemplate(ctx context.Context, role string) string {
	// 检查数据库中是否有 role→prompt 映射
	promptName := role
	sessionID := session.GetCurrentSessionID(ctx)
	if cfg, err := roles.GetRoleConfig(role, sessionID); err == nil && cfg != nil {
		if cfg.Prompt != "" {
			promptName = cfg.Prompt
		}
	}

	// 按文档约定优先级: .dscli/prompt/ → ~/.dscli/prompt/ → 内嵌
	// 先尝试项目级配置 (${PROJECT_ROOT}/.dscli/prompt/)
	p, err := GetPromptPath(promptName, false)
	if err == nil {
		prompt := readPromptFile(p)
		if prompt != "" {
			return prompt
		}
	}

	// 再尝试系统级配置 (~/.dscli/prompt/)
	p, err = GetPromptPath(promptName, true)
	if err == nil {
		prompt := readPromptFile(p)
		if prompt != "" {
			return prompt
		}
	}

	// 最后返回内嵌默认模板（只读兜底）
	return GetDefaultPromptTemplate(promptName)
}

// roleTemplateMap 角色到内嵌模板的映射
var roleTemplateMap = map[string]string{
	"dev":    devTemplate,
	"expert": expertTemplate,
	"review": reviewTemplate,
}

// GetDefaultPromptTemplate 获取内嵌的默认提示词模板
// role: dev, expert, review
func GetDefaultPromptTemplate(role string) string {
	if tmpl, ok := roleTemplateMap[role]; ok {
		return tmpl
	}
	return devTemplate // 未知角色默认使用 dev 模板
}

// newPromptTemplate 根据角色获取模板
// role: dev/expert/review
// newPromptTemplate 根据角色获取模板
// role: dev/expert/review
func newPromptTemplate(ctx context.Context, role string) *promptTemplate {
	tmpl := GetPromptTemplate(ctx, role)
	return &promptTemplate{
		Name:     role,
		Template: tmpl,
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
	tmpl := newPromptTemplate(c.ctx, c.Role)
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
	role := context.ContextValue(ctx, context.CurrentRoleKey, "dev")
	config := &promptConfig{
		CurrentDate:      time.Now().Format("2006年01月02日"),
		ProjectRoot:      projectRoot,
		ConfigDir:        config.ConfigDir,
		WorkingDirectory: getWorkingDirectory(),
		Hostname:         getHostname(),
		Username:         getUsername(),
		Role:             role,
		ModelID:          modelID,
		ctx:              ctx,
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

// LoadPrompts loads the system prompt combined with skill and note prompts.
// Only dev role gets skills and notes; expert/review roles skip them.
// LoadPrompts loads the system prompt combined with skill and note prompts.
// Skills are injected according to role config; when no config exists,
// only dev role gets skills (hardcoded fallback).
func LoadPrompts(ctx context.Context) ([]Message, error) {
	systemPrompt := GetSystemPrompt(ctx)

	content := systemPrompt

	// 根据角色配置决定是否注入 skill prompt
	role := context.ContextValue(ctx, context.CurrentRoleKey, "dev")
	// 查找数据库中的角色配置
	sessionID := session.GetCurrentSessionID(ctx)
	cfg, cfgErr := roles.GetRoleConfig(role, sessionID)
	if cfgErr != nil {
		outfmt.Debug("获取角色配置失败: %v，回退到默认行为\n", cfgErr)
	}

	var skillPrompt string
	if cfg == nil {
		// Fallback: 只有 dev 角色获得 skill prompt
		if role == "dev" || role == "" {
			skillPrompt = skills.BuildSkillPrompt(ctx)
		}
	} else {
		allowedSkills := roles.ParseSkillsList(cfg.Skills)
		if allowedSkills == nil {
			// "all" — 加载所有技能
			skillPrompt = skills.BuildSkillPrompt(ctx)
		} else if len(allowedSkills) > 0 {
			// 指定技能列表 — 按名称过滤
			skillPrompt = skills.BuildSkillPrompt(ctx, allowedSkills...)
		}
		// else: 空列表 → 不注入 skill prompt
	}

	if skillPrompt != "" {
		content += "\n\n"
		content += skillPrompt
	}

	// 笔记提示词所有角色都加载（帮助回忆上下文）
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
