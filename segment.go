package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Domain 领域定义
type Domain struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

// PromptSegment 提示词段落
type PromptSegment struct {
	ID        int64     `json:"id"`
	DomainID  int64     `json:"domain_id"`
	ModelID   int64     `json:"model_id"` // -1=通用, 0=deepseek-chat, 1=deepseek-reasoner
	Name      string    `json:"name"`
	Content   string    `json:"content"`
	SortOrder int       `json:"sort_order"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ProjectDomain 项目领域关联
type ProjectDomain struct {
	ID          int64     `json:"id"`
	ProjectRoot string    `json:"project_path"`
	DomainID    int64     `json:"domain_id"`
	CreatedAt   time.Time `json:"created_at"`
}

// 初始化函数
func init() {
	// 注册表结构
	RegisterTableSchema(
		`CREATE TABLE IF NOT EXISTS domains (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			description TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS prompt_segments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			domain_id INTEGER NOT NULL,
			model_id INTEGER NOT NULL DEFAULT -1,
			name TEXT NOT NULL,
			content TEXT NOT NULL,
			sort_order INTEGER NOT NULL DEFAULT 0,
			enabled BOOLEAN NOT NULL DEFAULT true,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (domain_id) REFERENCES domains(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS project_domains (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_path TEXT NOT NULL,
			domain_id INTEGER NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (domain_id) REFERENCES domains(id) ON DELETE CASCADE,
			UNIQUE(project_path)
		)`,
	)

	// 注册索引
	RegisterIndexSchema(
		`CREATE INDEX IF NOT EXISTS idx_prompt_segments_domain_model 
		ON prompt_segments(domain_id, model_id, sort_order) WHERE enabled = true`,
		`CREATE INDEX IF NOT EXISTS idx_prompt_segments_enabled 
		ON prompt_segments(enabled, sort_order)`,
	)

	// 注册升级脚本
	RegisterUpgradeSchema(
		`DROP INDEX IF EXISTS idx_project_domains_root`,
		`ALTER TABLE project_domains RENAME COLUMN project_root TO project_path`,
		`CREATE INDEX IF NOT EXISTS idx_project_domains_path
		ON project_domains(project_path)`,
		`INSERT OR IGNORE INTO domains (id, name, description) VALUES 
		(0, 'programming', '编程开发 - 代码编写、审查、调试等'),
		(1, 'documentation', '文档写作 - 技术文档、用户手册、API文档等'),
		(2, 'mathematics', '数学研究 - 数学推导、证明、公式计算等'),
		(3, 'industrial', '工业控制 - 燃气锅炉、PLC、自动化控制等'),
		(4, 'research', '科学研究 - 实验设计、数据分析、论文写作等'),
		(5, 'general', '通用助手 - 日常问答、文本处理、学习辅导等')`,
	)

	// 注册后初始化钩子
	RegisterPostInitHook(func(db *sql.DB) error {
		return initDefaultSegments(db)
	})
}

// initDefaultSegments 初始化默认段落
func initDefaultSegments(db *sql.DB) error {
	// 编程领域的ID为0
	programmingID := int64(0)

	// 检查是否已经有段落
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM prompt_segments WHERE domain_id = ?", programmingID).Scan(&count)
	if err != nil {
		return err
	}

	if count > 0 {
		return nil
	}

	// 插入默认段落
	segments := []struct {
		name    string
		modelID int64
		order   int
		content string
	}{
		{
			name:    "基本原则",
			modelID: -1,
			order:   10,
			content: `你是一个专业的编程助手。`,
		},
		{
			name:    "工作流程",
			modelID: -1,
			order:   20,
			content: `你的工作流程：
1. 仔细分析用户的问题
2. 调用适当的工具
3. 逐步推进
4. 给出清晰答案`,
		},
		{
			name:    "代码质量要求",
			modelID: -1,
			order:   30,
			content: `代码质量要求：
1. 保持代码简洁
2. 遵循最佳实践
3. 添加必要注释
4. 考虑错误处理`,
		},
	}

	for _, segment := range segments {
		_, err := db.Exec(`
			INSERT INTO prompt_segments (domain_id, model_id, name, content, sort_order, enabled)
			VALUES (?, ?, ?, ?, ?, true)
		`, programmingID, segment.modelID, segment.name, segment.content, segment.order)
		if err != nil {
			return fmt.Errorf("插入段落失败: %w", err)
		}
	}

	return nil
}

// BuildSystemMessages 构建系统消息
func BuildSystemMessages(ctx context.Context) ([]Message, error) {
	// 使用包含段落的系统消息构建器
	return BuildSystemMessagesWithSegments(ctx)
}
