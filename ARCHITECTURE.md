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
    Version = "0.5.2"
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

*最后更新：2026年03月03日*
*维护者：dscli 开发团队*