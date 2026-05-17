---
name: go-fix
description: Go 代码现代化助手。go fix 工具自动发现并应用新语言特性（如 strings.SplitSeq、fmt.Appendf、omitzero 等）。
keywords:
- go
- go-fix
- modernize
- refactor
- analyzer
---

# go-fix：Go 代码现代化助手

Go 1.24+ 内置的 `go fix` 工具，自动发现并应用新语言特性。

## 核心命令

```bash
go fix -diff ./...           # 预览所有建议（推荐第一步）
go fix ./...                 # 应用所有建议
go fix -diff -NAME ./...     # 预览特定分析器（-slicescontains 等）
go fix -NAME ./...           # 仅应用特定分析器
```

选择分析器用 `-NAME` 标志（如 `-slicescontains`、`-stringsseq`），不是数字。

## 全部分析器（22个）

| 标志 | 说明 |
|------|------|
| `-any` | `interface{}` → `any` |
| `-buildtag` | 检查 `//go:build` 和 `// +build` 指令 |
| `-fmtappendf` | `[]byte(fmt.Sprintf)` → `fmt.Appendf` |
| `-forvar` | 移除冗余的循环变量重声明 |
| `-hostport` | 检查 `net.Dial` 地址格式 |
| `-inline` | 应用 `//go:fix inline` 指令 |
| `-mapsloop` | 手动 map 遍历 → `maps` 包 |
| `-minmax` | `if/else` → `min`/`max` |
| `-newexpr` | 利用 go1.26 `new(expr)` 简化代码 |
| `-omitzero` | `json:",omitempty"` → `json:",omitzero"` ⚠️ 语义不同 |
| `-plusbuild` | 移除废弃的 `//+build` 注释 |
| `-rangeint` | 三段式 for → `for range n` |
| `-reflecttypefor` | `reflect.TypeOf(x)` → `TypeFor[T]()` |
| `-slicescontains` | `for range` 查找 → `slices.Contains` |
| `-slicessort` | `sort.Slice` → `slices.Sort` |
| `-stditerators` | `Len`/`At` 风格 API → 迭代器 |
| `-stringsbuilder` | `+=` 拼接 → `strings.Builder` |
| `-stringscut` | `strings.Index` 等 → `strings.Cut` |
| `-stringscutprefix` | `HasPrefix`/`TrimPrefix` → `CutPrefix` |
| `-stringsseq` | `Split`/`Fields` → `SplitSeq`/`FieldsSeq`（零分配） |
| `-testingcontext` | `context.WithCancel` → `t.Context` |
| `-waitgroup` | `wg.Add(1)/go/wg.Done()` → `wg.Go` |

完整列表运行：`go tool fix help`

## 工作流

1. **预览**：`go fix -diff ./...` 查看所有建议
2. **审查**：逐条检查是否适合项目
3. **应用**：`go fix ./...` 或 `go fix -NAME ./...` 逐个应用
4. **验证**：`go fix -diff ./...` 确认无残留建议
5. **提交**：git commit

## ⚠️ 关键原则

1. **先 -diff 预览，不盲用** — 部分建议可能有语义变化
2. **行为变化要警惕** — 如 `omitzero`（空值零值都忽略）vs `omitempty`（只忽略空值）
3. **CI 可集成** — `go fix -diff ./... && echo "clean"` 失败说明有未应用的建议
4. **逐分析器应用更安全** — `go fix -diff -NAME` 逐个审查

## 实战案例

```
# 原代码
sb.WriteString(fmt.Sprintf("value: %d\n", v))

# go fix 建议（-fmtappendf）
fmt.Fprintf(&sb, "value: %d\n", v)
```

减少一次 `fmt.Sprintf` 的内存分配。
