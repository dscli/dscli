---
name: gofumpt
description: gofumpt 严格 Go 格式化工具。比 gofmt 更严格、向后兼容的 Go 代码格式化器，由 mvdan 创建。涵盖所有格式化规则、-extra 选项和诊断技巧。
keywords:
- gofumpt
- go
- format
- fmt
- gofmt
- formatter
- mvdan
- style
---

# gofumpt

更严格的 Go 代码格式化器。`gofumpt` = **go** + **fumpt** (mvdan 的命名风格)，是 `gofmt` 的增强分支。

## 核心定位

| 工具 | 做什么 | 关系 |
|------|--------|------|
| `gofmt` | Go 官方格式化 | 基础 |
| `go fix` | Go 现代化迁移（API 变更、新语言特性） | 互补 |
| `gofumpt` | 比 gofmt 更严格的格式化规则 | 超集 |

`gofumpt` 是 `gofmt` 的**严格超集**——所有 gofumpt 接受的代码 gofmt 也接受，反之不一定。运行 `gofmt` 后 `gofumpt` 不会再产生变更。

## 安装

```bash
go install mvdan.cc/gofumpt@latest
```

要求 Go 1.25+（fork 自 Go 1.26.0 的 `cmd/gofmt`）。

## 基础用法

```bash
# 格式化并写入（类似 gofmt -w）
gofumpt -l -w .

# 只显示需要格式化的文件
gofumpt -l .

# 与 goimports 配合
goimports -w file.go && gofumpt -w file.go
```

`vendor` 和 `testdata` 目录默认跳过（除非显式传入）。生成的 Go 文件也不应用额外规则（除非显式传入）。

## 所有格式化规则

### 基础规则（默认启用）

1. **赋值运算符后不换行** — `foo :=` 后不再另起一行接 `"bar"`
2. **函数体无首尾空行** — `func foo() {` 和 `}` 之间不留空行
3. **多行函数签名 `) {` 分离** — 参数换行时 `)` 独占一行，增强可读性
4. **单语句块无空行** — `if err != nil { return err }` 干净无空行
5. **简单错误检查前无空行** — `foo, err := ...` 紧接着 `if err != nil`
6. **复合字面量换行一致** — 任一元素换行则所有元素都换行，大括号对齐
7. **多行函数调用的括号对齐** — 开闭括号缩进一致
8. **空字段列表单行** — `interface{}`、`struct{}`、`func()` 不展开
9. **std 导入独立分组置顶** — `"io"` 等标准库导入与其他隔开
10. **短 case 子句单行** — `case 'a', 'b', 'c':` 不换行
11. **多行顶层声明用空行分隔** — 多行函数/类型之间必须有空行
12. **单变量声明不用括号** — `var foo = "bar"` 而非 `var (foo = "bar")`
13. **连续顶层声明合并** — 多个 `var`/`const` 合并为一个 `var (...)` 块
14. **简单 var 用 `:=`** — `s := "string"` 替代 `var s = "string"`
15. **`-s` 简化始终启用** — 等价于 `gofmt -s`（如 `[]int{1}` → `{1}`）
16. **八进制用 `0o` 前缀** — `0755` → `0o755`（Go 1.13+ 模块）
17. **非指令注释必须空格** — `// Foo` 而非 `//Foo`（`//go:noinline` 等指令除外）
18. **复合字面量无首尾空行** — 大括号内部不留首尾空行
19. **字段列表无首尾空行** — 接口/结构体内部不留首尾空行
20. **移除肯定无用的括号** — `chan (int)` → `chan int`，`f((3))` → `f(3)`

### `-extra` 规则（需显式启用）

```bash
gofumpt -extra -w .
```

21. **同类型相邻参数合并** — `func Foo(a string, b string)` → `func Foo(a, b string)`
22. **避免裸 return** — 命名返回值函数中 `return` → `return err`，增加清晰度

## 诊断技巧

在文件中插入 `//gofumpt:diagnose` 注释，运行 gofumpt 后会输出版本和语言版本信息：

```go
//gofumpt:diagnose
```

输出示例：
```
//gofumpt:diagnose v0.1.1-0.20211103104632-bdfa3b02e50a -lang=go1.16
```

## 与 go fix 的关系

- **gofumpt**：专注于**代码外观**（空格、换行、括号），不改变语义
- **go fix**：专注于**代码现代化**（API 迁移、新语言特性），可能改变语义
- 两者互补：`go fix` 先现代化，`gofumpt` 再严格格式化

## dscli 项目使用

本项目推荐在提交前运行：

```bash
gofumpt -extra -w .
```

结合已有的 `go fix` skill 完成代码现代化，再用 `gofumpt` 统一格式。
