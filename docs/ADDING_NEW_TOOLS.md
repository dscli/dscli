# 如何添加新工具到 dscli

本文档详细介绍了如何为 dscli 添加新的工具，包括工具注册、参数定义、处理器实现等。

## 目录

1. [工具架构概述](#工具架构概述)
2. [工具命名约定](#工具命名约定)
3. [添加新工具的步骤](#添加新工具的步骤)
4. [工具定义详解](#工具定义详解)
5. [处理器实现指南](#处理器实现指南)
6. [工具分类](#工具分类)
7. [最佳实践](#最佳实践)
8. [示例：git_am 工具实现](#示例git_am-工具实现)
9. [常见问题](#常见问题)
## 工具架构概述

dscli 的工具系统基于以下组件：

- **ToolDef**: 工具定义结构体，包含工具名称、描述、参数等
- **工具注册表**: 全局的 `toolRegistry` 映射，存储所有工具定义
- **工具处理器**: 处理工具调用的函数，接收参数并返回结果
- **工具分类**: 将工具按功能分类（git、system、code 等）

## 工具命名约定

### 命名规则
- 使用小写字母和下划线：`git_am`、`read_file`、`issue_create`
- 保持与底层命令一致：`git_am` 对应 `git am`
- 使用有意义的名称：`git_format_patch` 而不是 `git_fp`

### 特殊命名说明
- **git_am**: 全称是 "apply patch from mail"，用于应用邮件格式的patch
  - 这个命名考虑了将来可能集成邮件功能
  - 与 `git format-patch` 配合使用，形成完整的patch工作流
  - 支持邮件格式的RFC 2822标准patch

### 命名示例
- `git_add` - Git添加文件
- `git_commit` - Git提交更改
- `git_format_patch` - 生成patch文件
- `git_am` - 应用patch文件（apply patch from mail）
- `read_code_section` - 读取代码片段
- `write_code_section` - 写入代码片段
## 添加新工具的步骤

添加新工具到 dscli 需要以下步骤：

### 步骤 1：创建工具文件
1. 在项目根目录下创建新的 `.go` 文件，如 `new_tool.go`
2. 文件命名建议：`工具名.go`，如 `git_am.go`

### 步骤 2：定义工具处理器
1. 实现工具处理函数：`func handleToolName(ctx context.Context, args map[string]string) (string, error)`
2. 函数名格式：`handle` + 工具名（首字母大写），如 `handleGitAm`

### 步骤 3：注册工具
1. 在 `init()` 函数中调用 `RegisterTool()`
2. 提供完整的工具定义，包括名称、描述、参数等

### 步骤 4：编译测试
```bash
go build -o dscli .
```

### 步骤 5：验证工具
1. 启动 dscli：`./dscli`
2. 检查工具是否出现在可用工具列表中
3. 测试工具功能是否正常
## 工具定义详解
### ToolDef 结构体

```go
type ToolDef struct {
    Name        string   // 工具名称（小写字母和下划线）
    DisplayName string   // 显示名称（自动生成）
    Description string   // 工具描述（详细说明功能和使用方法）
    Parameters  map[string]any  // 参数定义（JSON Schema）
    Category    string   // 工具分类
    Timeout     time.Duration  // 执行超时时间
    Handler     func(ctx context.Context, args map[string]string) (string, error)
}
```

### 参数定义（JSON Schema）

参数定义使用 JSON Schema 格式，示例：

```go
Parameters: map[string]any{
    "type": "object",
    "properties": map[string]any{
        "param1": map[string]any{
            "type":        "string",
            "description": "参数1的描述",
        },
        "param2": map[string]any{
            "type":        "string",
            "description": "参数2的描述",
        },
    },
    "required":             []string{"param1"}, // 必需参数
    "additionalProperties": false, // 禁止额外参数
},
```

### 参数类型支持

- `string`: 字符串类型，只支持字符串类型，需要其他类型自己转换

## 处理器实现指南

### 基本模式

```go
func handleToolName(ctx context.Context, args map[string]string) (string, error) {
    // 1. 获取参数
    param1, ok := args["param1"]
    if !ok {
        param1 = "默认值"
    }
    
    // 2. 参数验证
    if param1 == "" {
        return "", fmt.Errorf("param1不能为空")
    }
    
    // 3. 执行操作
    result, err := doSomething(param1)
    if err != nil {
        return "", fmt.Errorf("操作失败: %w", err)
    }
    
    // 4. 返回结果
    return result, nil
}
```

### 使用现有基础设施

#### 执行 Shell 命令

```go
// 使用 gitCommand（适用于 git 命令）
out, err := gitCommand(ctx, "status", "--short")

// 使用 ShellExec（适用于任意命令）
ctx = context.WithValue(ctx, ShellName, "bash")
ctx = context.WithValue(ctx, ShellArgs, []string{})
result, err := ShellExec(ctx, "echo Hello")
```

#### 输出日志

```go
Println("执行操作:", param1)  // 普通日志
Notice("重要操作:", param1)   // 通知日志
Error("操作失败:", err)       // 错误日志
```

### 错误处理

- 返回有意义的错误信息
- 使用 `fmt.Errorf` 包装错误
- 考虑超时和取消

```go
if ctx.Err() == context.DeadlineExceeded {
    return "", fmt.Errorf("操作超时")
}
```

## 工具分类

现有工具分类：

- **git**: Git 相关操作（git_add、git_commit、git_log 等）
- **system**: 系统操作（shell、python）
- **code**: 代码操作（read_code_section、write_code_section 等）
- **file**: 文件操作（read_file、write_file 等）
- **issue**: Issue 管理（issue_create、issue_list 等）
- **web**: 网络操作（web_reader）

## 最佳实践

### 1. 描述清晰
- 提供详细的工具描述
- 说明参数用途和格式
- 包含使用示例

### 2. 参数设计
- 使用有意义的参数名
- 提供默认值（如果需要）
- 验证参数有效性

### 3. 错误处理
- 提供具体的错误信息
- 处理边界情况
- 记录错误日志

### 4. 性能考虑
- 设置合理的超时时间
- 避免长时间阻塞的操作
- 使用上下文传递取消信号

### 5. 安全性
- 验证用户输入
- 避免执行危险命令
- 限制资源使用

## 示例：git_am 工具实现

以下是完整的 `git_am.go` 实现示例：
```go
package main

import (
    "context"
    "fmt"
    "strings"
    "time"
)

func init() {
    RegisterTool(ToolDef{
        Name:        "git_am",
        Description: `应用通过git format-patch生成的patch文件。
git am 全称是 "apply patch from mail"，专门用于应用邮件格式的patch。

主要功能：
1. 应用patch：将patch内容应用到当前分支
2. 错误处理：支持--continue、--skip、--abort等恢复选项
3. 邮件格式：支持RFC 2822标准邮件格式的patch
4. 与git format-patch配合：形成完整的patch工作流

参数说明：
- patch: patch内容（必需），通过git format-patch生成的RFC 2822格式patch
- options: git am选项（可选），如--continue、--skip、--abort、--quit、--show-current-patch

使用示例：
1. 应用patch：git_am(patch="从patch内容...")
2. 继续应用：git_am(options="--continue")
3. 放弃应用：git_am(options="--abort")

注意：patch内容较长时建议通过标准输入传递，避免命令行长度限制。`,
        Parameters: map[string]any{
            "type": "object",
            "properties": map[string]any{
                "patch": map[string]any{
                    "type":        "string",
                    "description": "patch内容（RFC 2822格式），通过git format-patch生成",
                },
                "options": map[string]any{
                    "type":        "string",
                    "description": `git am选项，支持：
1. 应用选项：--signoff、--keep、--3way等（默认无选项）
2. 恢复选项：--continue、--skip、--abort、--quit、--show-current-patch
多个选项用空格分隔，例如：--signoff --3way`,
                },
            },
            "required":             []string{},
            "additionalProperties": false,
        },
        Category: "git",
        Timeout:  120 * time.Second, // git am可能需要较长时间
        Handler:  handleGitAm,
    })
}

// handleGitAm 处理git am命令（apply patch from mail）
// git am 全称是 "apply patch from mail"，用于应用邮件格式的patch
func handleGitAm(ctx context.Context, args map[string]string) (string, error) {
    patch, hasPatch := args["patch"]
    options, hasOptions := args["options"]

    // 如果没有提供patch和options，返回错误
    if !hasPatch && !hasOptions {
        return "", fmt.Errorf("必须提供patch内容或options参数")
    }

    // 构建git am命令
    gitArgs := []string{"am"}

    // 添加选项
    if hasOptions && options != "" {
        optionList := strings.Fields(options)
        gitArgs = append(gitArgs, optionList...)
    }

    // 如果提供了patch内容，通过标准输入传递
    if hasPatch && patch != "" {
        // 设置context值，指定使用git作为解释器
        ctx = context.WithValue(ctx, ShellName, "git")
        ctx = context.WithValue(ctx, ShellArgs, gitArgs)
        
        Println("git", strings.Join(gitArgs, " "))
        return ShellExec(ctx, patch)
    } else {
        // 如果没有patch内容，直接执行git am命令（用于--continue等操作）
        Println("git", strings.Join(gitArgs, " "))
        return gitCommand(ctx, gitArgs...)
    }
}
```

## 常见问题

### Q: 工具没有出现在列表中
A: 检查工具是否成功注册，确保 `init()` 函数被调用。

### Q: 参数解析失败
A: 检查参数定义是否符合 JSON Schema 格式。

### Q: 工具执行超时
A: 调整 `Timeout` 字段的值，或检查处理器是否正确处理上下文取消。

### Q: 如何调试工具
A: 使用 `Println`、`Notice`、`Error` 等函数输出调试信息。

## 总结

添加新工具到 dscli 是一个系统化的过程，遵循以下步骤：

1. **设计工具接口**：明确工具的功能和参数
2. **实现处理器**：编写处理逻辑，注意错误处理和资源管理
3. **注册工具**：在 `init()` 函数中注册工具定义
4. **测试验证**：编译测试，确保工具正常工作
5. **文档记录**：更新相关文档，记录工具使用方法

通过遵循这些指南，你可以轻松地为 dscli 添加新的功能强大的工具。