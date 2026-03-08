# 代码结构感知工具开发计划 - 已完成 ✅

## 项目目标
创建一套基于代码结构而非行号的文件操作工具，使LLM能更安全、更准确地操作代码文件。

## 工具列表

### 1. read_code_structure.go - 读取代码文件的结构信息
**状态**: ✅ 已完成 (2026-03-08)
**功能**: 读取代码文件的结构信息（函数、类、方法、导入等）
**依赖**: 无

**任务清单**:
- [x] 创建 `code_read_structure.go` 文件
- [x] 实现 `readCodeStructure` 函数
- [x] 设计工具参数和返回值格式
- [x] 注册工具到LLM
- [x] 验证工具在对话中可用

**接口设计**:
```go
// 读取代码文件的结构信息
func readCodeStructure(path string) (FileStructure, error)
```

### 2. read_code_section.go - 基于结构读取代码片段
**状态**: ✅ 已完成 (2026-03-08)
**功能**: 基于代码结构定位并读取特定代码片段
**依赖**: `code_read_structure.go`

**任务清单**:
- [x] 创建 `code_read_section.go` 文件
- [x] 实现 `readCodeSection` 函数
- [x] 设计代码片段定位语法
- [x] 注册工具到LLM
- [x] 验证工具在对话中可用

**接口设计**:
```go
// 基于结构读取代码片段
func readCodeSection(path string, selector string) (string, error)
```

### 3. write_code_section.go - 基于结构修改代码片段
**状态**: ✅ 已完成 (2026-03-08)
**功能**: 基于代码结构定位并修改特定代码片段
**依赖**: `code_read_structure.go`, `code_read_section.go`

**任务清单**:
- [x] 创建 `code_write_section.go` 文件
- [x] 实现 `writeCodeSection` 函数
- [x] 设计安全写入机制
- [x] 添加dry-run模式
- [x] 注册工具到LLM
- [x] 验证工具在对话中可用

**接口设计**:
```go
// 基于结构修改代码片段
func writeCodeSection(path string, selector string, newContent string, dryRun bool) (string, error)
```

### 4. search_code_semantic.go - 语义搜索代码
**状态**: ✅ 已完成 (2026-03-08)
**功能**: 基于语义搜索代码中的特定模式
**依赖**: `code_read_structure.go`

**任务清单**:
- [x] 创建 `code_search_semantic.go` 文件
- [x] 实现 `searchCodeSemantic` 函数
- [x] 设计语义搜索算法
- [x] 注册工具到LLM
- [x] 验证工具在对话中可用

**接口设计**:
```go
// 基于语义搜索代码
func searchCodeSemantic(path string, pattern string, contextLines int, caseSensitive bool, maxMatches int) (string, error)
```

## 2026-03-08 任务完成总结

✅ **已完成四个代码操作工具的开发：**

1. **`read_code_structure`** - 读取代码文件的结构信息（函数、类、方法、导入等）
2. **`read_code_section`** - 基于代码结构定位并读取特定代码片段
3. **`write_code_section`** - 基于代码结构定位并修改特定代码片段
4. **`search_code_semantic`** - 基于语义搜索代码中的特定模式

**主要功能特点：**
- 支持函数、类、方法、导入等代码结构的识别
- 支持基于代码结构的精确定位
- 支持语义搜索和上下文显示
- 支持dry-run模式预览修改
- 工具注册名避免冲突（如 `search_code_semantic` 避免与 `file.go` 冲突）

**技术要点：**
- 使用Go标准库的 `go/parser` 和 `go/ast` 进行代码解析
- 实现智能的上下文管理和重叠区域避免
- 支持可选参数和默认值
- 完整的错误处理和参数验证

所有工具已成功注册到系统中，可以通过chat命令调用。

## 2026-03-08 新增工具

### 5. git_am.go - 应用patch文件
**状态**: ✅ 已完成 (2026-03-08)
**功能**: 应用通过git format-patch生成的patch文件（apply patch from mail）
**依赖**: 现有的git工具基础设施

**任务清单**:
- [x] 创建 `git_am.go` 文件
- [x] 实现 `handleGitAm` 函数
- [x] 支持通过标准输入传递patch内容
- [x] 支持--continue、--skip、--abort等恢复选项
- [x] 注册工具到LLM
- [x] 验证工具在对话中可用

**接口设计**:
```go
// 应用patch文件
func handleGitAm(ctx context.Context, args map[string]string) (string, error)
```

### 6. 工具添加文档
**状态**: ✅ 已完成 (2026-03-08)
**功能**: 提供详细的工具添加指南和最佳实践
**文件**: `docs/ADDING_NEW_TOOLS.md`

**内容要点**:
- [x] 工具架构概述
- [x] 添加新工具的步骤
- [x] 工具定义详解
- [x] 处理器实现指南
- [x] 工具分类说明
- [x] 最佳实践
- [x] 完整示例（git_am工具）
- [x] 工具命名约定

## 设计原则

1. **LLM友好**: 工具接口要符合LLM的认知模式
2. **结构导向**: 基于代码结构而非行号
3. **容错性强**: 减少因定位错误导致的文件损坏
4. **渐进迁移**: 新工具成熟后，逐步淘汰旧工具
5. **安全第一**: 写入操作必须有安全机制（如dry-run）

## 技术依赖

- 现有的 `ParseFileStructure` 函数
- Go语言AST分析能力
- 现有的工具注册机制

## 风险与缓解

1. **风险**: 结构分析不准确
   **缓解**: 提供详细的错误信息，支持多种定位方式

2. **风险**: 跨语言兼容性
   **缓解**: 先聚焦Go语言，后续扩展

3. **风险**: 工具接口设计不合理
   **缓解**: 小步快跑，每个工具完成后立即测试验证

## 项目状态

✅ **所有代码结构感知工具已全部完成**
✅ **新增git_am工具已完成**
✅ **工具添加文档已完成**

**下一步**: 将此文档内容合并到TODO.md，然后删除TODO_CODE.md文件，保持项目文档整洁。