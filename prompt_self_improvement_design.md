# AI自我提示词改进系统设计

## 设计目标
让AI能够安全、可控地优化自己的行为指南，实现渐进式自我改进。

## 核心原则
1. **安全性第一**：不能破坏核心约束和权限边界
2. **渐进式改进**：小步快跑，每次只修改一小部分
3. **可追溯性**：所有修改都有记录和理由
4. **可回滚**：任何时候都可以恢复到之前版本

## 系统架构

### 1. 数据层扩展
```sql
-- 现有表结构
CREATE TABLE prompt_segments (
    id INTEGER PRIMARY KEY,
    domain_id INTEGER,
    model_id INTEGER DEFAULT -1,
    name TEXT NOT NULL,        -- 分段名称
    content TEXT NOT NULL,     -- 模板内容
    sort_order INTEGER DEFAULT 0,
    enabled BOOLEAN DEFAULT true,
    editable BOOLEAN DEFAULT false,  -- 新增：是否可编辑
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);

-- 新增：提示词版本表
CREATE TABLE prompt_versions (
    id INTEGER PRIMARY KEY,
    segment_id INTEGER,
    old_content TEXT,
    new_content TEXT,
    diff TEXT,                 -- 差异对比
    modified_by TEXT,          -- 'ai' 或 'human'
    reason TEXT,               -- 修改理由
    status TEXT,               -- 'pending', 'approved', 'rejected', 'applied'
    reviewed_by TEXT,          -- 审核人
    reviewed_at TIMESTAMP,
    created_at TIMESTAMP
);
```

### 2. 工具层设计

#### 工具1：`prompt_analyze`（分析工具）
**功能**：分析当前提示词，识别潜在问题
**输出**：分析报告，不实际修改
**风险等级**：低

```go
// 分析维度：
1. 完整性检查：是否包含所有必要部分
2. 一致性检查：各部分之间是否协调
3. 有效性检查：是否有模糊或矛盾的描述
4. 改进建议：具体的优化方向
```

#### 工具2：`prompt_suggest`（建议工具）
**功能**：生成具体的修改方案
**输出**：修改补丁（diff格式）
**风险等级**：中

```go
// 输入：分析报告 + 修改目标
// 输出：{
//   "segment_id": 1,
//   "old_content": "...",
//   "new_content": "...",
//   "diff": "...",
//   "reason": "修改理由",
//   "expected_effect": "预期效果"
// }
```

#### 工具3：`prompt_update`（更新工具）
**功能**：应用经过验证的修改
**限制**：只能修改 `editable=true` 的分段
**风险等级**：高

```go
// 工作流程：
1. 检查修改权限（editable字段）
2. 验证修改安全性（关键词黑名单）
3. 创建版本记录（status='pending'）
4. 等待用户审核或自动验证
5. 应用修改（status='applied'）
```

### 3. 安全机制

#### 保护层1：**编辑权限控制**
```sql
-- 初始设置：只有非核心部分可编辑
UPDATE prompt_segments SET editable = true 
WHERE name IN ('工作流程', '质量要求', '执行指导');
-- 核心身份和权限边界不可编辑
UPDATE prompt_segments SET editable = false 
WHERE name IN ('核心身份', '注意事项', '权限边界');
```

#### 保护层2：**关键词保护**
```go
var protectedKeywords = []string{
    "版权", "所有者", "Nan Jun Jie",
    "权限边界", "不能删除",
    "sqlite.db", "dscli.env",
    "人类所有", "工具优先",
}
```

#### 保护层3：**修改限制**
- 每次最多修改一个分段
- 每次修改不超过原内容的20%
- 不能删除保护性内容
- 需要提供详细的修改理由

### 4. 审核流程

#### 自动审核（适用于低风险修改）
```go
// 自动通过条件：
1. 只修改了可编辑分段
2. 不涉及保护关键词
3. 修改比例小于10%
4. 有合理的修改理由
```

#### 人工审核（适用于高风险修改）
```go
// 需要人工审核的情况：
1. 修改了核心身份描述
2. 涉及权限或安全相关内容
3. 修改比例超过20%
4. 自动审核不通过
```

## 实施路线图

### 阶段1：分析能力（1-2周）
- 实现 `prompt_analyze` 工具
- 添加基本的分析规则
- 输出格式化分析报告

### 阶段2：建议能力（2-3周）
- 实现 `prompt_suggest` 工具
- 添加模板变量支持
- 实现diff生成功能

### 阶段3：受限修改（3-4周）
- 扩展数据库表结构
- 实现 `prompt_update` 工具
- 添加安全验证机制

### 阶段4：完整流程（4-6周）
- 实现审核流程
- 添加版本管理
- 完善错误处理和回滚

## 预期收益

### 短期收益
1. AI能够识别提示词中的问题
2. 提供具体的改进建议
3. 减少人工维护成本

### 长期收益
1. AI能够自适应不同项目需求
2. 通过使用反馈持续优化
3. 实现真正的"元认知"能力

## 风险评估与缓解

### 风险1：AI破坏性修改
**缓解**：多层保护机制 + 版本回滚

### 风险2：修改导致行为异常
**缓解**：小范围修改 + A/B测试思想

### 风险3：审核流程复杂
**缓解**：分级审核 + 自动验证

## 结论

AI自我提示词改进是一个有挑战但价值很高的特性。通过分阶段实施、多层安全保护和渐进式改进，可以在保证系统安全的前提下，逐步实现AI的自我优化能力。

建议从最简单的 `prompt_analyze` 工具开始，积累经验后再逐步推进到更复杂的功能。