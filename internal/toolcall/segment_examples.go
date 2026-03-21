package toolcall

// SegmentExamples 段落示例
var SegmentExamples = map[string]string{
	// 编程领域通用段落
	"programming_general": `## 编程指导原则
1. 编写清晰、可维护的代码
2. 遵循{{if .IsGoProject}}Go语言{{else}}项目{{end}}的最佳实践
3. 添加必要的注释和文档
4. 考虑边界条件和错误处理`,

	// Go项目特定段落
	"go_specific": `## Go项目特定要求
{{if .IsGoProject}}
1. 遵循Go的代码规范（gofmt, go vet）
2. 使用适当的错误处理模式
3. 编写可测试的代码
4. 合理使用接口和组合
{{else}}
（此指导仅适用于Go项目）
{{end}}`,

	// Git相关段落
	"git_workflow": `## Git工作流程
{{if .GitUserName}}
当前用户：{{.GitUserName}}
{{end}}
{{if .IsGitClean}}
✅ 工作区干净，可以开始新任务
{{else}}
⚠️  工作区有未提交的更改，建议先提交或暂存
{{end}}

最佳实践：
1. 提交前运行测试
2. 编写有意义的提交信息
3. 保持提交小而专注`,

	// 项目特定段落
	"project_context": `## 项目上下文
项目：{{.ProjectName}} ({{.ProjectType}})
日期：{{.FormatDate}}
{{if .WorkingDirectory}}
工作目录：{{.WorkingDirectory}}
{{end}}

注意事项：
1. 了解项目结构和约定
2. 遵循团队编码规范
3. 考虑项目特定的约束条件`,

	// 代码审查段落
	"code_review": `## 代码审查指导
审查代码时关注：
1. 功能正确性
2. 代码可读性
3. 性能影响
4. 安全性考虑
5. 测试覆盖率

{{if .IsGoProject}}
Go特定审查点：
- 错误处理是否恰当
- 并发安全性
- 内存管理
- 接口设计
{{end}}`,

	// 调试帮助段落
	"debugging_help": `## 调试指导
调试步骤：
1. 重现问题
2. 定位问题范围
3. 分析根本原因
4. 验证修复方案

可用工具：
{{if .IsGoProject}}
- go test -v (测试)
- go vet (静态分析)
- delve (调试器)
- pprof (性能分析)
{{else}}
- 项目特定的测试工具
- 日志分析
- 调试器
{{end}}`,

	// 文档编写段落
	"documentation": `## 文档编写指导
编写高质量文档：
1. 明确目标读者
2. 结构清晰
3. 示例丰富
4. 及时更新

{{if .ProjectName}}
**{{.ProjectName}}** 项目文档应包含：
- README.md（项目概述）
- API文档
- 使用示例
- 贡献指南
{{end}}`,
}

// GetExampleSegment 获取示例段落
func GetExampleSegment(name string) string {
	if example, ok := SegmentExamples[name]; ok {
		return example
	}
	return ""
}

// AllExampleNames 获取所有示例名称
func AllExampleNames() []string {
	names := make([]string, 0, len(SegmentExamples))
	for name := range SegmentExamples {
		names = append(names, name)
	}
	return names
}

// ExampleUsage 示例使用方式
func ExampleUsage() string {
	return `## 段落模板使用示例

### 基本变量替换
{{.CurrentDate}} - 当前日期
{{.ProjectName}} - 项目名称
{{.ProjectType}} - 项目类型
{{.GitUserName}} - Git用户名

### 条件判断
{{if .IsGoProject}}
这是Go项目特定的指导
{{else}}
通用项目指导
{{end}}

{{if .GitUserName}}
当前用户：{{.GitUserName}}
{{end}}

### 函数调用
{{.FormatDate}} - 格式化日期
{{.IsGoProject}} - 是否是Go项目
{{.IsGitClean}} - Git是否干净

### 组合使用
项目：{{.ProjectName}} ({{.ProjectType}})
{{if .IsGoProject}}
遵循Go最佳实践
{{end}}
日期：{{.FormatDate}}`
}
