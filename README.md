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

1. **AI 编程助手** - 基于 DeepSeek API 的智能对话和代码补全
2. **开发工具** - 文件操作等实用功能
3. **集成环境** - 支持 Emacs 集成，提供流畅的开发体验

简单说：dscli = AI助手 + 开发工具 + 命令行效率

## 📦 版本信息

**当前版本：v0.5.4**

### 版本历史
- v0.5.4 (2026-03-09) - 新增余额显示和低余额提醒功能，增强用户体验
- v0.5.3 (2026-03-06) - 添加段落系统，统一提示词管理
- v0.5.2 (2026-03-05) - 重构工具实现，删除重复代码，拆分专门工具文件
- v0.5.1 (2026-02-28) - 版本更新发布
- v0.5.0 (2026-02-28) - 功能完备版本，包含43个迭代
- v0.4.0 - 格式化系统重构，支持多种输出模式
- v0.3.0 - 添加Git issue管理功能
- v0.2.0 - 增强AI工具调用能力
- v0.1.0 - 初始版本发布
## ✨ 核心功能

### 🤖 AI 功能
- **`dscli chat`** - 与 DeepSeek AI 对话，支持工具调用（文件读写、Git操作等）
- **`dscli fim`** - 代码补全功能，提升编码效率
- **`dscli models`** - 查看可用的 AI 模型
- **`dscli balance`** - 查看 API 使用情况和余额

### 🛠️ 实用特性
- **多格式输出** - 支持 `--mode markdown`（默认）和 `--mode org` 输出格式
- **数据库支持** - SQLite 存储对话历史、配置等
- **项目感知** - 自动识别 Git 仓库根目录，按项目隔离对话历史
- **`dscli version`** - 查看版本信息

## 🚀 快速开始

### 安装
### 安装
```bash
# 方式1：使用 go install（推荐）
go install gitcode.com/dscli/dscli@v0.5.4

# 方式2：从源码构建
git clone https://gitcode.com/dscli/dscli.git
cd dscli
git checkout v0.5.4
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
# 基本对话（Markdown格式输出）
echo "如何用Go实现HTTP服务器？" | dscli chat

# Org模式输出
echo "解释这个算法的时间复杂度" | dscli chat --mode org

# 使用推理模型
echo "分析这个代码的性能问题" | dscli chat --model deepseek-reasoner

# 代码补全
echo "def fibonacci(n):" | dscli fim
```

### 2. 查看模型和余额
```bash
# 查看可用模型
dscli models

# 查看账户余额
dscli balance

# JSON格式输出
dscli models --format json
dscli balance --format json
```

### 3. 查看版本信息
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
- 环境文件：`~/.dscli/dscli.env`
- 日志文件：`~/.dscli/dscli.log`
- 数据库：`~/.dscli/sqlite.db`

## 🏗️ 项目结构

```
dscli/
├── main.go              # 主入口
├── version.go           # 版本信息
├── chat.go              # AI 对话功能
├── fim.go               # 代码补全
├── models.go            # 模型管理
├── balance.go           # 余额查询
├── db.go                # 数据库操作
├── tools.go             # 工具调用
├── prompt.go            # 系统提示词
├── formatter.go         # 格式化器
├── fmt.go               # 格式化工具
├── markdown2org.go      # Markdown到Org转换器
└── client.go            # API客户端
```

## 🔄 工作流程

1. **项目感知** - 自动识别 Git 仓库根目录
2. **上下文隔离** - 每个项目有独立的对话历史
3. **工具集成** - AI 可以直接操作文件、执行 Git 命令
4. **多格式输出** - 支持 Markdown 和 Org 模式输出

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

**dscli v0.5.4** - 让命令行开发更智能、更高效！
**dscli v0.5.4** - 让命令行开发更智能、更高效！