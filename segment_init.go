package main

import (
	"database/sql"
	"fmt"
	"log"
)

func init() {
	// 注册段落初始化钩子
	RegisterPostInitHook(initSegmentsHook)
}

// initSegmentsHook 段落初始化钩子
func initSegmentsHook(db *sql.DB) error {
	// 检查是否已有系统级段落
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM prompt_segments WHERE domain_id = 0 AND model_id IN (0, 1)").Scan(&count)
	if err != nil {
		return fmt.Errorf("检查段落失败: %w", err)
	}

	// 如果已有系统级段落，跳过初始化
	if count > 0 {
		log.Println("系统级段落已存在，跳过初始化")
		return nil
	}

	// 插入默认系统级段落
	segments := []struct {
		name      string
		content   string
		modelID   int64
		sortOrder int
	}{
		{
			name: "Deepseek Chat 系统提示词",
			content: `你是一个专业的编程助手。

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

请基于以上信息，为用户提供专业的编程帮助。`,
			modelID:   DeepseekChat,
			sortOrder: 10,
		},
		{
			name: "Deepseek Reasoner 系统提示词",
			content: `你是编程领域一个深入思考者。

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

请基于以上原则，为用户提供深入的编程思考和洞察。`,
			modelID:   DeepseekReasoner,
			sortOrder: 20,
		},
	}

	// 开始事务
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer tx.Rollback()

	// 插入系统级段落
	for _, segment := range segments {
		_, err := tx.Exec(`
			INSERT INTO prompt_segments (domain_id, model_id, name, content, sort_order, enabled)
			VALUES (0, ?, ?, ?, ?, 1)
		`, segment.modelID, segment.name, segment.content, segment.sortOrder)
		if err != nil {
			return fmt.Errorf("插入系统级段落失败: %w", err)
		}
		log.Printf("已插入系统级段落: %s (模型: %d)", segment.name, segment.modelID)
	}

	// 提交事务
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}

	log.Println("✅ 段落初始化完成")
	return nil
}

// GetTemplateVariables 获取可用的模板变量
func GetTemplateVariables() []TemplateVariable {
	return []TemplateVariable{
		{
			Name:        "{{.CurrentDate}}",
			Description: "当前日期，格式：2006年01月02日",
			Example:     "2026年03月06日",
		},
		{
			Name:        "{{.Hostname}}",
			Description: "主机名",
			Example:     "dev01",
		},
		{
			Name:        "{{.Username}}",
			Description: "当前用户名",
			Example:     "nanjj",
		},
		{
			Name:        "{{.WorkingDirectory}}",
			Description: "当前工作目录",
			Example:     "/home/nanjj/src/gitcode.com/dscli/dscli",
		},
		{
			Name:        "{{.ProjectRoot}}",
			Description: "项目根目录",
			Example:     "/home/nanjj/src/gitcode.com/dscli/dscli",
		},
		{
			Name:        "{{.ConfigDir}}",
			Description: "配置目录",
			Example:     "/home/nanjj/.dscli",
		},
		{
			Name:        "{{.ProjectName}}",
			Description: "项目名称",
			Example:     "dscli",
		},
		{
			Name:        "{{.ProjectType}}",
			Description: "项目类型",
			Example:     "Go项目",
		},
		{
			Name:        "{{.GitUserName}}",
			Description: "Git用户名（可能为空）",
			Example:     "Nan Jun Jie",
		},
		{
			Name:        "{{.GitUserEmail}}",
			Description: "Git邮箱（可能为空）",
			Example:     "nanjunjie@139.com",
		},
		{
			Name:        "{{.GitBranch}}",
			Description: "当前Git分支（可能为空）",
			Example:     "main",
		},
		{
			Name:        "{{.GitStatus}}",
			Description: "Git状态（可能为空）",
			Example:     "工作区干净",
		},
		{
			Name:        "{{.ModelID}}",
			Description: "模型ID（-1: chat, -2: reasoner）",
			Example:     "-1",
		},
	}
}

// TemplateVariable 模板变量信息
type TemplateVariable struct {
	Name        string
	Description string
	Example     string
}
