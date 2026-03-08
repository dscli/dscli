# dscli 项目架构指南

## 🏗️ 架构原则

### 1. 目录结构约束
**核心原则**：保持Go代码结构简单扁平
- ✅ **所有Go代码文件都在项目根目录下**
- ✅ **不创建Go代码子目录**（如 `cmd/`, `pkg/`, `internal/` 等）
- ✅ **非代码目录允许**：`docs/`, `build/` 等非Go代码目录可以存在
- ✅ **配置文件目录**：`~/.dscli/` 用于用户配置和数据库

### 2. 文件组织模式
**一对一对应原则**：
```
[功能].go          ← 功能实现文件
[功能]_test.go     ← 对应的测试文件
```

**当前文件组织示例**：
```
chat.go           ← 聊天功能实现
chat_test.go      ← 聊天功能测试

tools.go          ← 工具系统实现  
tools_test.go     ← 工具系统测试

db.go             ← 数据库操作
db_test.go        ← 数据库测试
```

### 3. 包管理
- **单一包名**：所有Go文件使用 `package main`
- **无内部包**：不创建 `internal/` 子包
- **导入简洁**：通过文件命名区分功能模块，而不是目录

## 📁 当前文件结构

### 核心功能文件
```
main.go           ← 程序入口点
main_test.go      ← 入口点测试

client.go         ← AI客户端接口
chat.go           ← 聊天处理逻辑
tools.go          ← 工具调用系统
db.go             ← 数据库操作
output.go         ← 格式化输出
formatter.go      ← 格式转换
types.go          ← 类型定义
```

### 工具相关文件
```
web.go            ← 网页读取工具
netrc.go          ← 网络认证工具
fim.go            ← 代码补全功能
issue.go          ← Issue处理
balance.go        ← 余额查询
models.go         ← 模型管理
prompt.go         ← 提示词管理
version.go        ← 版本信息
markdown2org.go   ← 格式转换工具
```

### 测试文件
```
*_test.go         ← 每个功能对应的测试文件
```

## 🚫 禁止的模式

### 1. 禁止创建Go代码子目录
```bash
# ❌ 不允许
mkdir cmd
mkdir pkg
mkdir internal
mkdir plugin
mkdir workflow

# ✅ 允许（非Go代码）
mkdir docs
mkdir build
mkdir examples
```

### 2. 禁止复杂的包结构
```go
// ❌ 不允许
package cmd
package pkg
package internal

// ✅ 允许
package main
```

### 3. 禁止分散的功能文件
```bash
# ❌ 不允许
chat/
  handler.go
  processor.go
  utils.go

# ✅ 允许
chat.go          ← 包含所有聊天相关功能
chat_utils.go    ← 如果需要分离，使用后缀区分
```

## ✅ 推荐的模式

### 1. 功能聚合
如果功能相关，可以放在同一个文件中：
```go
// skill.go
package main

// 技能定义
type Skill struct {
    Name string
    Description string
}

// 技能管理函数
func CreateSkill() {}
func EnableSkill() {}
func DisableSkill() {}
```

### 2. 文件大小控制
如果文件过大（>1000行），考虑按功能拆分：
```bash
# 原始大文件
tools.go          ← 2000行

# 拆分后
tools.go          ← 核心工具定义（800行）
tools_exec.go     ← 工具执行逻辑（700行）
tools_stats.go    ← 工具统计功能（500行）
```

### 3. 测试文件组织
保持测试文件与源文件一一对应：
```bash
# 源文件
batch.go
workflow.go
config.go

# 测试文件  
batch_test.go
workflow_test.go
config_test.go
```

## 🔄 新功能开发流程

### 步骤1：确定文件位置
```bash
# 新功能：批处理
# 文件：batch.go（根目录下）
# 测试：batch_test.go（根目录下）
```

### 步骤2：实现功能
```go
// batch.go
package main

func ProcessBatch(tasks []string) error {
    // 实现批处理逻辑
}
```

### 步骤3：添加测试
```go
// batch_test.go
package main

func TestProcessBatch(t *testing.T) {
    // 测试批处理功能
}
```

### 步骤4：更新文档
- 更新 `TODO.md` 任务状态
- 更新 `ROADMAP.md` 进度
- 如有必要，更新 `ARCHITECTURE.md`

## 📊 文件统计和监控

### 当前状态
```bash
# 查看文件统计
$ ls *.go | wc -l          # 总Go文件数
$ ls *_test.go | wc -l     # 测试文件数
$ ls *.go | grep -v _test.go | wc -l  # 源文件数
```

### 质量指标
1. **测试覆盖率**：`go test -cover`
2. **文件大小**：单个文件不超过1500行
3. **函数复杂度**：单个函数不超过50行
4. **依赖关系**：避免循环依赖

## 🎯 架构优势

### 1. 简单性
- 无需复杂的导入路径
- 文件查找直观
- 构建过程简单

### 2. 可维护性
- 所有代码一目了然
- 修改影响范围明确
- 新人上手快速

### 3. 一致性
- 统一的代码组织方式
- 标准的测试模式
- 清晰的职责划分

## 🚨 例外情况

### 允许的例外
1. **第三方库**：`vendor/` 目录（如果使用vendor）
2. **生成代码**：`generated/` 目录（如果使用代码生成）
3. **资源文件**：`assets/`, `templates/` 等非Go文件

### 不允许的例外
1. **业务逻辑**：不能在子目录中
2. **工具实现**：不能在子目录中
3. **接口定义**：不能在子目录中

## 🤝 贡献者指南

### 开发前检查
1. 确认新功能是否需要新文件
2. 确认文件放在根目录下
3. 确认有对应的测试文件
4. 确认不违反架构约束

### 提交前检查
1. 运行 `go test ./...` 确保测试通过
2. 运行 `go build` 确保编译通过
3. 检查文件大小和复杂度
4. 更新相关文档

---


## 📋 核心类型定义

### 1. ToolDef 工具定义结构
`ToolDef` 是 dscli 工具系统的核心类型，用于定义和注册所有可调用的工具：

```go
// ToolDef 工具定义
type ToolDef struct {
    Name        string  // 工具名称（必须与工具注册名称一致）
    DisplayName string  // 显示名称（自动从Name生成，如"read_file" -> "ReadFile"）
    Description string  // 工具描述（用于AI理解工具功能）
    Parameters  map[string]any  // 参数定义（JSON Schema格式）
    Category    string  // 工具分类（如"file_ops"、"git"、"system"等）
    Timeout     time.Duration  // 工具执行超时时间（必须设置）
    Handler     func(ctx context.Context, args map[string]string) (string, error)  // 工具处理器函数
}
```

#### 字段说明：

1. **Name**（工具名称）
   - 必须使用小写字母和下划线分隔（如 `read_file`, `git_add`）
   - 必须与工具注册名称一致
   - 用于工具调用时的标识

2. **DisplayName**（显示名称）
   - 自动从 `Name` 生成（通过 `GetToolDisplayName` 函数）
   - 格式：下划线转驼峰（如 `read_file` → `ReadFile`）
   - 用于日志和显示目的

3. **Description**（工具描述）
   - 清晰描述工具的功能和用途
   - AI 根据描述决定是否调用该工具
   - 应包含使用示例和注意事项

4. **Parameters**（参数定义）
   - JSON Schema 格式的参数定义
   - 必须包含 `type`、`properties`、`required` 等字段
   - 示例：
   ```go
   Parameters: map[string]any{
       "type": "object",
       "properties": map[string]any{
           "path": map[string]any{
               "type":        "string",
               "description": "文件路径，如main.go",
           },
       },
       "required":             []string{"path"},
       "additionalProperties": false,
   },
   ```

5. **Category**（工具分类）
   - 用于工具分组和管理
   - 现有分类：`file_ops`、`git`、`system`、`database`、`web` 等
   - 新工具应根据功能选择合适的分类

6. **Timeout**（超时时间）
   - 必须设置合理的超时时间
   - 防止工具执行时间过长阻塞系统
   - 示例：`30 * time.Second`

7. **Handler**（处理器函数）
   - 工具的实际执行函数
   - 函数签名：`func(ctx context.Context, args map[string]string) (string, error)`
   - 必须正确处理错误和超时

### 2. 工具注册机制

#### 2.1 工具注册表
```go
// toolRegistry 工具注册表
var toolRegistry = map[string]ToolDef{}

// RegisterTool 注册工具
func RegisterTool(tool ToolDef) {
    tool.DisplayName = GetToolDisplayName(tool.Name)
    toolRegistry[tool.Name] = tool
}
```

#### 2.2 工具注册时机
所有工具必须在 `init()` 函数中注册：
```go
func init() {
    RegisterTool(ToolDef{
        Name:        "read_file",
        Description: "读取项目内指定文件的内容",
        Parameters:  // ... 参数定义
        Category:    "file_ops",
        Timeout:     30 * time.Second,
        Handler:     handleReadFile,
    })
}
```

### 3. 工具调用流程

#### 3.1 工具调用处理
```go
// HandleToolCall 处理工具调用（带统计和超时）
func HandleToolCall(ctx context.Context, toolName string, argsRaw json.RawMessage) (string, error) {
    // 1. 从注册表获取工具定义
    // 2. 解析参数
    // 3. 设置超时上下文
    // 4. 执行工具处理器
    // 5. 记录使用统计
    // 6. 返回结果或错误
}
```

#### 3.2 工具调用上下文
工具处理器接收的 `context.Context` 包含：
- **超时控制**：通过 `context.WithTimeout` 设置
- **项目根目录**：通过 `ProjectRoot` 全局变量访问
- **工具显示名称**：通过 `ToolDisplayName` 上下文键访问

### 4. 工具开发规范

#### 4.1 新工具开发步骤
1. **定义处理器函数**：
   ```go
   func handleNewTool(ctx context.Context, args map[string]string) (string, error) {
       // 1. 参数验证
       // 2. 业务逻辑
       // 3. 错误处理
       // 4. 返回格式化结果
   }
   ```

2. **定义参数Schema**：
   ```go
   Parameters: map[string]any{
       "type": "object",
       "properties": map[string]any{
           "param1": map[string]any{
               "type":        "string",
               "description": "参数1描述",
           },
       },
       "required":             []string{"param1"},
       "additionalProperties": false,
   },
   ```

3. **注册工具**：
   ```go
   func init() {
       RegisterTool(ToolDef{
           Name:        "new_tool",
           Description: "新工具描述",
           Parameters:  // 参数定义
           Category:    "appropriate_category",
           Timeout:     30 * time.Second,
           Handler:     handleNewTool,
       })
   }
   ```

#### 4.2 工具结果格式
工具应返回格式化的结果字符串：
```go
result := fmt.Sprintf(`✅ 执行成功:
详细信息: %s

📊 执行统计:
执行时间: %v
状态: 成功`,
    details, executionTime)
```

#### 4.3 错误处理要求
1. **参数错误**：返回清晰的错误信息
2. **执行错误**：包含错误上下文
3. **超时错误**：正确处理context取消
4. **资源错误**：检查文件、网络等资源可用性

### 5. 工具测试要求

#### 5.1 必须包含的测试
1. **参数验证测试**：测试各种参数组合
2. **正常路径测试**：测试工具正常功能
3. **错误路径测试**：测试各种错误情况
4. **超时测试**：测试超时处理
5. **并发测试**：测试并发安全性

#### 5.2 测试示例
```go
func TestHandleNewTool(t *testing.T) {
    testCases := []struct {
        name     string
        args     map[string]string
        wantErr  bool
        contains string
    }{
        {
            name: "正常参数",
            args: map[string]string{"param1": "value1"},
            contains: "✅ 执行成功",
        },
        {
            name: "缺少必需参数",
            args: map[string]string{},
            wantErr: true,
            contains: "参数错误",
        },
    }
    
    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            result, err := handleNewTool(context.Background(), tc.args)
            // 验证逻辑...
        })
    }
}
```

### 6. 工具维护指南

#### 6.1 工具版本兼容性
1. **参数变更**：向后兼容，新增参数应为可选
2. **功能变更**：记录变更日志
3. **废弃工具**：标记为废弃，提供替代方案

#### 6.2 工具性能监控
1. **执行时间**：监控工具平均执行时间
2. **成功率**：监控工具调用成功率
3. **使用频率**：统计工具使用频率
4. **错误分析**：分析工具错误类型和频率

#### 6.3 工具文档要求
每个工具必须有：
1. **功能描述**：清晰的功能说明
2. **参数说明**：每个参数的详细说明
3. **使用示例**：典型使用场景示例
4. **注意事项**：使用时的注意事项
5. **错误代码**：可能的错误代码和含义

---
## 🛠️ 工具可靠性规范

### 1. 工具调用可靠性原则
工具调用必须遵循以下可靠性原则：

#### 1.1 重试机制
所有工具调用必须实现智能重试机制：
```go
type RetryConfig struct {
    MaxAttempts     int           // 最大重试次数（默认3）
    InitialDelay    time.Duration // 初始延迟（默认100ms）
    MaxDelay        time.Duration // 最大延迟（默认5s）
    BackoffFactor   float64       // 退避因子（默认2.0）
    RetryableErrors []error       // 可重试的错误类型
}

func RetryToolCall(toolFunc func() error, config RetryConfig) error {
    // 指数退避重试实现
}
```

#### 1.2 超时控制
每个工具必须设置合理的超时时间：
```go
// 引用完整的 ToolDef 定义（详见"核心类型定义"章节）
// ToolDef 包含以下字段：
// - Name: 工具名称
// - DisplayName: 显示名称
// - Description: 工具描述
// - Parameters: 参数定义（JSON Schema格式）
// - Category: 工具分类
// - Timeout: 工具执行超时时间（必须设置）
// - Handler: 工具处理器函数
```

**注意**：完整的 `ToolDef` 定义详见[📋 核心类型定义](#-核心类型定义)章节。
#### 1.3 结果验证
工具调用后必须验证结果完整性：
```go
func VerifyWriteFile(path, expectedContent string) error {
    // 1. 验证文件存在
    // 2. 验证文件内容
    // 3. 验证文件权限
    // 4. 返回验证结果
}
```

### 2. 错误处理最佳实践

#### 2.1 错误分类
```go
type ToolError struct {
    Type     ErrorType  // 错误类型：网络错误、参数错误、权限错误等
    ToolName string     // 工具名称
    Message  string     // 错误信息
    Retryable bool      // 是否可重试
}
```

#### 2.2 优雅降级
关键工具必须有降级方案：
```go
func CreateToolFallback(toolName string) FallbackFunc {
    switch toolName {
    case "write_file":
        return func(params map[string]interface{}) (interface{}, error) {
            // 降级方案：写入临时文件或返回错误信息
        }
    // ... 其他工具
    }
}
```

### 3. 监控和告警规范

#### 3.1 监控指标
必须监控以下指标：
- 工具调用成功率
- 平均响应时间
- 错误率分布
- 重试成功率
- 降级触发率

#### 3.2 告警规则
```go
type AlertRule struct {
    MetricName string
    Threshold  float64
    Duration   time.Duration
    Severity   AlertSeverity
}
```

### 4. 工具开发要求

#### 4.1 新工具开发规范
开发新工具时必须：
1. 实现完整的错误处理
2. 设置合理的超时时间
3. 提供结果验证方法
4. 实现降级策略
5. 添加监控指标

#### 4.2 工具测试要求
工具测试必须包含：
1. 正常路径测试
2. 错误路径测试
3. 超时测试
4. 重试测试
5. 降级测试

---

## 📅 日期处理规范

### 1. 系统提示词中的日期
系统提示词中的日期必须动态生成，使用当前系统时间：

```go
// ✅ 正确：动态生成日期
currentDate := time.Now().Format("2006年01月02日")
prompt := `当前日期：` + currentDate + `，请基于当前日期处理与日期相关的需求。`

// ❌ 错误：硬编码日期
prompt := `当前日期：2024年，请基于当前日期处理与日期相关的需求。`
```

### 2. 文档中的日期
所有文档中的日期必须反映实际更新时间：

```markdown
# 文档标题

最后更新：2026年03月03日
维护者：dscli 开发团队
```

### 3. 版本信息中的日期
版本信息应包含构建日期：

```go
// version.go
var (
    Version = "0.5.4"
    Build   = "2026-03-03"
)
```

### 4. 测试中的日期处理
测试中需要处理日期相关逻辑时，应使用可预测的日期：

```go
func TestDateRelatedFunction(t *testing.T) {
    // 使用固定日期进行测试
    testDate := time.Date(2026, 3, 3, 0, 0, 0, 0, time.Local)
    // 测试逻辑...
}
```

---

## 🧪 单元测试规范

### 1. 测试文件命名
测试文件必须与源文件一一对应：
```
[功能].go          ← 源文件
[功能]_test.go     ← 测试文件
```

### 2. 测试函数命名
测试函数使用 `Test` 前缀，描述性名称：
```go
func TestFunctionName(t *testing.T) {
    // 测试逻辑
}

func TestFunctionName_EdgeCase(t *testing.T) {
    // 边界条件测试
}

func TestFunctionName_ErrorHandling(t *testing.T) {
    // 错误处理测试
}
```

### 3. 测试用例组织
使用表格驱动测试：
```go
func TestFunctionName(t *testing.T) {
    testCases := []struct {
        name     string
        input    string
        expected string
        wantErr  bool
    }{
        // 正常路径
        {"normal case", "input", "expected", false},
        // 边界条件
        {"empty string", "", "", false},
        {"exact boundary", strings.Repeat("a", 72), strings.Repeat("a", 72), false},
        // 错误路径
        {"invalid input", "invalid", "", true},
    }
    
    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            // 测试逻辑
        })
    }
}
```

### 4. 必须包含的测试类型
1. **正常路径测试**：基本功能验证
2. **边界条件测试**：空值、最大值、最小值等
3. **错误路径测试**：无效输入、异常情况
4. **并发安全测试**：涉及并发的函数
5. **性能基准测试**：关键性能路径

### 5. 测试覆盖率要求
- 总体覆盖率不低于80%
- 关键功能覆盖率不低于90%
- 错误处理路径必须覆盖

### 6. 测试质量指标
| 指标 | 要求 | 检查方式 |
|------|------|----------|
| **测试覆盖率** | ≥80% | `go test -cover` |
| **边界测试** | 必须包含 | 代码审查 |
| **错误测试** | 必须包含 | 代码审查 |
| **测试可读性** | 清晰易懂 | 代码审查 |
| **测试独立性** | 不依赖外部 | 独立运行 |

---

*最后更新：2026年03月03日*
*维护者：dscli 开发团队*