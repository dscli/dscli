# dscli - AI增强的开发者工具箱

```
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

### 🖥️ TUI 终端界面
- **`dscli tui`** — 启动交互式终端用户界面（Terminal UI），在同一界面中访问所有功能

TUI 提供以下功能面板：
  - 🏠 **Dashboard** — 主菜单导航，Logo 展示
  - 💰 **Balance** — 查看账户余额和可用状态
  - 🤖 **Models** — 浏览可用 AI 模型列表
  - 📜 **History** — 查看会话历史消息，支持查看详情
  - 🔧 **Skills** — 浏览已安装的技能列表
  - 📝 **Prompt** — 查看系统提示词内容
  - 💬 **Chat** — 与 DeepSeek 实时对话，支持推理内容、工具调用展示

TUI 操作方式：
  - `j/k` 或 `↑/↓` — 导航
  - `Enter` — 选择/确认
  - `i` — 聚焦输入框（Chat 模式）
  - `q/esc` — 返回上级
  - `Ctrl+C` — 退出

启动方式：
```bash
# 默认启动（使用聊天模型）
dscli tui

# 使用推理模型
dscli tui --model deepseek-reasoner

# 指定加载的历史消息数量
dscli tui --histsize 16
```
### 📝 会话管理
- **`dscli history`** — 对话历史管理（list / load / show / edit / update）
- **`dscli history recall <关键词>`** — 搜索历史消息，回忆过往讨论

### 🛠️ 开发工具
- **`dscli flycheck <路径>`** — 静态代码检查（Go 用 staticcheck，Python 用 ruff）
- **`dscli skill`** — 技能管理（list / use / query / set-auto-inject）
- **`dscli prompt`** — 系统提示词管理（show / edit，支持项目级和全局）
- **`dscli config edit`** — 编辑配置文件

### 💬 微信集成
- **`dscli wechat`** — 微信 AI 工具接口（登录、收发消息、好友/群组管理）

### 🎨 通用特性
- **多格式输出** — 支持 `--mode markdown`（默认）和 `--mode org` 输出格式
- **数据库支持** — SQLite 存储对话历史、配置、笔记等
- **项目感知** — 自动识别 Git 仓库根目录，按项目隔离对话历史
- **会话统计** — 每次对话后显示耗时、花费、余额
- **`dscli version`** — 查看版本和运行时信息

## 🚀 快速开始

### 安装
```bash
# 方式1：使用 go install（推荐）
go install gitcode.com/dscli/dscli@latest

# 方式2：从源码构建
git clone https://gitcode.com/dscli/dscli.git
cd dscli
git checkout v0.7.6
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

# 使用推理模型
echo "分析这个代码的性能问题" | dscli chat --model deepseek-reasoner

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

### 3. 开发工具
```bash
# 静态代码检查
dscli flycheck internal/...

# 技能管理
dscli skill list
dscli skill query "go fix"

# 编辑系统提示词
dscli prompt edit
```

### 4. 查看模型和余额
```bash
# 查看可用模型
dscli models

# 查看账户余额
dscli balance

# JSON 格式输出
dscli models --format json
dscli balance --format json
```

### 5. TUI 终端界面
```bash
# 启动交互式终端界面
dscli tui

# 使用推理模型启动
dscli tui --model deepseek-reasoner

# 加载更多历史消息
dscli tui --histsize 32
```

### 6. 查看版本信息
```bash
dscli version
```
## ⚙️ 高级配置

### 环境变量
```bash
# 必需：API 密钥
export DEEPSEEK_API_KEY="your-api-key"

# 可选：API 地址（默认 https://api.deepseek.com）
export DEEPSEEK_BASE_URL="https://api.deepseek.com"

# 可选：模型配置
export MODEL_DEEPSEEK_CHAT="deepseek-chat"
export MODEL_DEEPSEEK_REASONER="deepseek-reasoner"
```
### 配置文件
- 配置目录：`~/.dscli/`
- 配置文件：`~/.dscli/config.dscli`
- 数据库：`~/.dscli/sqlite.db`
- 全局提示词：`~/.dscli/prompt/chat.md`、`~/.dscli/prompt/reasoner.md`
- 全局技能：`~/.dscli/skills/`

### 项目级配置
- 项目配置目录：`<project>/.dscli/`
- 项目提示词：`<project>/.dscli/prompt/chat.md`
- 项目技能：`<project>/.dscli/skills/`
- 优先级：项目配置 > 全局配置 > 内置默认

## 🏗️ 项目结构

dscli/
├── main.go                    # 主入口
├── root.go                    # 根命令与初始化
├── version.go                 # 版本信息
├── chat.go                    # AI 对话（chat 命令）
├── fim.go                     # 代码补全（fim 命令）
├── models.go                  # 模型查询（models 命令）
├── balance.go                 # 余额查询（balance 命令）
├── history.go                 # 历史管理（history 命令 + recall 子命令）
├── flycheck.go                # 静态检查（flycheck 命令）
├── skill.go                   # 技能管理（skill 命令 + add/remove 子命令）
├── skill_add.go               # 技能添加子命令
├── skill_remove.go            # 技能移除子命令
├── prompt.go                  # 提示词管理（prompt 命令）
├── config_edit.go             # 配置编辑（config 命令）
├── wechat_cmd.go              # 微信集成（wechat 命令）
├── tui.go                     # TUI 终端界面（tui 命令）
├── parse.go                   # 解析工具
├── formatter.go               # 格式化器
├── Makefile                   # 构建脚本
└── internal/
    ├── config/                 # 配置管理（配置文件读写）
    ├── context/                # 上下文管理（Context key/value）
    ├── dsc/                    # DeepSeek API 客户端（Chat/FIM/Balance/Models）
    ├── editor/                 # 外部编辑器调用
    ├── flycheck/               # 静态检查引擎（Go/Python）
    ├── memories/               # 持久记忆存储（FTS5 全文搜索）
    ├── outfmt/                 # 输出格式化（Markdown→Org、表格、等待动画）
    ├── parse/                  # 输入解析
    ├── prompt/                 # 系统提示词渲染、消息模型、笔记
    ├── session/                # 会话管理
    ├── shell/                  # Shell 命令执行与校验
    ├── skills/                 # 技能系统（本地/全局）
    ├── sqlite/                 # SQLite 数据库操作
    ├── tokenizer/              # 中文分词器（gse + 停词表 + CJK 过滤）
    │   └── stopwords/          # 嵌入式停词表（cn/hit/scu）
    ├── toolcall/               # 工具调用框架
    │   ├── alltools/           # 工具注册入口
    │   ├── ask/                # ask_expert / ask_user / code_review
    │   ├── code/               # 代码读写/搜索/结构分析
    │   ├── file/               # 文件读写/搜索
    │   ├── flycheck/           # flycheck 工具
    │   ├── history/            # note 工具（对话笔记）
    │   ├── issue/              # Git issue 管理
    │   ├── memory/             # 记忆工具（mem_search / mem_save 等）
    │   ├── recall/             # recall 工具（历史消息搜索）
    │   ├── shell/              # Shell 执行工具
    │   ├── skill/              # skill 工具
    │   └── web/                # Web 内容获取（web_reader）
    ├── tui/                    # TUI 界面模型（bubbletea）
    └── wechat/                 # 微信客户端（登录/消息/好友/群组）
## 🔄 工作流程


1. **项目感知** — 自动识别 Git 仓库根目录，确定项目上下文
2. **系统提示词** — 加载项目/全局/默认三级提示词，注入技能和笔记
3. **上下文隔离** — 每个项目有独立的会话和对话历史
4. **工具集成** — AI 可直接操作文件、搜索代码、执行 Git/Shell 命令、管理 Issue
5. **会话统计** — 对话结束后显示耗时和余额消耗

## 🤝 贡献

欢迎贡献代码、报告问题或提出建议！
1. Fork 项目
2. 创建功能分支
3. 提交更改
4. 推送到分支
5. 创建 Pull Request

## 📄 许可证

Apache License 2.0

## 📞 支持

- 项目地址：[gitcode.com/dscli/dscli](https://gitcode.com/dscli/dscli)
- 问题反馈：[创建 Issue](https://gitcode.com/dscli/dscli/issues)

---

**dscli** — 让命令行开发更智能、更高效！