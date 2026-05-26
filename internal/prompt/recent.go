package prompt

import (
	"context"
	"fmt"
	"strings"

	"gitcode.com/dscli/dscli/internal/sqlite"
)

// RecentMessages 返回当前 session 中最近的用户/助手消息（过滤 tool/tool_calls），
// 按 created_at 降序排列（最新在顶部）。
func RecentMessages(ctx context.Context, limit int) ([]Message, error) {
	sessionID := GetCurrentSessionID(ctx)

	db, err := sqlite.OpenDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, `
		SELECT id, role, content, created_at
		FROM messages
		WHERE session_id = ?
		  AND (role = 'user' OR (role = 'assistant' AND (tool_calls IS NULL OR tool_calls = '' OR tool_calls = '[]')))
		ORDER BY created_at DESC
		LIMIT ?`, sessionID, limit)
	if err != nil {
		return nil, fmt.Errorf("查询最近消息失败: %w", err)
	}
	defer rows.Close()

	var msgs []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.Role, &m.Content, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("扫描消息失败: %w", err)
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历消息失败: %w", err)
	}

	return msgs, nil
}

// HandleRecent 格式化最近消息为表格，供 LLM 工具调用。
func HandleRecent(ctx context.Context, limit int) (result, warning string, err error) {
	const maxRecentResults = 20

	if limit <= 0 || limit > maxRecentResults {
		limit = maxRecentResults
	}

	msgs, err := RecentMessages(ctx, limit)
	if err != nil {
		return result, warning, err
	}

	if len(msgs) == 0 {
		result = "当前会话没有历史消息。"
		return result, warning, err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "最近 **%d** 条消息（按时间降序，最新在顶部）：\n\n", len(msgs))
	fmt.Fprintf(&b, "| ID | 时间 | 角色 | 关键词 |\n")
	fmt.Fprintf(&b, "|----|------|------|--------|\n")
	for _, m := range msgs {
		roleLabel := "用户"
		if m.Role == "assistant" {
			roleLabel = "助手"
		}
		timeStr := FormatTime(m.CreatedAt)
		preview := Truncate(m.Content, 80)
		fmt.Fprintf(&b, "| %d | %s | %s | %s |\n", m.ID, timeStr, roleLabel, preview)
	}
	result = b.String()
	return result, warning, err
}
