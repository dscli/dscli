package toolcall

import (
	"bytes"
	"context"
	"log"
	"strings"
	"text/template"
)

// SystemPromptTemplate 系统提示词模板
type SystemPromptTemplate struct {
	// 模板内容
	Template string
	// 模板名称（用于缓存）
	Name string
}

// TemplateData 模板数据
type TemplateData struct {
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

// NewTemplateData 从SystemPromptConfig创建模板数据
func NewTemplateData(config *SystemPromptConfig) *TemplateData {
	return &TemplateData{
		CurrentDate:      config.CurrentDate,
		ProjectRoot:      config.ProjectRoot,
		ConfigDir:        config.ConfigDir,
		GitUserName:      config.GitUserName,
		GitUserEmail:     config.GitUserEmail,
		GitBranch:        config.GitBranch,
		GitStatus:        config.GitStatus,
		ProjectName:      config.ProjectName,
		ProjectType:      config.ProjectType,
		WorkingDirectory: config.WorkingDirectory,
		Hostname:         config.Hostname,
		Username:         config.Username,
		ModelID:          config.ModelID,
	}
}

// GetTemplateForModel 获取指定模型的模板
func GetTemplateForModel(modelID int64) *SystemPromptTemplate {
	switch modelID {
	case DeepseekChat:
		return &SystemPromptTemplate{
			Name:     "deepseek-chat",
			Template: deepseekChatTemplate,
		}
	case DeepseekReasoner:
		return &SystemPromptTemplate{
			Name:     "deepseek-reasoner",
			Template: deepseekReasonerTemplate,
		}
	default:
		log.Fatalf("不支持模型ID: %d", modelID)
		return nil
	}
}

// Render 渲染模板
func (t *SystemPromptTemplate) Render(data *TemplateData) (string, error) {
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

// deepseekChatTemplate Deepseek Chat模板
const deepseekChatTemplate = `# 🎯 专业编程助手

## 核心身份
你是dscli项目的专业编程助手，提供深入的技术分析和解决方案。

## 🔄 工作流程
1. **全面理解问题**：分析背景、约束条件和目标
2. **深入分析思考**：从多个角度考虑可能性、边界条件和潜在影响
3. **提供深刻洞察**：给出有价值的见解和解决方案，不仅仅是表面答案

## 🧠 思考原则
- **逻辑严谨**：推理过程无漏洞，结论有充分依据
- **系统思维**：从整体和系统的角度分析问题
- **深度优先**：追求深刻理解，而不是快速回答

## 📅 当前环境
- 日期：{{.CurrentDate}}
- 项目：{{.ProjectName}}（{{.ProjectType}}）
- 用户：{{.GitUserName}} <{{.GitUserEmail}}>
- 分支：{{.GitBranch}}（{{.GitStatus}}）

## 🛠️ 可用能力
- **文件/代码操作**：读写、搜索、结构分析
- **Git管理**：提交、推送、patch生成/应用
- **交互咨询**：向用户/专家提问
- **代码审查**：专家审阅代码质量
- **系统工具**：Shell、Python、SQLite、Web
- **Issue管理**：创建、更新、跟踪任务

## 📋 质量要求
- **代码简洁**：避免不必要的复杂性
- **最佳实践**：遵循Go语言和项目规范
- **充分注释**：解释复杂逻辑和设计决策
- **错误处理**：防御性编程，有意义的错误信息

## 🚀 执行指导
1. **智能选择工具**：根据任务需求选择最合适的工具
2. **参数优化**：提供准确完整的参数
3. **逐步推进**：保持逻辑严谨，逐步解决问题
4. **及时保存**：重要更改及时提交到Git

## ⚠️ 注意事项
- **权限边界**：可以操作项目文件，但不能删除sqlite.db和dscli.env
- **版权尊重**：版权归人类所有，所有者：{{.GitUserName}} <{{.GitUserEmail}}>
- **工具优先**：优先使用现有工具，避免重复造轮子

---

**提示词版本**：v3.0.0（分层模块化架构）
**使用提示**：如需查看完整工具文档，请使用 system_prompt 工具

请基于以上信息，为用户提供专业的编程帮助。`

const deepseekReasonerTemplate = `你是编程领域一个深入思考者。
## 思考环境
- 当前日期：{{.CurrentDate}}
- 项目：{{.ProjectName}}（{{.ProjectType}}）
- 版权所有者：{{.GitUserName}} <{{.GitUserEmail}}>

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

// GeneratePromptWithTemplate 使用模板生成提示词
func (c *SystemPromptConfig) GeneratePromptWithTemplate() string {
	template := GetTemplateForModel(c.ModelID)
	data := NewTemplateData(c)

	prompt, err := template.Render(data)
	if err != nil {
		log.Printf("渲染模板失败: %v", err)
		// 返回一个基本的提示词，而不是回退到字符串拼接
		return "你是一个专业的编程助手。请基于当前环境提供帮助。"
	}

	return prompt
}

// GetEnhancedSystemPromptWithTemplate 获取使用模板的增强系统提示词
func GetEnhancedSystemPromptWithTemplate(ctx context.Context) string {
	config := NewSystemPromptConfig(ctx)
	return config.GeneratePromptWithTemplate()
}

// LoadEnhancedPromptsWithTemplate 加载使用模板的增强提示词
func LoadEnhancedPromptsWithTemplate(ctx context.Context) ([]Message, error) {
	return []Message{{
		Role:    "system",
		Content: GetEnhancedSystemPromptWithTemplate(ctx),
	}}, nil
}
