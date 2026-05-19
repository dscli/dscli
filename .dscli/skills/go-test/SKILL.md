---
name: go-test
description: Go 测试最佳实践与自动化脚本。测试运行(run.sh)、反模式检查(lint.sh)、config 隔离脚手架。
keywords:
- go
- test
- testing
- 测试
- table-driven
- mock
- t.Run
- t.Helper
- integration
- e2e
- lint
- run
- config
- scaffold
---

# Go 测试

## 快速开始

```bash
# 运行全部测试（含 race detector）
bash ~/.dscli/skills/go-test/scripts/run.sh

# 只跑单元测试（跳过集成/e2e）
bash ~/.dscli/skills/go-test/scripts/run.sh --short

# 检查测试反模式
bash ~/.dscli/skills/go-test/scripts/lint.sh

# 向测试文件添加 config 隔离 helper
bash ~/.dscli/skills/go-test/scripts/setup-config-isolation.sh internal/pkg/my_test.go
```

## 核心原则

1. **测试行为，不测试实现** — 从用户角度验证契约
2. **单元+集成互补** — 单元测试验证逻辑正确性，集成测试发现真实环境问题
3. **只测自有逻辑** — 不测试标准库或第三方库的行为

## 项目测试模式

### Config 隔离（全局状态安全）

项目用 `withConfig` helper 隔离 config 全局状态——设置值，`t.Cleanup` 自动恢复。
`lp/lightpanda_test.go` 中有完整实现。新文件用脚本一键添加。

```bash
bash ~/.dscli/skills/go-test/scripts/setup-config-isolation.sh internal/mypackage/my_test.go
```

### 函数变量替换（mock 策略）

项目不用接口 mock，用函数变量替换 + `restoreFuncVars` 恢复：
- 在测试函数开头 `defer restoreFuncVars(...)`
- `lightpanda_test.go:285` 是项目参考实现

### 运行时跳过（集成测试）

不设 build tag，运行时检查外部依赖：

```go
path, err := exec.LookPath("external-tool")
if err != nil {
    t.Skip("external-tool not installed")
}
```

项目有 9 处使用此模式。测试始终编译，CI 有依赖时自动跑，本地无依赖时自动跳过。

### 进程管理（e2e 测试）

参考 `lightpanda_integration_test.go:TestGet_EndToEnd`：
1. `exec.LookPath` 检查依赖
2. `httptest.NewServer` 搭建测试服务器
3. `exec.CommandContext` + `cmd.Start` 启动外部进程
4. 轮询等待就绪
5. `defer cmd.Process.Kill()` 清理
6. 调用被测 API，验证输出

### 表驱动 + t.Run

沿用项目现有范式。关键：只测自有逻辑，不测标准库。

### 错误消息

`t.Errorf("Func(%q) = (%v, %v), want (%v, %v)", ...)` — got/want 格式，包含上下文。
`t.Fatal` 用于不可恢复的判断，`t.Error` 用于可继续的断言。

## 反模式清单

`lint.sh` 自动检测前 4 项。

| 反模式 | 检测 |
|--------|------|
| `config.Set` 未通过 `withConfig` 隔离 | ✅ 自动 |
| 函数变量赋值未 defer restore | ✅ 自动 |
| 空 `_test.go` 文件 | ✅ 自动 |
| helper 函数缺少 `t.Helper()` | ✅ 自动 |
| 测试标准库行为（`url.Parse` 等） | 人工 |
| 循环论证 mock（mock 返回期望值，断言输出含期望值）| 人工 |
| 过度 mock（mock 全部依赖，测的是 mock 逻辑）| 人工 |
| 测试间全局状态污染 | 人工 |

## 什么该测 / 不该测

| 该测 | 不该测 |
|------|--------|
| 导出函数的输入/输出契约 | 简单 getter/setter |
| 错误路径和边界条件 | 标准库行为（`url.Parse` 等） |
| 项目特有的解析/构造逻辑 | 纯数据结构 |
| 并发控制流（锁、重试） | 第三方库内部 |
| 完整用户场景（端到端） | 仅 mock 所有依赖的假测试 |

## Scripts

- `scripts/run.sh`: 标准测试运行器（race, vet, 汇总）
- `scripts/lint.sh`: 反模式自动检测
- `scripts/setup-config-isolation.sh`: 添加 `withConfig` helper
