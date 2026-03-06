package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strings"
	"text/template"
)

// SegmentManager 段落管理器
type SegmentManager struct{}

// NewSegmentManager 创建段落管理器
func NewSegmentManager() *SegmentManager {
	return &SegmentManager{}
}

// GetSystemPrompt 获取系统提示词
func (sm *SegmentManager) GetSystemPrompt(ctx context.Context, modelID int64, domainID int64) (string, error) {
	// 获取系统级段落（domain_id=0）和指定领域的段落
	segments, err := sm.GetSegmentsForPrompt(ctx, modelID, domainID)
	if err != nil {
		return "", fmt.Errorf("获取段落失败: %w", err)
	}

	// 如果没有找到段落，使用硬编码模板
	if len(segments) == 0 {
		log.Printf("没有找到模型 %d 的段落，使用硬编码模板", modelID)
		return sm.getHardcodedTemplate(modelID), nil
	}

	// 拼接所有段落内容
	var builder strings.Builder
	for _, segment := range segments {
		builder.WriteString(segment.Content)
		builder.WriteString("\n\n")
	}

	return strings.TrimSpace(builder.String()), nil
}

// GetSegmentsForPrompt 获取用于生成提示词的段落
func (sm *SegmentManager) GetSegmentsForPrompt(ctx context.Context, modelID int64, domainID int64) ([]PromptSegment, error) {
	db, err := OpenDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	// 查询系统级段落和指定领域的段落
	// 模型ID匹配规则：-1表示通用段落，或者与指定modelID匹配
	rows, err := db.QueryContext(ctx, `
		SELECT id, domain_id, model_id, name, content, sort_order, enabled
		FROM prompt_segments 
		WHERE enabled = 1 
		AND (domain_id = 0 OR domain_id = ?)
		AND (model_id = -1 OR model_id = ?)
		ORDER BY 
			CASE WHEN domain_id = 0 THEN 0 ELSE 1 END, -- 系统级段落在前
			sort_order
	`, domainID, modelID)
	if err != nil {
		return nil, fmt.Errorf("查询段落失败: %w", err)
	}
	defer rows.Close()

	var segments []PromptSegment
	for rows.Next() {
		var seg PromptSegment
		err := rows.Scan(&seg.ID, &seg.DomainID, &seg.ModelID, &seg.Name, &seg.Content, &seg.SortOrder, &seg.Enabled)
		if err != nil {
			return nil, fmt.Errorf("扫描段落失败: %w", err)
		}
		segments = append(segments, seg)
	}

	return segments, nil
}

// getHardcodedTemplate 获取硬编码模板
func (sm *SegmentManager) getHardcodedTemplate(modelID int64) string {
	// 创建基础配置
	config := &SystemPromptConfig{
		CurrentDate: "2026年03月06日", // 示例日期
		ProjectRoot: "/home/nanjj/src/gitcode.com/dscli/dscli",
		ConfigDir:   "/home/nanjj/.dscli",
		ProjectName: "dscli",
		ProjectType: "Go项目",
		Hostname:    "dev01",
		Username:    "nanjj",
	}

	switch modelID {
	case DeepseekChat:
		return config.generateDeepseekChatPrompt()
	case DeepseekReasoner:
		return config.generateDeepseekReasonerPrompt()
	default:
		log.Printf("不支持模型ID: %d，使用通用模板", modelID)
		return config.generateDeepseekChatPrompt()
	}
}

// RenderSystemPrompt 渲染系统提示词
func (sm *SegmentManager) RenderSystemPrompt(ctx context.Context, modelID int64, domainID int64, config *SystemPromptConfig) (string, error) {
	// 获取系统提示词
	promptContent, err := sm.GetSystemPrompt(ctx, modelID, domainID)
	if err != nil {
		return "", fmt.Errorf("获取系统提示词失败: %w", err)
	}

	// 解析模板
	tmpl, err := template.New("system_prompt").Parse(promptContent)
	if err != nil {
		log.Printf("解析模板失败: %v，使用原始内容", err)
		// 如果解析失败，直接返回原始内容
		return promptContent, nil
	}

	// 渲染模板
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, config); err != nil {
		log.Printf("渲染模板失败: %v，使用原始内容", err)
		// 如果渲染失败，直接返回原始内容
		return promptContent, nil
	}

	return strings.TrimSpace(buf.String()), nil
}

// ListSegments 列出所有段落
func (sm *SegmentManager) ListSegments(ctx context.Context) ([]PromptSegment, error) {
	db, err := OpenDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, `
		SELECT id, domain_id, model_id, name, content, sort_order, enabled
		FROM prompt_segments 
		ORDER BY domain_id, model_id, sort_order
	`)
	if err != nil {
		return nil, fmt.Errorf("查询段落失败: %w", err)
	}
	defer rows.Close()

	var segments []PromptSegment
	for rows.Next() {
		var seg PromptSegment
		err := rows.Scan(&seg.ID, &seg.DomainID, &seg.ModelID, &seg.Name, &seg.Content, &seg.SortOrder, &seg.Enabled)
		if err != nil {
			return nil, fmt.Errorf("扫描段落失败: %w", err)
		}
		segments = append(segments, seg)
	}

	return segments, nil
}

// UpdateSegment 更新段落
func (sm *SegmentManager) UpdateSegment(ctx context.Context, id int64, name, content string, sortOrder int, enabled bool) error {
	db, err := OpenDB()
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = db.ExecContext(ctx, `
		UPDATE prompt_segments 
		SET name = ?, content = ?, sort_order = ?, enabled = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, name, content, sortOrder, enabled, id)
	return err
}

// DeleteSegment 删除段落
func (sm *SegmentManager) DeleteSegment(ctx context.Context, id int64) error {
	db, err := OpenDB()
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = db.ExecContext(ctx, `
		DELETE FROM prompt_segments 
		WHERE id = ?
	`, id)
	return err
}

// CreateSegment 创建段落
func (sm *SegmentManager) CreateSegment(ctx context.Context, domainID, modelID int64, name, content string, sortOrder int) error {
	db, err := OpenDB()
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = db.ExecContext(ctx, `
		INSERT INTO prompt_segments (domain_id, model_id, name, content, sort_order, enabled)
		VALUES (?, ?, ?, ?, ?, 1)
	`, domainID, modelID, name, content, sortOrder)
	return err
}

// GetSegmentByID 根据ID获取段落
func (sm *SegmentManager) GetSegmentByID(ctx context.Context, id int64) (*PromptSegment, error) {
	db, err := OpenDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	seg := &PromptSegment{}
	err = db.QueryRowContext(ctx, `
		SELECT id, domain_id, model_id, name, content, sort_order, enabled
		FROM prompt_segments 
		WHERE id = ?
	`, id).Scan(&seg.ID, &seg.DomainID, &seg.ModelID, &seg.Name, &seg.Content, &seg.SortOrder, &seg.Enabled)
	if err != nil {
		return nil, err
	}
	return seg, nil
}

// ListDomains 列出所有领域
func (sm *SegmentManager) ListDomains(ctx context.Context) ([]Domain, error) {
	db, err := OpenDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, `
		SELECT id, name, description, created_at
		FROM domains 
		ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("查询领域失败: %w", err)
	}
	defer rows.Close()

	var domains []Domain
	for rows.Next() {
		var domain Domain
		err := rows.Scan(&domain.ID, &domain.Name, &domain.Description, &domain.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("扫描领域失败: %w", err)
		}
		domains = append(domains, domain)
	}

	return domains, nil
}

// GetDomainByID 根据ID获取领域
func (sm *SegmentManager) GetDomainByID(ctx context.Context, id int64) (*Domain, error) {
	db, err := OpenDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	domain := &Domain{}
	err = db.QueryRowContext(ctx, `
		SELECT id, name, description, created_at
		FROM domains 
		WHERE id = ?
	`, id).Scan(&domain.ID, &domain.Name, &domain.Description, &domain.CreatedAt)
	if err != nil {
		return nil, err
	}
	return domain, nil
}
