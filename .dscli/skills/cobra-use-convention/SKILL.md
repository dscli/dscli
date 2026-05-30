---
name: cobra-use-convention
description: Cobra Use 行参数约定：[ ] 表示可选，< > 或裸写表示必选。别盲抄现有代码的 Use 格式——现有代码也可能写错了。
author: Bohr <bohr@dscli.io>
keywords:
- cobra
- use
- convention
- bracket
- arg
- required
- optional
- help
---

# Cobra Use 行参数约定

## 核心规则

Cobra 命令的 `Use` 字段遵循：

| 写法 | 含义 | 示例 |
|------|------|------|
| `arg` 或 `<arg>` | **必选**参数 | `update <id> <path>` |
| `[arg]` | **可选**参数 | `list [filter]` |
| `[flags]` | flag 标记（Cobra 自动追加） | |

**关键**：如果用了 `Args: cobra.ExactArgs(2)`，参数就是必选的，Use 行里就该用 `<arg>` 而不是 `[arg]`。

## 别盲抄现有代码

这是本 skill 最重要的教训。当你给现有项目加新命令时：

1. ❌ 看到旁边的命令写 `Use: "assign [a] [b]"` + `ExactArgs(2)`，就照抄格式
2. ✅ 停下来想：`[brackets]` 对必选参数来说是不是写错了？

**现有代码 ≠ 正确代码。** 老代码可能有多年没人纠正的坏习惯。加新代码时，你有责任先查规范、再写，而不是把坏习惯扩散。

## 检查清单

写 Cobra 命令时：

- [ ] Use 行里必选参数用 `<name>` 或裸写，不用 `[name]`
- [ ] 可选参数才用 `[name]`
- [ ] `Args` 验证器（`ExactArgs`/`MinimumNArgs` 等）与 Use 行一致
- [ ] 运行 `cmd --help` 看 Usage 行是否合理

## 相关教训

同样的原则适用于其他场景：
- 别因为现有代码没用 `errors.Is` 就继续用 `==` 比 sentinel error
- 别因为现有代码没写 `rows.Err()` 检查就省略
- 别因为现有代码用 `fmt.Errorf` 不 wrap 就继续裸传

**参考现有模式，但用你自己的判断力。**
