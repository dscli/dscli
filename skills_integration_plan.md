# Skills系统接入方案

## 1. 系统架构概述

Skills系统是一个基于规则的知识库，用于存储和复用最佳实践。系统通过以下方式工作：

```
用户请求 → 系统分析 → 匹配相关技能 → 应用技能规则 → 生成响应
```

## 2. 表结构详解

### 2.1 skills表（技能定义）
| 字段 | 类型 | 说明 |
|------|------|------|
| id | INTEGER | 主键，自增 |
| name | TEXT | 技能名称（唯一） |
| description | TEXT | 技能描述 |
| content | TEXT | 技能内容/规则（核心） |
| category | TEXT | 分类（go, git, markdown, test等） |
| priority | INTEGER | 优先级（1-100，默认50） |
| is_global | BOOLEAN | 是否全局技能（0=项目特定，1=全局） |
| usage_count | INTEGER | 使用次数统计 |
| created_at | DATETIME | 创建时间 |
| updated_at | DATETIME | 更新时间 |

### 2.2 project_skills表（项目技能关联）
| 字段 | 类型 | 说明 |
|------|------|------|
| project_path | TEXT | 项目路径（复合主键） |
| skill_id | INTEGER | 技能ID（外键） |
| is_enabled | BOOLEAN | 是否启用（默认1） |
| enabled_at | DATETIME | 启用时间 |
| last_used | DATETIME | 最后使用时间 |

## 3. 技能内容格式

技能内容（content字段）应采用结构化格式，例如：

```yaml
# 技能：Go测试规范
trigger:
  - "test"
  - "测试"
  - "go test"
  - "单元测试"

rules:
  - "测试文件应以_test.go结尾"
  - "测试函数名应以Test开头"
  - "使用t.Run进行子测试"
  - "表格驱动测试优先"
  - "测试覆盖率应达到80%以上"

examples:
  - |
    // 好的测试示例
    func TestAdd(t *testing.T) {
        tests := []struct {
            a, b int
            want int
        }{
            {1, 2, 3},
            {0, 0, 0},
            {-1, 1, 0},
        }
        
        for _, tt := range tests {
            t.Run(fmt.Sprintf("%d+%d", tt.a, tt.b), func(t *testing.T) {
                got := Add(tt.a, tt.b)
                if got != tt.want {
                    t.Errorf("Add(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
                }
            })
        }
    }
```

## 4. 接入实现方案

### 4.1 技能匹配引擎
```go
// SkillMatcher 技能匹配器
type SkillMatcher struct {
    db *sql.DB
}

// MatchSkills 匹配相关技能
func (sm *SkillMatcher) MatchSkills(projectPath, userQuery string) ([]Skill, error) {
    // 1. 获取项目启用的技能
    // 2. 分析用户查询，提取关键词
    // 3. 匹配技能触发条件
    // 4. 按优先级排序返回
}

// ApplySkills 应用技能规则
func (sm *SkillMatcher) ApplySkills(skills []Skill, context Context) string {
    // 1. 合并所有相关技能的内容
    // 2. 根据上下文调整规则
    // 3. 生成指导性文本
}
```

### 4.2 技能管理接口
```go
// SkillManager 技能管理器
type SkillManager interface {
    CreateSkill(skill Skill) error
    UpdateSkill(id int, skill Skill) error
    DeleteSkill(id int) error
    EnableSkill(projectPath string, skillID int) error
    DisableSkill(projectPath string, skillID int) error
    ListSkills(projectPath string, category string) ([]Skill, error)
    SearchSkills(keyword string) ([]Skill, error)
}
```

### 4.3 技能自动学习
```go
// SkillLearner 技能学习器
type SkillLearner struct {
    db *sql.DB
}

// LearnFromInteraction 从交互中学习
func (sl *SkillLearner) LearnFromInteraction(sessionID int, userQuery, assistantResponse string) {
    // 1. 分析成功的交互
    // 2. 提取可复用的模式
    // 3. 创建或更新技能
    // 4. 关联到相关项目
}
```

## 5. 初始技能建议

### 5.1 Go开发技能
```sql
INSERT INTO skills (name, description, content, category, priority, is_global) VALUES
('Go测试规范', 'Go语言测试最佳实践', '{"trigger":["test","测试"],"rules":["测试文件以_test.go结尾","测试函数以Test开头","使用表格驱动测试"]}', 'go', 80, 1),
('Go错误处理', 'Go语言错误处理规范', '{"trigger":["error","错误"],"rules":["使用errors.New创建错误","使用fmt.Errorf格式化错误","错误信息应清晰明确"]}', 'go', 70, 1),
('Go并发模式', 'Go语言并发编程模式', '{"trigger":["goroutine","并发","channel"],"rules":["使用sync.WaitGroup等待goroutine","避免goroutine泄漏","合理使用缓冲channel"]}', 'go', 60, 1);
```

### 5.2 Git操作技能
```sql
INSERT INTO skills (name, description, content, category, priority, is_global) VALUES
('Git提交规范', 'Git提交信息编写规范', '{"trigger":["commit","提交"],"rules":["提交信息格式：<type>: <subject>","type可以是feat/fix/docs/style/refactor/test/chore","subject不超过50字符"]}', 'git', 90, 1),
('Git分支管理', 'Git分支管理策略', '{"trigger":["branch","分支"],"rules":["main分支用于发布","develop分支用于开发","feature分支用于新功能","hotfix分支用于紧急修复"]}', 'git', 80, 1);
```

### 5.3 Markdown转换技能
```sql
INSERT INTO skills (name, description, content, category, priority, is_global) VALUES
('Markdown到Org转换', 'Markdown到Org模式转换规则', '{"trigger":["markdown2org","转换"],"rules":["# -> *","## -> **","**text** -> *text*","*text* -> /text/","`code` -> =code="]}', 'markdown', 85, 1);
```

## 6. 集成到现有系统

### 6.1 修改助手响应流程
```go
func generateResponse(userQuery string, context Context) string {
    // 原有逻辑...
    
    // 新增：技能集成
    matcher := NewSkillMatcher(db)
    skills, err := matcher.MatchSkills(context.ProjectPath, userQuery)
    if err == nil && len(skills) > 0 {
        skillGuidance := matcher.ApplySkills(skills, context)
        // 将技能指导合并到响应中
        response = mergeWithSkills(response, skillGuidance)
    }
    
    return response
}
```

### 6.2 技能使用统计
```go
func recordSkillUsage(projectPath string, skillID int) {
    // 更新skills表的usage_count
    // 更新project_skills表的last_used
    // 记录使用日志
}
```

## 7. 优势与价值

### 7.1 技术优势
1. **知识复用**：避免重复回答相同问题
2. **一致性**：确保相同问题得到一致回答
3. **可维护性**：技能集中管理，易于更新
4. **可扩展性**：支持技能分类和优先级

### 7.2 用户体验
1. **更准确**：基于最佳实践的指导
2. **更高效**：快速获取标准化答案
3. **更智能**：随着使用越来越聪明
4. **个性化**：项目特定的技能配置

## 8. 实施步骤

### 第一阶段：基础框架（1-2周）
1. 实现技能管理接口
2. 创建初始技能库
3. 集成技能匹配引擎

### 第二阶段：智能学习（2-3周）
1. 实现技能自动学习
2. 添加使用统计和分析
3. 优化匹配算法

### 第三阶段：高级功能（3-4周）
1. 技能优先级调整
2. 技能冲突解决
3. 多项目技能共享

## 9. 风险与应对

### 风险1：技能冲突
- **应对**：优先级系统，后添加的技能覆盖先前的

### 风险2：技能过时
- **应对**：定期审查和更新机制

### 风险3：性能问题
- **应对**：缓存热门技能，异步更新统计

## 10. 总结

Skills系统将为项目提供一个智能化的知识库，通过复用最佳实践提高开发效率和质量。系统设计灵活，易于扩展，能够随着项目成长而进化。
