package history

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"gitcode.com/dscli/dscli/internal/sqlite"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

// Result 搜索结果
type Result struct {
	Message     toolcall.Message
	ProjectPath string
}

// SearchMessages 搜索消息，支持多关键词（空格分隔），按 LIKE 匹配
// 只搜索 role=user 和 role=assistant(无tool_calls) 的消息
// 仅搜索当前 session（对应当前项目）的消息，避免跨项目回忆
// days: 搜索最近N天，<=0 表示不限时间
// limit: 返回结果数量上限
func SearchMessages(ctx context.Context, keywords []string, days int, limit int) ([]Result, error) {
	if len(keywords) == 0 {
		return nil, fmt.Errorf("至少需要一个搜索关键词")
	}

	db, err := sqlite.OpenDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	sessionID := GetCurrentSessionID(ctx)

	// 构建 WHERE 条件
	conditions := []string{
		// 限定当前项目 session，避免跨项目回忆
		`m.session_id = ?`,
		// 只搜索 user 消息和助手总结（无 tool_calls 的 assistant 消息）
		`(m.role = 'user' OR (m.role = 'assistant' AND (m.tool_calls IS NULL OR m.tool_calls = '' OR m.tool_calls = '[]')))`,
	}

	args := []any{sessionID}

	// 时间过滤（参数化查询，避免 SQL 注入）
	if days > 0 {
		conditions = append(conditions, `m.created_at >= ?`)
		args = append(args, time.Now().AddDate(0, 0, -days).Format("2006-01-02 15:04:05"))
	}

	// 关键词 LIKE 条件（OR 逻辑）
	var likeConditions []string
	for _, kw := range keywords {
		kw = strings.TrimSpace(kw)
		if kw == "" {
			continue
		}
		likeConditions = append(likeConditions, `m.content LIKE ?`)
		args = append(args, `%`+kw+`%`)
	}
	if len(likeConditions) == 0 {
		return nil, fmt.Errorf("没有有效的搜索关键词")
	}
	conditions = append(conditions, "("+strings.Join(likeConditions, " OR ")+")")

	whereClause := strings.Join(conditions, " AND ")

	query := fmt.Sprintf(`
		SELECT m.id, m.session_id, m.role, m.content, m.tool_call_id, m.tool_calls,
		       m.created_at, m.model_id, m.reasoning_content, s.project_path
		FROM messages m
		JOIN sessions s ON m.session_id = s.id
		WHERE %s
		ORDER BY m.created_at DESC
		LIMIT ?`, whereClause)

	args = append(args, limit)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("搜索消息失败: %w", err)
	}
	defer rows.Close()

	var results []Result
	for rows.Next() {
		var r Result
		var toolCallID, toolCalls sql.NullString
		if err := rows.Scan(&r.Message.ID, &r.Message.SessionID, &r.Message.Role,
			&r.Message.Content, &toolCallID, &toolCalls,
			&r.Message.CreatedAt, &r.Message.ModelID, &r.Message.ReasoningContent,
			&r.ProjectPath); err != nil {
			return nil, fmt.Errorf("扫描搜索结果失败: %w", err)
		}
		if toolCallID.Valid {
			r.Message.ToolCallID = toolCallID.String
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历搜索结果失败: %w", err)
	}

	return results, nil
}

// FormatTime 格式化时间为简短形式
func FormatTime(t time.Time) string {
	now := time.Now()
	if t.Year() == now.Year() && t.YearDay() == now.YearDay() {
		return t.Format("15:04")
	}
	if t.Year() == now.Year() {
		return t.Format("01-02 15:04")
	}
	return t.Format("2006-01-02 15:04")
}

// Truncate 截断内容用于预览
func Truncate(content string, maxLen int) string {
	// 去掉前导空白
	content = strings.TrimSpace(content)
	// 将换行替换为空格
	content = strings.ReplaceAll(content, "\n", " ")
	content = strings.ReplaceAll(content, "\r", " ")
	// 合并多个空格
	parts := strings.Fields(content)
	content = strings.Join(parts, " ")

	runes := []rune(content)
	if len(runes) <= maxLen {
		return content
	}
	return string(runes[:maxLen]) + "..."
}