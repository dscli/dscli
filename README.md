# dscli - AI增强的开发者工具箱

```text
     o
    /|\
     |   +---------------+
    / \  | dscli tools   |
 ~~~~~~~~~| AI assistant  |
dscli    +---------------+
```

## 🎯 什么是 dscli？

**dscli** 是一个基于 DeepSeek API 的智能命令行工具，集 AI 编程助手、代码检查、项目管理于一体。

1. **AI 编程助手** — 深度集成 DeepSeek，支持工具调用的多轮对话
2. **开发工具箱** — 文件操作、代码搜索、Git 管理、静态检查、Shell 执行
3. **会话记忆** — 项目级对话历史、笔记系统、跨对话回忆
4. **可定制化** — 自定义系统提示词、技能系统、多格式输出

简单说：**dscli = AI 助手 + 开发工具 + 会话记忆 + 命令行效率**

## 📦 版本信息

### 版本历史

- v0.8.0 (2026-05-17) — AI 人物系统（32 位科学家）、技能 author 字段自动填充、输出格式统一、`git author` 风格用户展示
- v0.7.6 (2026-05-03) — P0 nil panic 修复、类型别名清理、recall 限制、11 个新测试
- v0.7.5 (2026-05-03) — toolcall 结果截断阈值提升至 1M 上下文适配
- v0.7.4 (2026-04-29) — 重组包结构，整合 prompt / note / session
- v0.7.3 (2026-04-15) — recall 工具支持关键词搜索历史消息
- v0.7.2 (2026-04-10) — note 工具支持跨对话记忆
- v0.7.1 (2026-03-16) — 重构测试，性能从 27 秒提升到 6 秒（4.2 倍）
- v0.7.0 (2026-03-16) — 集成自动代码格式修正工具链，重构 shell 命令判断逻辑，添加超时控制
- v0.6.0 (2026-03-13) — 合并 vimscript 分支，添加 vimscript 语言支持，优化 web reader
- v0.5.5 (2026-03-12) — 修复 modernize 工具引入的问题，优化代码结构
- v0.5.4 (2026-03-09) — 添加 AskExpert 函数，改进 AI 助手交互体验
- v0.5.2 (2026-03-08) — 重构代码结构，分离关注点，提高可维护性
- v0.5.0 (2026-02-28) — 功能完备版本，包含 43 个迭代
- v0.4.0 — 格式化系统重构，支持多种输出模式
- v0.3.0 — 添加 Git issue 管理功能
- v0.2.0 — 增强 AI 工具调用能力
- v0.1.0 — 初始版本发布

## ✨ 核心功能

### 🤖 AI 对话

- **`dscli chat`** — 与 DeepSeek AI 多轮对话，支持工具调用（文件读写、代码搜索、Git 操作等）
- **`dscli fim`** — 代码补全（Fill-in-the-Middle），提升编码效率
- **`dscli models`** — 查看可用的 AI 模型列表
- **`dscli balance`** — 查询 API 余额和使用情况

### 📝 会话管理

- **`dscli history`** — 对话历史管理（list / load / show / edit / update）
- **`dscli history recall <关键词>`** — 搜索历史消息，回忆过往讨论

### 🛠️ 开发工具

- **`dscli flycheck <路径>`** — 静态代码检查（Go 用 staticcheck，Python 用 ruff）
- **`dscli skill`** — 技能管理（list / show / add / remove / query / validate / set-auto-inject / save；含 YAML frontmatter author 自动填充）
- **`dscli prompt`** — 系统提示词管理（show / edit，支持项目级和全局）
- **`dscli completion`** — 生成 Shell 自动补全脚本（bash / zsh / fish / powershell）
- **`dscli config edit`** — 编辑配置文件

### 💬 微信集成

- **`dscli wechat`** — 微信 AI 工具接口（登录、收发消息、好友/群组管理）

### 🎨 通用特性

- **多格式输出** — 支持 `--mode markdown`（默认）和 `--mode org` 输出格式
- **数据库支持** — SQLite 存储对话历史、配置、笔记等
- **项目感知** — 自动识别 Git 仓库根目录，按项目隔离对话历史
- **会话统计** — 每次对话后显示耗时、花费、余额
- **`dscli version`** — 查看版本和运行时信息

### 🎭 AI 人物

32 位科学家人格随机分配，附带性格与邮箱。

- **随机分配** — 首次使用随机抽取，持久绑定
- **人格注入** — 性格描述自动注入系统提示词

## 🚀 快速开始

### 安装

```bash
# 方式1：使用 go install（推荐）
go install gitcode.com/dscli/dscli@latest

# 方式2：从源码构建
git clone https://gitcode.com/dscli/dscli.git
cd dscli
git checkout v0.8.0
make install    # 安装到 $GOPATH/bin

# 方式3：下载预编译二进制
# 查看 Releases 页面获取最新版本
```

### 配置

1. 获取 DeepSeek API 密钥：[DeepSeek 平台](https://platform.deepseek.com/)
2. 设置环境变量：

```bash
export DEEPSEEK_API_KEY="your-api-key-here"
```

## 📖 使用示例

### 1. AI 编程助手

```bash
# 基本对话（Markdown 格式输出）
echo "如何用 Go 实现 HTTP 服务器？" | dscli chat

# Org 模式输出
echo "解释这个算法的时间复杂度" | dscli chat --mode org

# 代码补全
echo "def fibonacci(n):" | dscli fim
```

### 2. 会话管理

```bash
# 查看对话历史列表
dscli history list

# 搜索历史消息
dscli history recall "Go 错误处理"

# 查看指定消息详情
dscli history show 42

# 编辑消息内容
dscli history edit 42
```

### 3. 技能管理

```bash
# 列出所有技能
dscli skill list

# 搜索技能
dscli skill query "go fix"

# 查看技能详情
dscli skill show go-fix

# 校验技能
dscli skill validate go-fix

# 安装技能
dscli skill add ~/src/agent-skills/skills/go-fix
dscli skill add ~/src/agent-skills/skills/go-fix --target=global

# 移除技能
dscli skill remove go-fix

# 设置自动注入
dscli skill set-auto-inject go-fix true

# 创建/更新技能（author 自动从 git config 填充）
dscli skill save --name my-skill --content "..." --desc "说明"
```

### 4. 记忆管理

```bash
# 列出当前项目的所有记忆
dscli memory list

# 搜索记忆
dscli memory search "flycheck 超时"

# 查看记忆完整内容
dscli memory show 1

# 记忆统计
dscli memory stats
```

### 5. 角色定制

dscli 内置三个 AI 角色：**dev**（开发助手，全工具/全技能）、
**expert**（领域专家，无工具/无技能）、**review**（代码审查，
shell+file_read/无技能）。每个角色可独立配置系统提示词、可用工具
和技能列表。

**浏览工具：**

```bash
# 列出所有可用工具（按分类展示）
dscli tool list

# 按分类筛选
dscli tool list --category file
```

**管理提示词：**

```bash
# 列出所有提示词
dscli prompt list

# 查看提示词内容
dscli prompt show review

# 基于 review 添加新的提示词 editor
dscli prompt show review | dscli prompt add editor

# 编辑提示词
dscli prompt edit editor
```

**配置角色：**

```bash
# 查看当前角色配置
dscli role list
dscli role show dev

dscli role update review --skills "go-fix,gofumpt" \
    --tools "shell,file_read" --prompt editor

# 恢复默认配置
dscli role reset review
```

### 6. 开发工具

```bash
# 静态代码检查
dscli flycheck internal/...

# Emacs flycheck（支持 119+ 语言）
dscli flycheck --emacs internal/

# 解析文件结构（供 LLM 编辑）
dscli parse main.go
dscli parse main.go -l python
```

### 7. 查看模型和余额

```bash
# 查看可用模型
dscli models

# 查看账户余额
dscli balance

# JSON 格式输出
dscli models --format json
dscli balance --format json
```

### 8. 配置文件

配置文件默认为 `~/.dscli/config.dscli`，首次运行时通过环境变量自动生成：

```bash
# 行首注释
deepseek-api-key = sk-xxx          # 行末注释
deepseek-base-url = https://api.deepseek.com
```

格式规则：

- 每行一个 `key = value` 配置项
- `#` 支持行首和行末注释

常用配置项：

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `deepseek-api-key` | | API 密钥 |
| `context-window` | `1000000` | 上下文窗口大小（token） |
| `max-tokens` | `393216` | 单次最大输出 token |
| `user-balance` | `true` | 对话结束后显示余额消耗 |
| `deepseek-v4` | `true` | 启用 V4 模型 |

## 🔄 工作流程

1. **项目感知** — 自动识别 Git 仓库根目录，确定项目上下文
2. **系统提示词** — 加载项目/全局/默认三级提示词，注入技能和笔记
3. **上下文隔离** — 每个项目有独立的会话和对话历史
4. **工具集成** — AI 可直接操作文件、搜索代码、执行 Git/Shell 命令、管理 Issue
5. **会话统计** — 对话结束后显示耗时和余额消耗

## 🤝 贡献

欢迎贡献代码、报告问题或提出建议！详细规范请参阅 [CONTRIBUTING.md](CONTRIBUTING.md)。

Apache License 2.0

## 📞 支持

- 项目地址：[gitcode.com/dscli/dscli](https://gitcode.com/dscli/dscli)
- 问题反馈：[创建 Issue](https://gitcode.com/dscli/dscli/issues)

---

**dscli** — 让命令行开发更智能、更高效！
