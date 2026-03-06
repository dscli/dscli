package main

import (
	"bytes"
	"context"
	"html/template"
	"log"
	"strings"
)

// PromptTemplate 提示词模板管理器
type PromptTemplate struct {
	config *SystemPromptConfig
}

// NewPromptTemplate 创建提示词模板管理器
func NewPromptTemplate(ctx context.Context) *PromptTemplate {
	return &PromptTemplate{
		config: NewSystemPromptConfig(ctx),
	}
}

// GeneratePrompt 生成系统提示词
func (pt *PromptTemplate) GeneratePrompt() string {
	switch pt.config.ModelID {
	case DeepseekChat:
		return pt.generateWithTemplate(deepseekChatTemplate)
	case DeepseekReasoner:
		return pt.generateWithTemplate(deepseekReasonerTemplate)
	default:
		log.Fatalf("不支持模型ID: %d", pt.config.ModelID)
		return ""
	}
}

// generateWithTemplate 使用模板生成提示词
func (pt *PromptTemplate) generateWithTemplate(tmplStr string) string {
	tmpl, err := template.New("system_prompt").Parse(tmplStr)
	if err != nil {
		log.Printf("解析模板失败: %v", err)
		// 回退到原始方法
		return pt.config.GeneratePrompt()
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, pt.config); err != nil {
		log.Printf("执行模板失败: %v", err)
		// 回退到原始方法
		return pt.config.GeneratePrompt()
	}

	return strings.TrimSpace(buf.String())
}

// 辅助函数，用于模板中
func (c *SystemPromptConfig) HasGitChanges() bool {
	return c.GitStatus != "工作区干净" && c.GitStatus != ""
}

// Deepseek Chat 模板
const deepseekChatTemplate = `你是一个专业的编程助手。

当前日期：{{.CurrentDate}}，请基于当前日期处理与日期相关的需求。

## 环境信息
- 主机：{{.Hostname}}（用户：{{.Username}}）
- 工作目录：{{.WorkingDirectory}}
- 项目根目录：{{.ProjectRoot}}
- 配置目录：{{.ConfigDir}}

## 项目信息
- 项目名称：{{.ProjectName}}
- 项目类型：{{.ProjectType}}

## Git状态
{{- if .GitUserName}}
- 用户：{{.GitUserName}} <{{.GitUserEmail}}>
{{- end}}
{{- if .GitBranch}}
- 分支：{{.GitBranch}}
{{- end}}
{{- if .GitStatus}}
- 状态：{{.GitStatus}}
{{- end}}

## 文件操作权限
1. 你可以增删改查当前工作目录下的任何文件
2. 你可以操作配置目录下的任何文件，但不能删除以下文件：
   - sqlite.db（技能数据库）
   - dscli.env（环境配置文件）

## 版权信息
1. 版权归人类所有
{{- if .GitUserName}}
2. 版权所有者：{{.GitUserName}} <{{.GitUserEmail}}>
{{- end}}

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

// Deepseek Reasoner 模板
const deepseekReasonerTemplate = `你是编程领域一个深入思考者。

## 思考环境
- 当前日期：{{.CurrentDate}}
- 项目：{{.ProjectName}}（{{.ProjectType}}）
{{- if .GitUserName}}
- 版权所有者：{{.GitUserName}} <{{.GitUserEmail}}>
{{- end}}

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

// GetTemplateSystemPrompt 获取模板化的系统提示词
func GetTemplateSystemPrompt(ctx context.Context) string {
	template := NewPromptTemplate(ctx)
	return template.GeneratePrompt()
}

// LoadTemplatePrompts 加载模板化的提示词
func LoadTemplatePrompts(ctx context.Context) ([]Message, error) {
	return []Message{{
		Role:    "system",
		Content: GetTemplateSystemPrompt(ctx),
	}}, nil
}
