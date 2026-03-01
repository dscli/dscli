# 🔄 dscli 热重载功能

## 概述
热重载功能允许在 `dscli chat` 会话中重启进程，使用新编译的版本继续对话，无需手动重启会话。

## 问题背景
当修改 `dscli` 代码后：
1. 需要重新编译安装：`make install`
2. 当前会话使用的是旧版本 `dscli`
3. 需要退出会话并重新启动才能使用新功能

## 解决方案
通过热重载机制，可以在会话中自动重启进程：

### 工作流程
```
用户会话 → AI回复 → 工具调用 → 输出"exec dscli chat --reload"
    ↓
HandleToolCalls 检测到重载命令
    ↓
执行 exec dscli chat --reload
    ↓
新进程启动（带--reload标记）
    ↓
handleReload 函数恢复对话
    ↓
找到未完成的工具调用
    ↓
继续正常对话流程
```

## 使用方法

### 1. 修改代码后
```bash
# 1. 修改 dscli 代码
vim chat.go

# 2. 编译安装新版本
make install

# 3. 在现有 dscli chat 会话中，让 AI 输出：
exec dscli chat --reload
```

### 2. AI 如何触发重载
当 AI 需要重启进程时，可以通过工具调用输出重载命令：
```bash
# AI 可以通过 execute_script 工具输出重载命令
echo "exec dscli chat --reload"
```

### 3. 自动检测
`HandleToolCalls` 函数会自动检测工具执行结果中的重载命令：
```go
if strings.Contains(result, "exec dscli chat --reload") {
    Info("🔄 检测到重载命令，正在重启进程...")
    // 执行 exec 替换进程
}
```

## 技术实现

### 1. 重载命令检测 (`tools.go`)
```go
// HandleToolCalls 中检测重载命令
if strings.Contains(result, "exec dscli chat --reload") {
    cmd := exec.Command("bash", "-c", "exec dscli chat --reload")
    cmd.Dir = ProjectRoot
    cmd.Stdin = os.Stdin
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    
    if err := cmd.Run(); err != nil {
        Error("重载失败: %v", err)
        result = fmt.Sprintf("重载失败: %v", err)
    } else {
        os.Exit(0)
    }
}
```

### 2. 重载进程处理 (`chat.go`)
```go
func handleReload(ctx context.Context, prompts []Message, skills []Message, history []Message) (err error) {
    Info("🔄 检测到重载进程，正在恢复对话...")
    
    // 找到未完成的工具调用
    var lastAssistant *Message
    for idx := len(history) - 1; idx >= 0; idx-- {
        if history[idx].Role == "assistant" && len(history[idx].ToolCalls) > 0 {
            lastAssistant = &history[idx]
            break
        }
    }
    
    // 执行未完成的工具调用并继续对话
    // ...
}
```

### 3. 命令行标志
```go
chatCmd.Flags().BoolVar(&reload, "reload", false, "重载进程（内部使用）")
```

## 使用示例

### 场景：改进工具调用信息显示
1. **发现问题**：用户看到重复的"调用 1 个工具..."信息
2. **修改代码**：改进 `ChatRound` 函数，添加工具名称显示
3. **编译安装**：`make install`
4. **触发重载**：在会话中让 AI 输出 `exec dscli chat --reload`
5. **验证效果**：新进程启动，使用改进后的代码继续对话

### 预期输出
```
# 重载前（旧版本）
调用 1 个工具...
调用 1 个工具...
调用 1 个工具...

# AI 输出重载命令
exec dscli chat --reload

# 重载进程启动
🔄 检测到重载进程，正在恢复对话...
🔄 检测到重载命令，正在重启进程...

# 重载后（新版本）
调用 1 个工具：
  1. write_file
继续调用 1 个工具...
```

## 注意事项

### 1. 进程状态
- `exec` 会替换当前进程，继承环境变量和文件描述符
- 标准输入/输出/错误流保持不变
- 当前工作目录保持不变

### 2. 对话恢复
- 重载进程会自动恢复未完成的工具调用
- 对话历史从数据库加载
- 工具调用ID保持一致性

### 3. 递归处理
- 重载进程可能再次触发重载（支持多次重载）
- 每次重载都会清除 `reload` 标记
- 递归深度正确维护

### 4. 错误处理
- 如果 `exec` 失败，返回错误信息给 AI
- 如果找不到未完成的工具调用，继续正常对话
- 记录详细的日志信息

## 优势总结

✅ **无缝升级**：无需手动重启会话
✅ **上下文保持**：对话历史完整保留
✅ **自动恢复**：未完成的工具调用自动继续
✅ **多次支持**：支持多次重载操作
✅ **错误处理**：完善的错误检测和恢复机制
✅ **向后兼容**：不影响现有功能

## 相关提交
- `feat: 改进工具调用信息显示，避免重复和混淆`
- `feat: 实现热重载功能，支持在会话中重启dscli进程`

## 未来改进
1. 添加重载确认提示
2. 支持部分重载（仅重载特定模块）
3. 添加重载统计信息
4. 优化重载性能
