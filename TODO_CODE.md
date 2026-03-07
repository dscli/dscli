# 代码结构感知工具开发计划

## 项目目标
创建一套基于代码结构而非行号的文件操作工具，使LLM能更安全、更准确地操作代码文件。

## 工具列表

**状态**: ✅ 已完成
**状态**: ⏳ 待开始
**功能**: 读取代码文件的结构信息（函数、类、方法等）
**依赖**: 无
**任务清单**:
- [x] 创建 `code_read_structure.go` 文件
- [x] 实现 `readCodeStructure` 函数
- [x] 设计工具参数和返回值格式
- [x] 注册工具到LLM
- [ ] 编写测试用例
- [ ] 验证工具在对话中可用
- [ ] 注册工具到LLM
- [ ] 编写测试用例
- [ ] 验证工具在对话中可用
**状态**: 🚧 进行中
**接口设计**:
```go
// 读取代码文件的结构信息
func readCodeStructure(path string) (FileStructure, error)
```

### 2. code_read_section.go - 基于结构读取代码片段
**状态**: 🚧 进行中
**功能**: 基于代码结构定位并读取特定代码片段
**依赖**: `code_read_structure.go`
**优先级**: 中

**任务清单**:
- [ ] 创建 `code_read_section.go` 文件
- [ ] 实现 `readCodeSection` 函数
- [ ] 设计代码片段定位语法
- [ ] 注册工具到LLM
- [ ] 编写测试用例
- [ ] 验证工具在对话中可用

**接口设计**:
```go
// 基于结构读取代码片段
func readCodeSection(path string, selector string) (string, error)
```

### 3. code_write_section.go - 基于结构修改代码片段
**状态**: 🚧 进行中
**功能**: 基于代码结构定位并修改特定代码片段
**依赖**: `code_read_structure.go`, `code_read_section.go`
**优先级**: 中

**任务清单**:
- [ ] 创建 `code_write_section.go` 文件
- [ ] 实现 `writeCodeSection` 函数
- [ ] 设计安全写入机制
- [ ] 添加dry-run模式
- [ ] 注册工具到LLM
- [ ] 编写测试用例
- [ ] 验证工具在对话中可用

**接口设计**:
```go
// 基于结构修改代码片段
func writeCodeSection(path string, selector string, newContent string, dryRun bool) (string, error)
```

### 4. code_search_semantic.go - 语义搜索代码
**状态**: 🚧 进行中
**功能**: 基于语义搜索代码中的特定模式
**依赖**: `code_read_structure.go`
**优先级**: 低

**任务清单**:
- [ ] 创建 `code_search_semantic.go` 文件
- [ ] 实现 `searchCodeSemantic` 函数
- [ ] 设计语义搜索算法
- [ ] 注册工具到LLM
- [ ] 编写测试用例
- [ ] 验证工具在对话中可用

**接口设计**:
### 2026-03-08 进度更新
- [x] 创建TODO.md文档
- [x] 完成 `code_read_structure.go` 的基础实现和注册
- [x] 完成 `code_read_section.go`
- [x] 完成 `code_write_section.go`
- [x] 完成 `code_search_semantic.go`

### 2026-03-08 开始项目
- [x] 创建TODO.md文档
- [x] 完成 `code_read_structure.go`
- [x] 完成 `code_read_section.go`


### 2026-03-08 任务完成总结
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
- [x] 完成 `code_search_semantic.go`
- [ ] 完成 `code_write_section.go`
- [ ] 完成 `code_search_semantic.go`

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