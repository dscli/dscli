# dscli - AI增强的开发者工具箱

```
      o
     /|\
      |   +----------------------------+
     / \  | * dscli.el - Emacs集成    |
~~~~~~~~~~| $ dscli chat - AI助手     |
dscli     | > dscli issue - Git管理   |
          +----------------------------+
```

## 🎯 什么是 dscli？

**dscli** 是一个 AI 增强的命令行开发者工具箱，它结合了：

1. **AI 编程助手** - 基于 DeepSeek API 的智能对话和代码补全
2. **开发工具** - Git issue 管理、技能管理、文件操作等实用功能
3. **集成环境** - 支持 Emacs 集成，提供流畅的开发体验

简单说：dscli = AI助手 + 开发工具 + 命令行效率

## ✨ 核心功能

### 🤖 AI 功能
- **`dscli chat`** - 与 DeepSeek AI 对话，支持工具调用（文件读写、Git操作等）
- **`dscli fim`** - 代码补全功能，提升编码效率
- **`dscli models`** - 查看可用的 AI 模型
- **`dscli balance`** - 查看 API 使用情况和余额

### 🔧 开发工具
- **`dscli issue`** - Git 仓库 issue 管理
  - `list` - 列出 issue（支持 open/closed/all 状态）
  - `show <number>` - 查看 issue 详情
  - `create` - 创建新 issue
  - `update` - 更新 issue
- **技能管理** - 保存和复用常用 prompt，提高工作效率
- **对话历史** - 基于项目的对话历史存储，支持上下文记忆

### 🛠️ 实用工具
- **`markdown2org`** - Markdown 转 Org 格式
- **数据库支持** - SQLite 存储对话历史、技能、配置等
- **Emacs 集成** - 通过 dscli.el 在 Emacs 中使用所有功能

## 🚀 快速开始

### 安装
```bash
# 方式1：使用 go install
go install gitcode.com/dscli/dscli@latest

# 方式2：从源码构建
git clone https://gitcode.com/dscli/dscli.git
cd dscli
make install    # 安装到 $GOPATH/bin
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
# 基本对话
echo "如何用Go实现HTTP服务器？" | dscli chat

# 使用推理模型
echo "解释这个算法的时间复杂度" | dscli chat --model deepseek-reasoner

# 代码补全
echo "def fibonacci(n):" | dscli fim
```

### 2. Git Issue 管理
```bash
# 列出所有打开的 issue
dscli issue list

# 查看特定 issue 详情
dscli issue show 123

# 列出已关闭的 issue
dscli issue list --state closed
```

### 3. 技能管理
```bash
# 通过 chat 命令使用技能
echo "使用代码审查技能" | dscli chat
```

## 🔌 Emacs 集成

### 安装 dscli.el
```emacs-lisp
(add-to-list 'load-path "/path/to/dscli.el")
(require 'dscli)
```

### 基本使用
- `M-x dscli-chat` - 启动 AI 对话
- 在临时缓冲区输入问题
- 按 `C-c C-c` 发送
- 查看 org mode 格式的回答

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
- 环境文件：`~/.dscli/dscli.env`
- 日志文件：`~/.dscli/dscli.log`
- 数据库：`~/.dscli/sqlite.db`

## 🏗️ 项目结构

```
dscli/
├── main.go              # 主入口
├── chat.go              # AI 对话功能
├── issue.go             # Git issue 管理
├── fim.go               # 代码补全
├── models.go            # 模型管理
├── balance.go           # 余额查询
├── db.go                # 数据库操作
├── tools.go             # 工具调用
├── prompt.go            # 系统提示词
└── markdown2org.go      # 格式转换
```

## 🔄 工作流程

1. **项目感知** - 自动识别 Git 仓库根目录
2. **上下文隔离** - 每个项目有独立的对话历史
3. **工具集成** - AI 可以直接操作文件、执行 Git 命令
4. **技能复用** - 保存常用 prompt，提高效率

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

**dscli** - 让命令行开发更智能、更高效！