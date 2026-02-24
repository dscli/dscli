# dscli v0.4 发布说明

## 版本亮点
**DeepSeek-Reasoner 模型支持** - 完整实现 deepseek-reasoner 模型集成，支持模型类型隔离和差异化配置。

## 新特性

### 1. DeepSeek-Reasoner 模型支持
- **模型选择**：通过 `--model` 参数支持 `deepseek-chat` 和 `deepseek-reasoner`
- **历史隔离**：不同模型的对话历史完全独立（通过 `model_id` 字段）
- **功能差异化**：
  - `deepseek-chat`：编程助手，支持完整工具集
  - `deepseek-reasoner`：深度思考者，专注推理分析，显示推理内容
- **系统提示优化**：为不同模型提供专用的系统提示

### 2. 数据库架构升级
- **model_id字段**：在 messages 表中添加 `model_id INTEGER NOT NULL DEFAULT 0`
- **查询过滤**：所有数据库查询按 `session_id` 和 `model_id` 双重过滤
- **向后兼容**：自动升级现有数据库表结构

### 3. 代码质量提升
- **GetSessionID重构**：改进错误处理，延迟初始化，函数重命名为 `CreateOrGetSessionID`
- **GitDiff修复**：修复 `git diff` 命令参数，支持多文件路径
- **测试增强**：添加数据库测试，更新现有测试

## 使用示例

```bash
# 使用聊天模型（支持工具操作）
echo "创建Go项目结构" | dscli chat --model deepseek-chat

# 使用推理模型（专注深度思考）
echo "分析架构设计优缺点" | dscli chat --model deepseek-reasoner
```

## 详细更改

### 功能实现
- `ad3bbd3` feat: 完整实现deepseek-reasoner模型支持
- `9897be8` feat: 支持deepseek-reasoner模型并区分模型类型
- `fb7183f` fix(db): 恢复并完善model_id字段实现

### 代码重构
- `a2c7097` refactor: 重构GetSessionID并修复GitDiff潜在Bug
- `2892902` refactor(chat): 移除未使用的reasoner变量
- `dc2a309` refactor(tools): 修复注释并重命名函数以保持一致性

### 文档和测试
- `06b7aaf` docs: 优化skills.org格式规范
- `6a8ffa8` docs: 合并skills_integration_plan.md到docs/skills.org
- `664d77f` test(skills): 添加Skills系统单元测试
- `8e74b2c` style: 格式化markdown2org_test.go测试文件

### 代码质量
- `a8df692` style: 格式化代码（gofmt）
- `b025872` fix(markdown2org): 修复代码块下划线处理并添加测试
- `f4d085f` refactor(markdown2org): 将零宽度空格字符改为\u200b转义序列

## 技术架构

### 模型隔离设计
```
dscli chat --model deepseek-chat
    ├── 系统提示：编程助手
    ├── 工具配置：完整工具集
    ├── 历史存储：model_id = 0
    └── 输出：操作指导 + 代码

dscli chat --model deepseek-reasoner
    ├── 系统提示：深度思考者
    ├── 工具配置：无工具
    ├── 历史存储：model_id = 1
    └── 输出：推理过程 + 分析结论
```

### 数据库升级
```sql
-- 自动执行的ALTER TABLE
ALTER TABLE messages ADD COLUMN model_id INTEGER NOT NULL DEFAULT 0;

-- 所有查询现在包含model_id过滤
SELECT * FROM messages WHERE session_id = ? AND model_id = ?;
```

## 向后兼容
- 现有用户的数据库会自动升级支持 `model_id` 字段
- 默认模型保持为 `deepseek-chat`，确保现有工作流不受影响
- 所有现有功能完全兼容

## 下一步计划
1. 模型性能优化和缓存机制
2. 更多模型支持扩展
3. 用户配置界面改进

---

**发布日期**: $(date +%Y-%m-%d)
**版本**: v0.4
**提交数量**: 13 (自 v0.3)
**主要贡献**: DeepSeek-Reasoner 模型完整支持
