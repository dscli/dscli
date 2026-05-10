---
name: dscli
description: dscli 内置技能，介绍 dscli 特色功能：自定义 prompt、skill 自优化、角色工具配置、历史回顾、笔记、记忆等。
keywords: [dscli, prompt, skill, role, note, recall, memory]
auto_inject: true
---

# dscli 特色功能

dscli 是一个强大的 AI 编程助手 CLI 工具，以下是其核心特色功能指南。

## 1. 自定义 Prompt（绑定角色）

每个角色（dev/expert/review）都支持自定义系统提示词：

- **内建模板**: `dev.md` / `expert.md` / `review.md` 提供默认提示词
- **全局覆盖**: 在 `~/.dscli/prompt/<role>.md` 创建自定义提示词
- **项目覆盖**: 在 `<project>/.dscli/prompt/<role>.md` 创建项目级提示词
- **优先级**: 项目 > 全局 > 内嵌

角色通过 SQLite 数据库 (`role_configs` 表) 绑定 prompt。可以使用 `dscli role` 命令为角色指定不同的 prompt 模板。

## 2. Skill 自优化（auto_inject）

Skills 是 dscli 的知识注入机制，支持**自动嵌入上下文**（auto_inject）：

- 在 skill 的 YAML frontmatter 中设置 `auto_inject: true`
- 设置了 auto_inject 的 skill 会在每次对话中**自动注入完整内容**到系统提示词
- 无需 LLM 手动调用 `skill_by_name`，不消耗额外的 tool call
- 可通过 `skill_set_auto_inject` 工具**动态切换** auto_inject 状态

使用场景：编写指南、编码规范、项目约定等需要始终生效的知识。

## 3. 角色可自配置工具

每个角色可独立控制**可用工具集**（tools）和**可用技能**（skills）：

```
-- role_configs 表结构：
role TEXT, skills TEXT, tools TEXT, prompt TEXT, session_id INTEGER
```

- `skills: "all"` — 所有技能可用
- `skills: ""` — 无技能
- `skills: "go-fix,gofumpt"` — 仅指定技能可用
- `tools` 同理控制工具过滤

使用 `dscli role` 命令管理角色配置。角色修改即时生效，无需重启。

## 4. 历史回顾（recall）和笔记（note）

dscli 记录所有对话历史并提供强大的回顾能力：

### recall — 历史回顾
- 基于 **FTS5 全文搜索** 检索历史对话
- 支持中文分词，按相关性排序
- 仅在当前 session 内搜索，保证隐私
- 使用方法：`recall(keywords="关键词", days=30, limit=5)`

### note — 对话笔记
- 在对话结束时调用 `note(content="摘要")` 保存关键信息（≤40字）
- 笔记会在**下次对话的系统提示词**中自动注入，帮 LLM 回忆上下文
- 例如：`note(content="修复了 shell stderr 显示问题")`

## 5. 记忆系统（mem_save）

持久化记忆系统，用于跨对话保留重要发现和决策：

| 工具 | 用途 |
|------|------|
| `mem_save` | 保存新记忆（标题 + 内容 + 类型） |
| `mem_search` | FTS5 全文搜索记忆 |
| `mem_update` | 更新已有记忆 |
| `mem_delete` | 删除记忆（不可逆！） |
| `mem_get_observation` | 查看记忆完整内容 |
| `mem_stats` | 记忆统计信息 |

记忆类型（type）建议：
- `decision` — 架构决策
- `bugfix` — Bug 修复记录
- `pattern` — 代码模式/惯例
- `config` — 配置信息
- `discovery` — 技术发现
- `learning` — 经验教训

### 最佳实践
1. **记录决策**: 每次做出重要技术决策时保存
2. **记录 Bug**: 修复后保存原因和方案供未来参考
3. **记录模式**: 发现重复的代码模式时保存
4. **定期搜索**: 在新问题前搜索记忆，避免重复犯错
