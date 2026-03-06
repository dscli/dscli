package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"text/template"
)

// SegmentManager 段落管理器
type SegmentManager struct {
	db *sql.DB
}

// NewSegmentManager 创建段落管理器
func NewSegmentManager() *SegmentManager {
	return &SegmentManager{
		db: GetDB(),
	}
}

// ListSegments 列出所有段落
func (m *SegmentManager) ListSegments(domainName string, modelID int64) ([]PromptSegment, error) {
	query := `
		SELECT ps.id, ps.domain_id, ps.model_id, ps.name, ps.content, 
		       ps.sort_order, ps.enabled, ps.created_at, ps.updated_at,
		       d.name as domain_name
		FROM prompt_segments ps
		JOIN domains d ON ps.domain_id = d.id
		WHERE ps.enabled = true
	`

	args := []interface{}{}
	if domainName != "" {
		query += " AND d.name = ?"
		args = append(args, domainName)
	}

	if modelID != -2 { // -2 表示不筛选模型
		query += " AND (ps.model_id = -1 OR ps.model_id = ?)"
		args = append(args, modelID)
	}

	query += " ORDER BY d.name, ps.sort_order ASC"

	rows, err := m.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("查询段落失败: %w", err)
	}
	defer rows.Close()

	var segments []PromptSegment
	for rows.Next() {
		var segment PromptSegment
		var domainName string
		err := rows.Scan(
			&segment.ID, &segment.DomainID, &segment.ModelID,
			&segment.Name, &segment.Content, &segment.SortOrder,
			&segment.Enabled, &segment.CreatedAt, &segment.UpdatedAt,
			&domainName,
		)
		if err != nil {
			return nil, fmt.Errorf("扫描段落失败: %w", err)
		}
		segments = append(segments, segment)
	}

	return segments, nil
}

// GetSegment 获取单个段落
func (m *SegmentManager) GetSegment(id int64) (*PromptSegment, error) {
	var segment PromptSegment
	err := m.db.QueryRow(`
		SELECT id, domain_id, model_id, name, content, sort_order, enabled, created_at, updated_at
		FROM prompt_segments
		WHERE id = ?
	`, id).Scan(
		&segment.ID, &segment.DomainID, &segment.ModelID,
		&segment.Name, &segment.Content, &segment.SortOrder,
		&segment.Enabled, &segment.CreatedAt, &segment.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("获取段落失败: %w", err)
	}
	return &segment, nil
}

// ToggleSegment 切换段落启用状态
func (m *SegmentManager) ToggleSegment(id int64, enabled bool) error {
	_, err := m.db.Exec(`
		UPDATE prompt_segments 
		SET enabled = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, enabled, id)
	return err
}

// UpdateSegmentOrder 更新段落排序
func (m *SegmentManager) UpdateSegmentOrder(id int64, sortOrder int) error {
	_, err := m.db.Exec(`
		UPDATE prompt_segments 
		SET sort_order = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, sortOrder, id)
	return err
}

// DeleteSegment 删除段落
func (m *SegmentManager) DeleteSegment(id int64) error {
	_, err := m.db.Exec("DELETE FROM prompt_segments WHERE id = ?", id)
	return err
}

// AssignProjectDomain 为项目分配领域
func (m *SegmentManager) AssignProjectDomain(projectRoot string, domainName string) error {
	// 获取领域ID
	var domainID int64
	err := m.db.QueryRow("SELECT id FROM domains WHERE name = ?", domainName).Scan(&domainID)
	if err != nil {
		return fmt.Errorf("获取领域失败: %w", err)
	}

	// 插入或更新项目领域关联
	_, err = m.db.Exec(`
		INSERT INTO project_domains (project_root, domain_id)
		VALUES (?, ?)
		ON CONFLICT(project_root) DO UPDATE SET domain_id = ?
	`, projectRoot, domainID, domainID)

	return err
}

// GetProjectDomain 获取项目的领域
func (m *SegmentManager) GetProjectDomain(projectRoot string) (string, error) {
	var domainName string
	err := m.db.QueryRow(`
		SELECT d.name
		FROM project_domains pd
		JOIN domains d ON pd.domain_id = d.id
		WHERE pd.project_root = ?
	`, projectRoot).Scan(&domainName)

	if err == sql.ErrNoRows {
		return "general", nil // 默认使用通用领域
	}

	return domainName, err
}

// PreviewSegment 预览段落渲染结果
func (m *SegmentManager) PreviewSegment(ctx context.Context, content string) (string, error) {
	config := NewSystemPromptConfig(ctx)
	renderer := &SegmentTemplateRenderer{
		db:     m.db,
		config: config,
	}
	return renderer.RenderSegment(content)
}

// TestSegmentTemplate 测试段落模板
func (m *SegmentManager) TestSegmentTemplate(ctx context.Context, templateStr string) (string, error) {
	config := NewSystemPromptConfig(ctx)

	tmpl, err := template.New("test").Funcs(template.FuncMap{
		"formatDate":  func() string { return config.FormatDate() },
		"isGoProject": func() bool { return config.IsGoProject() },
		"isGitClean":  func() bool { return config.IsGitClean() },
	}).Parse(templateStr)
	if err != nil {
		return "", fmt.Errorf("模板语法错误: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, config); err != nil {
		return "", fmt.Errorf("模板渲染错误: %w", err)
	}

	return buf.String(), nil
}
