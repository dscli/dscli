# 系统提示词示例

## 新的系统提示词特性

### 1. 动态信息收集
新的系统提示词会自动收集以下信息：
- 当前日期和时间
- 项目根目录和配置目录
- Git信息（用户名、邮箱、分支、状态）
- 项目类型检测（Go、Node.js、Python等）
- 环境信息（主机名、用户名）

### 2. 模型特定提示词
- **Deepseek Chat**: 专注于编程助手功能，包含详细的操作指南
- **Deepseek Reasoner**: 专注于深度思考和分析，提供思考框架

### 3. 结构化输出
提示词按照以下结构组织：
```
## 环境信息
## 项目信息  
## Git状态
## 文件操作权限
## 版权信息
## 工作流程
## 重要原则
```

## 示例输出

### Deepseek Chat 示例
```markdown
你是一个专业的编程助手。

当前日期：2026年03月05日，请基于当前日期处理与日期相关的需求。

## 环境信息
- 主机：localhost（用户：nanjj）
- 工作目录：/home/nanjj/src/gitcode.com/dscli/dscli
- 项目根目录：/home/nanjj/src/gitcode.com/dscli/dscli
- 配置目录：/home/nanjj/.dscli

## 项目信息
- 项目名称：dscli
- 项目类型：Go项目

## Git状态
- 用户：nanjj <nanjj@example.com>
- 分支：main
- 状态：工作区干净

## 文件操作权限
1. 你可以增删改查当前工作目录下的任何文件
2. 你可以操作配置目录下的任何文件，但不能删除以下文件：
   - sqlite.db（技能数据库）
   - dscli.env（环境配置文件）

## 版权信息
1. 版权归人类所有
2. 版权所有者：nanjj <nanjj@example.com>

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

请基于以上信息，为用户提供专业的编程帮助。
```

### Deepseek Reasoner 示例
```markdown
你是编程领域一个深入思考者。

## 思考环境
- 当前日期：2026年03月05日
- 项目：dscli（Go项目）
- 版权所有者：nanjj <nanjj@example.com>

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

请基于以上原则，为用户提供深入的编程思考和洞察。
```

## 代码结构

### 主要文件
1. `system_prompt.go` - 新的系统提示词实现
2. `prompt.go` - 向后兼容的接口
3. `prompt_test.go` - 测试文件

### 核心类型
```go
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
```

### 主要函数
```go
// 创建配置并生成提示词
func NewSystemPromptConfig(ctx context.Context) *SystemPromptConfig
func (c *SystemPromptConfig) GeneratePrompt() string

// 向后兼容的接口
func GetSystemPrompt(ctx context.Context) string
func LoadPrompts(ctx context.Context) ([]Message, error)

// 增强接口
func GetEnhancedSystemPrompt(ctx context.Context) string
func LoadEnhancedPrompts(ctx context.Context) ([]Message, error)
```

## 优势

1. **信息丰富**: 提供全面的上下文信息
2. **动态更新**: 实时获取Git状态、项目信息等
3. **模型优化**: 为不同模型提供定制化提示词
4. **易于测试**: 结构化的配置对象便于测试
5. **可扩展**: 易于添加新的信息源或模型类型
6. **向后兼容**: 保持现有API不变

## 使用示例

```go
// 基本用法（向后兼容）
prompt := GetSystemPrompt(ctx)
messages, err := LoadPrompts(ctx)

// 高级用法
config := NewSystemPromptConfig(ctx)
customPrompt := config.GeneratePrompt()

// 获取特定模型的提示词
config.ModelID = DeepseekReasoner
reasonerPrompt := config.GeneratePrompt()
```

这个新的系统提示词系统为AI助手提供了更丰富、更准确的上下文信息，有助于提高回答的质量和相关性。