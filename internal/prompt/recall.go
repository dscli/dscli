package prompt

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"gitcode.com/dscli/dscli/internal/sqlite"
	"gitcode.com/dscli/dscli/internal/tokenizer"
)

// Result 搜索结果
type Result struct {
	Message     Message
	ProjectPath string
}

// SearchMessages 搜索消息，使用 FTS5 全文搜索（中文分词，按相关性排序）。
// 只搜索 role=user 和 role=assistant(无tool_calls) 的消息。
// 仅搜索当前 session（对应当前项目）的消息，避免跨项目回忆。
// keywords: 搜索关键词（空格分隔，OR 逻辑，匹配任一即返回）。
// days: 搜索最近N天，<=0 表示不限时间。
// limit: 返回结果数量上限。
func SearchMessages(ctx context.Context, keywords []string, days, limit int) ([]Result, error) {
	if len(keywords) == 0 {
		return nil, fmt.Errorf("至少需要一个搜索关键词")
	}

	// 过滤空白关键词
	var valid []string
	for _, kw := range keywords {
		kw = strings.TrimSpace(kw)
		if kw != "" {
			valid = append(valid, kw)
		}
	}
	if len(valid) == 0 {
		return nil, fmt.Errorf("没有有效的搜索关键词")
	}

	db, err := sqlite.OpenDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	sessionID := GetCurrentSessionID(ctx)

	// 构建 FTS5 查询：将关键词用空格拼接后分词，再用 OR 连接（保持旧 LIKE 的 OR 语义）
	rawQuery := strings.Join(valid, " ")
	ftsTokens := tokenizer.SanitizeFTS(rawQuery)
	if ftsTokens == "" {
		return nil, fmt.Errorf("搜索关键词分词后为空")
	}
	// SanitizeFTS 输出 "词1" "词2"，替换空格为 OR 实现 OR 逻辑
	ftsQuery := strings.ReplaceAll(ftsTokens, " ", " OR ")

	conditions := []string{
		"messages_fts MATCH ?",
		"m.session_id = ?",
		// 只搜索 user 消息和助手总结（无 tool_calls 的 assistant 消息）
		`(m.role = 'user' OR (m.role = 'assistant' AND (m.tool_calls IS NULL OR m.tool_calls = '' OR m.tool_calls = '[]')))`,
	}

	args := []any{ftsQuery, sessionID}

	// 时间过滤
	if days > 0 {
		conditions = append(conditions, `m.created_at >= ?`)
		args = append(args, time.Now().AddDate(0, 0, -days).Format("2006-01-02 15:04:05"))
	}

	whereClause := strings.Join(conditions, " AND ")

	query := fmt.Sprintf(`
		SELECT m.id, m.session_id, m.role, m.content, m.tool_call_id, m.tool_calls,
		       m.created_at, m.model_id, m.reasoning_content, s.project_path, fts.rank
		FROM messages_fts fts
		JOIN messages m ON m.id = fts.rowid
		JOIN sessions s ON m.session_id = s.id
		WHERE %s
		ORDER BY fts.rank
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
		var rank float64
		if err := rows.Scan(&r.Message.ID, &r.Message.SessionID, &r.Message.Role,
			&r.Message.Content, &toolCallID, &toolCalls,
			&r.Message.CreatedAt, &r.Message.ModelID, &r.Message.ReasoningContent,
			&r.ProjectPath, &rank); err != nil {
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

func HandleRecall(ctx context.Context, keywordsStr string, days, limit int) (result, warning string, err error) {
	// 防止 recall 结果撑爆 LLM 上下文
	const (
		maxRecallContentRunes = 2000 // 单条消息截断上限
		maxRecallResults      = 10   // 单次返回结果上限
	)

	// 按空格拆分关键词
	var keywords []string
	for kw := range strings.FieldsSeq(keywordsStr) {
		kw = strings.TrimSpace(kw)
		if kw != "" {
			keywords = append(keywords, kw)
		}
	}

	if len(keywords) == 0 {
		err = fmt.Errorf("没有有效的搜索关键词")
		return result, warning, err
	}

	results, searchErr := SearchMessages(ctx, keywords, days, limit)
	if searchErr != nil {
		err = searchErr
		return result, warning, err
	}

	if len(results) == 0 {
		result = "没有找到匹配的历史消息。"
		return result, warning, err
	}

	// 格式化结果（每条截断 + 总数限制，防止撑爆 LLM 上下文）
	var b strings.Builder
	fmt.Fprintf(&b, "找到 **%d** 条相关历史消息：\n\n", len(results))
	for i, r := range results {
		if i >= maxRecallResults {
			fmt.Fprintf(&b, "\n（还有 %d 条结果未显示，可缩小搜索范围或指定 limit）",
				len(results)-maxRecallResults)
			break
		}
		roleLabel := "🙋 用户"
		if r.Message.Role == "assistant" {
			roleLabel = "🤖 助手"
		}
		timeStr := FormatTime(r.Message.CreatedAt)

		content := r.Message.Content
		if runes := []rune(content); len(runes) > maxRecallContentRunes {
			content = string(runes[:maxRecallContentRunes]) + "..."
		}

		fmt.Fprintf(&b, "%d. %s %s %s\n", i+1, timeStr, roleLabel, content)
	}
	result = b.String()
	return result, warning, err
}
