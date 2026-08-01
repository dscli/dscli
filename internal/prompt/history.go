package prompt

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/session"
	"github.com/dscli/dscli/internal/sqlite"
	"github.com/nanjj/clog"
)

// 注意：Message 和 ToolCall 类型定义在 prompt 包中。
// 本包直接使用 Message 和 ToolCall，不再提供类型别名。
// 历史兼容：旧代码中 import "history" 后使用 history.Message 的地方，
// 需要改为使用 Message。

var GetCurrentSessionID = session.GetCurrentSessionID

// UpdateContent update message content
func UpdateContent(ctx context.Context, id int64, content string) (err error) {
	span, ctx := clog.StartSpanFromContext(ctx, "UpdateContent")
	defer span.Finish()
	db, err := sqlite.OpenDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close(ctx)
	res, err := db.ExecContext(ctx,
		`UPDATE messages SET content = ? WHERE id = ?`,
		content, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if affected != 1 {
		err = fmt.Errorf("failed to update message content")
	}
	return err
}

func ToSQLNullString(tcs []ToolCall) (toolCalls sql.NullString) {
	data, err := outfmt.JSONMarshal(tcs)
	if err != nil {
		return toolCalls
	}
	toolCalls.String = string(data)
	toolCalls.Valid = true
	return toolCalls
}

// UpdateToolCalls update message content
func UpdateToolCalls(ctx context.Context, id int64, tcs []ToolCall) (err error) {
	span, ctx := clog.StartSpanFromContext(ctx, "UpdateToolCalls")
	defer span.Finish()

	db, err := sqlite.OpenDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close(ctx)
	toolCalls := ToSQLNullString(tcs)
	res, err := db.ExecContext(ctx,
		`UPDATE messages SET tool_calls = ? WHERE id = ?`,
		&toolCalls, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if affected != 1 {
		err = fmt.Errorf("failed to update message content")
	}
	return err
}

// UpdateHistory update message session_id to 0
func UpdateHistory(ctx context.Context, id int64) (err error) {
	span, ctx := clog.StartSpanFromContext(ctx, "UpdateHistory")
	defer span.Finish()

	db, err := sqlite.OpenDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close(ctx)
	_, err = db.ExecContext(ctx,
		`UPDATE messages SET session_id = 0 WHERE id = ?`,
		id)
	if err != nil {
		return err
	}
	return err
}

func ShowMessage(ctx context.Context, id int64) (message *Message, err error) {
	span, ctx := clog.StartSpanFromContext(ctx, "ShowMessage")
	defer span.Finish()

	db, err := sqlite.OpenDB(ctx)
	if err != nil {
		return message, err
	}
	defer db.Close(ctx)
	var toolCalls sql.NullString
	var toolCallID sql.NullString
	var tokens int
	message = &Message{}
	err = db.QueryRowContext(ctx, `SELECT id, session_id, role, content, tool_call_id, `+
		`tool_calls, created_at, model_id, reasoning_content, tokens FROM messages WHERE `+
		`id = ?`, id).Scan(&message.ID,
		&message.SessionID, &message.Role, &message.Content, &toolCallID,
		&toolCalls, &message.CreatedAt, &message.ModelID, &message.ReasoningContent,
		&tokens)
	if err != nil {
		return message, err
	}
	message.SetTokens(tokens)
	if toolCalls.Valid {
		err = json.Unmarshal([]byte(toolCalls.String), &message.ToolCalls)
		if err != nil {
			return message, err
		}
	}
	if toolCallID.Valid {
		message.ToolCallID = toolCallID.String
	}
	return message, nil
}

// ListHistory 加载指定会话的历史消息，按时间升序返回。
// beforeID > 0 时按 keyset 分页：只返回 id < beforeID 的消息（用于 history list 翻页，
// 客户端以"返回条数 < histsize"作为没有更多数据的终止条件）。
func ListHistory(ctx context.Context, beforeID int64) ([]*Message, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "ListHistory")
	defer span.Finish()
	sessionID := GetCurrentSessionID(ctx)
	modelID := context.ContextValue(ctx, context.CurrentModelIDKey, context.DeepseekChat)
	histSize := context.ContextValue(ctx, context.HistSizeKey, 8)
	db, err := sqlite.OpenDB(ctx)
	if err != nil {
		return nil, err
	}
	defer db.Close(ctx)
	query := `SELECT id, role, content, tool_call_id, tool_calls, created_at, reasoning_content, tokens
		FROM messages
		WHERE session_id = ? AND model_id = ?`
	args := []any{sessionID, modelID}
	if beforeID > 0 {
		query += ` AND id < ?`
		args = append(args, beforeID)
	}
	query += `
		ORDER BY id DESC
        LIMIT ?`
	args = append(args, histSize+2)
	rows, err := db.QueryContext(ctx, query, args...)
	// histSize + 2就可以，因为主要就是最后两个。
	// 注意我们按降低排的序：{100, 99, 98, ...} 最大ID在前面
	// 应用LIMIT，总能把最新消息的找出来。但我们提交给大语言模型时，
	// 最新消息要在最后: {...,98, 99, 100}。
	if err != nil {
		return nil, fmt.Errorf("查询历史消息失败: %w", err)
	}
	defer rows.Close()

	var messages []*Message
	for rows.Next() {
		m := &Message{}
		var toolCallID, toolCalls, reasoningContent sql.NullString
		var tokens int
		if err := rows.Scan(&m.ID, &m.Role, &m.Content, &toolCallID, &toolCalls, &m.CreatedAt, &reasoningContent, &tokens); err != nil {
			return nil, fmt.Errorf("扫描消息失败: %w", err)
		}
		m.SetTokens(tokens)
		if toolCallID.Valid {
			m.ToolCallID = toolCallID.String
		}
		if toolCalls.Valid {
			var toolCallsData []ToolCall
			if err := json.Unmarshal([]byte(toolCalls.String), &toolCallsData); err == nil {
				m.ToolCalls = toolCallsData
			}
		}
		if reasoningContent.Valid {
			m.ReasoningContent = reasoningContent.String
		}
		messages = append(messages, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历消息失败: %w", err)
	}

	JudgeHistory(messages)
	slices.Reverse(messages)
	return messages, nil
}

// LoadHistory 加载指定会话的所有历史消息，按时间升序返回
func LoadHistory(ctx context.Context) ([]Message, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "LoadHistory")
	defer span.Finish()

	histSize := context.ContextValue(ctx, context.HistSizeKey, 8)
	if histSize == 0 {
		return []Message{}, nil
	}

	sessionID := GetCurrentSessionID(ctx)
	modelID := context.ContextValue(ctx, context.CurrentModelIDKey, context.DeepseekChat)
	leftTokens := context.ContextValue(ctx, context.LeftTokensKey, 0)

	db, err := sqlite.OpenDB(ctx)
	if err != nil {
		return nil, err
	}
	defer db.Close(ctx)

	// 提高 LIMIT：原本 histSize+2 对工具调用场景太小（一个完整轮次可能 4-6 条消息），
	// 增大后确保压缩过滤后仍有足够的历史轮次。
	rows, err := db.Query(`
		SELECT id, role, content, tool_call_id, tool_calls, created_at, reasoning_content, tokens
		FROM messages
		WHERE session_id = ? AND model_id = ?
		ORDER BY id DESC
		LIMIT ?`, sessionID, modelID, histSize*5)
	if err != nil {
		return nil, fmt.Errorf("查询历史消息失败: %w", err)
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		var toolCallID, toolCalls sql.NullString
		var tokens int
		if err := rows.Scan(&m.ID, &m.Role, &m.Content, &toolCallID, &toolCalls, &m.CreatedAt, &m.ReasoningContent, &tokens); err != nil {
			return nil, fmt.Errorf("扫描消息失败: %w", err)
		}
		m.SetTokens(tokens)
		if toolCallID.Valid {
			m.ToolCallID = toolCallID.String
		}
		if toolCalls.Valid {
			var toolCallsData []ToolCall
			if err := json.Unmarshal([]byte(toolCalls.String), &toolCallsData); err == nil {
				m.ToolCalls = toolCallsData
			}
		}
		if m.tokens == 0 {
			tokens = m.GetTokens()
		}
		leftTokens -= tokens
		if leftTokens <= tokens*2 {
			break
		}
		messages = append(messages, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历消息失败: %w", err)
	}

	// Cleanup — 重排为 ASC 并正确配对 assistant↔tool
	messages = CleanupReverse(messages)

	// 对话压缩：当最后一轮已完成（最后一条是 assistant 且无 tool_calls），
	// 跳过中间 assistant(tc)+tool 消息，只保留 user + assistant(content)，
	// 用同样 token 预算容纳更多对话轮次。
	if len(messages) > 0 {
		last := messages[len(messages)-1]
		if last.Role == "assistant" && len(last.ToolCalls) == 0 {
			messages = compressHistory(messages)
			// 压缩后按 histSize 截断
			if len(messages) > histSize {
				messages = messages[len(messages)-histSize:]
			}
			return messages, nil
		}
	}

	// 原截断逻辑（对话未完成时使用）
	n := len(messages)
	idx := n - histSize
	if idx > 0 {
		for {
			m := messages[idx]
			role := m.Role
			if role == "assistant" || idx == 0 {
				break
			}
			idx -= 1
		}
	} else {
		idx = 0
	}
	return messages[idx:], nil
}

// JudgeHistory - Cleanup the history
func JudgeHistory(messages []*Message) {
	// The messages is in decrease order {100, 99, 98, ...}
	l := len(messages)
	if l == 0 {
		return
	}

	for i, message := range messages[0 : l-1] {
		nextMessage := messages[i+1]
		message.OK = true
		if message.Role == "assistant" {
			if i > 0 {
				prevMessage := messages[i-1]
				if len(message.ToolCalls) == 1 {
					if !prevMessage.OK && prevMessage.Role == "tool" {
						message.OK = false
					}
					if prevMessage.Role != "tool" {
						message.OK = false
					}
				}
			}
			continue
		}

		if message.Role == "user" || message.Role == "system" {
			continue
		}

		// handle the left role = tool
		message.OK = false
		if message.ToolCallID != "" &&
			nextMessage.Role == "assistant" &&
			len(nextMessage.ToolCalls) != 0 &&
			message.ToolCallID == nextMessage.ToolCalls[0].ID {
			message.OK = true
		}
	}
}

// CleanupReverse - make the messages clean, remove the mistake message
func CleanupReverse(messages []Message) (cleaned []Message) {
	// The messages is in reverse order, say
	// [{id=5},{id=4},{id=3},{id=1},{id=0}]
	// We need to find the tool message and check whether
	// the next is assistant message and the tool is is same with the tool's
	// The cleanup here only handle the one tool call situation
	l := len(messages)
	cleaned = make([]Message, l)
	k := l
	tms := []Message{}
	flag := false
outloop:
	for _, m := range messages {
		if m.Role == "tool" {
			if !flag {
				flag = true
			}
		}
		if flag && m.Role != "assistant" { // 把非assistant消息都加进来
			tms = append(tms, m)
		}

		if flag && m.Role == "assistant" {
			toolCalls := m.ToolCalls
			if len(toolCalls) != len(tms) { // skill all the messages in tms
				flag = false
				continue
			}
			if len(tms) > 1 { // reverse tms
				slices.Reverse(tms)
			}
			for i, tm := range tms {
				if tm.ToolCallID != toolCalls[i].ID {
					flag = false
					continue outloop
				}
			}
			size := len(tms) + 1
			begin := k - size
			cleaned[begin] = m
			for i, tm := range tms {
				cleaned[begin+i+1] = tm
			}
			tms = []Message{}
			k = begin
			if flag {
				flag = false
			}
			continue
		}

		if !flag {
			k--
			cleaned[k] = m
		}
	}
	return cleaned[k:]
}

// compressHistory removes intermediate tool-call messages, keeping only
// user messages and assistant messages without tool_calls. This produces
// a compressed view of the conversation where each turn is represented
// as (user, assistant-with-content) without the tool internals.
// Only safe to call when the last message is an assistant response
// without pending tool calls (i.e. conversation is at rest).
func compressHistory(messages []Message) []Message {
	var result []Message
	for _, m := range messages {
		if m.Role == "user" || (m.Role == "assistant" && len(m.ToolCalls) == 0) {
			result = append(result, m)
		}
	}
	return result
}

// MoveMessages moves all messages from the current session to the target session.
func MoveMessages(ctx context.Context, targetSessionID int64) error {
	span, ctx := clog.StartSpanFromContext(ctx, "MoveMessages")
	defer span.Finish()
	currentSessionID := GetCurrentSessionID(ctx)
	if currentSessionID == targetSessionID {
		return fmt.Errorf("目标项目与当前项目相同，无需移动")
	}

	db, err := sqlite.OpenDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close(ctx)

	// Verify target session exists
	var sid int64
	if err := db.QueryRow("SELECT id FROM sessions WHERE id = ?", targetSessionID).Scan(&sid); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("项目 %d 不存在，请用 dscli project list 查看可用项目", targetSessionID)
		}
		return err
	}

	res, err := db.ExecContext(ctx,
		`UPDATE messages SET session_id = ? WHERE session_id = ?`,
		targetSessionID, currentSessionID)
	if err != nil {
		return fmt.Errorf("移动消息失败: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if affected == 0 {
		return fmt.Errorf("当前项目没有需要移动的消息")
	}

	return nil
}
