package toolcall

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"text/template"

	"gitcode.com/dscli/dscli/internal/sqlite"
)

// SegmentTemplateRenderer 段落模板渲染器
type SegmentTemplateRenderer struct {
	config *SystemPromptConfig
}

// NewSegmentTemplateRenderer 创建段落模板渲染器
func NewSegmentTemplateRenderer(ctx context.Context) *SegmentTemplateRenderer {
	return &SegmentTemplateRenderer{
		config: NewSystemPromptConfig(ctx),
	}
}

// GetSegmentsForProject 获取项目的提示词段落
func (r *SegmentTemplateRenderer) GetSegmentsForProject() (segment []PromptSegment, err error) {
	// 获取项目对应的领域ID
	var domainID int64
	db, err := sqlite.OpenDB()
	if err != nil {
		return
	}
	defer db.Close()
	err = db.QueryRow(`
		SELECT d.id 
		FROM domains d
		LEFT JOIN project_domains pd ON d.id = pd.domain_id
		WHERE pd.project_path = ? OR d.name = 'general'
		ORDER BY pd.project_path DESC
		LIMIT 1
	`, r.config.ProjectRoot).Scan(&domainID)
	if err != nil {
		// 如果没有找到特定领域，使用通用领域
		err = db.QueryRow(`SELECT id FROM domains WHERE name = 'general'`).Scan(&domainID)
		if err != nil {
			return nil, fmt.Errorf("获取领域ID失败: %w", err)
		}
	}

	// 获取该领域的段落，按模型和排序
	rows, err := db.Query(`
		SELECT id, domain_id, model_id, name, content, sort_order, enabled, created_at, updated_at
		FROM prompt_segments 
		WHERE domain_id = ? AND enabled = true
		AND (model_id = -1 OR model_id = ?)
		ORDER BY sort_order ASC
	`, domainID, r.config.ModelID)
	if err != nil {
		return nil, fmt.Errorf("查询段落失败: %w", err)
	}
	defer rows.Close()

	var segments []PromptSegment
	for rows.Next() {
		var segment PromptSegment
		err := rows.Scan(
			&segment.ID, &segment.DomainID, &segment.ModelID,
			&segment.Name, &segment.Content, &segment.SortOrder,
			&segment.Enabled, &segment.CreatedAt, &segment.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("扫描段落失败: %w", err)
		}
		segments = append(segments, segment)
	}

	return segments, nil
}

// RenderSegment 渲染单个段落
func (r *SegmentTemplateRenderer) RenderSegment(content string) (string, error) {
	// 如果内容不包含模板语法，直接返回
	if !strings.Contains(content, "{{") {
		return content, nil
	}

	tmpl, err := template.New("segment").Parse(content)
	if err != nil {
		return "", fmt.Errorf("解析段落模板失败: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, r.config); err != nil {
		return "", fmt.Errorf("渲染段落模板失败: %w", err)
	}

	return strings.TrimSpace(buf.String()), nil
}

// RenderAllSegments 渲染所有段落
func (r *SegmentTemplateRenderer) RenderAllSegments() (string, error) {
	segments, err := r.GetSegmentsForProject()
	if err != nil {
		return "", err
	}

	var result strings.Builder
	for _, segment := range segments {
		rendered, err := r.RenderSegment(segment.Content)
		if err != nil {
			// 如果渲染失败，使用原始内容
			rendered = segment.Content
		}

		if rendered != "" {
			result.WriteString(rendered)
			result.WriteString("\n\n")
		}
	}

	return strings.TrimSpace(result.String()), nil
}

// BuildSystemMessagesWithSegments 构建包含段落的系统消息
func BuildSystemMessagesWithSegments(ctx context.Context) ([]Message, error) {
	// 获取当前项目的领域ID
	domainID := GetCurrentDomainID(ctx)

	// 获取当前模型ID
	modelID := GetCurrentModelID(ctx)

	// 获取系统提示词配置
	config := GetSystemPromptConfig(ctx)

	// 使用段落管理器渲染系统提示词
	sm := &SegmentManager{}
	prompt, err := sm.RenderSystemPrompt(ctx, modelID, domainID, config)
	if err != nil {
		// 如果失败，使用基础提示词
		return []Message{{
			Role:    "system",
			Content: GetSystemPrompt(ctx),
		}}, nil
	}

	return []Message{{
		Role:    "system",
		Content: prompt,
	}}, nil
}

// 辅助函数：在模板中可用的函数
func (c *SystemPromptConfig) FormatDate() string {
	return c.CurrentDate
}

func (c *SystemPromptConfig) IsGoProject() bool {
	return c.ProjectType == "Go项目"
}

func (c *SystemPromptConfig) IsGitClean() bool {
	return c.GitStatus == "工作区干净"
}

// UpdateSegmentContent 更新段落内容（支持模板）
func UpdateSegmentContent(id int64, content string) (err error) {
	db, err := sqlite.OpenDB()
	if err != nil {
		return
	}
	defer db.Close()
	_, err = db.Exec(`
		UPDATE prompt_segments 
		SET content = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, content, id)
	return err
}

// CreateSegment 创建新段落
func CreateSegment(domainID, modelID int64, name, content string, sortOrder int) (err error) {
	db, err := sqlite.OpenDB()
	if err != nil {
		return
	}
	defer db.Close()
	_, err = db.Exec(`
		INSERT INTO prompt_segments (domain_id, model_id, name, content, sort_order, enabled)
		VALUES (?, ?, ?, ?, ?, true)
	`, domainID, modelID, name, content, sortOrder)
	return err
}
