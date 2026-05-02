package history

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/sqlite"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

type (
	Message  = toolcall.Message
	ToolCall = toolcall.ToolCall
)

var GetCurrentSessionID = toolcall.GetCurrentSessionID

// UpdateContent update message content
func UpdateContent(ctx context.Context, id int64, content string) (err error) {
	db, err := sqlite.OpenDB()
	if err != nil {
		return
	}
	defer db.Close()
	res, err := db.ExecContext(ctx,
		`UPDATE messages SET content = ? WHERE id = ?`,
		content, id)
	if err != nil {
		return
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return
	}

	if affected != 1 {
		err = fmt.Errorf("failed to update message content")
	}
	return
}

func ToSQLNullString(tcs []toolcall.ToolCall) (toolCalls sql.NullString) {
	data, err := outfmt.JSONMarshal(tcs)
	if err != nil {
		return
	}
	toolCalls.String = string(data)
	toolCalls.Valid = true
	return
}

// UpdateToolCalls update message content
func UpdateToolCalls(ctx context.Context, id int64, tcs []ToolCall) (err error) {
	db, err := sqlite.OpenDB()
	if err != nil {
		return
	}
	defer db.Close()
	toolCalls := ToSQLNullString(tcs)
	res, err := db.ExecContext(ctx,
		`UPDATE messages SET tool_calls = ? WHERE id = ?`,
		&toolCalls, id)
	if err != nil {
		return
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return
	}

	if affected != 1 {
		err = fmt.Errorf("failed to update message content")
	}
	return
}

// UpdateHistory update message session_id to 0
func UpdateHistory(ctx context.Context, id int64) (err error) {
	db, err := sqlite.OpenDB()
	if err != nil {
		return
	}
	defer db.Close()
	_, err = db.ExecContext(ctx,
		`UPDATE messages SET session_id = 0 WHERE id = ?`,
		id)
	if err != nil {
		return
	}
	return
}

func ShowMessage(ctx context.Context, id int64) (message *Message, err error) {
	db, err := sqlite.OpenDB()
	if err != nil {
		return
	}
	defer db.Close()
	var toolCalls sql.NullString
	var toolCallID sql.NullString
	message = &Message{}
	err = db.QueryRowContext(ctx, `SELECT id, session_id, role, content, tool_call_id, `+
		`tool_calls, created_at, model_id, reasoning_content FROM messages WHERE `+
		`id = ?`, id).Scan(&message.ID,
		&message.SessionID, &message.Role, &message.Content, &toolCallID,
		&toolCalls, &message.CreatedAt, &message.ModelID, &message.ReasoningContent)
	if err != nil {
		return
	}
	if toolCalls.Valid {
		err = json.Unmarshal([]byte(toolCalls.String), &message.ToolCalls)
		if err != nil {
			return
		}
	}
	if toolCallID.Valid {
		message.ToolCallID = toolCallID.String
	}
	return message, nil
}

// ListHistory 加载指定会话的所有历史消息，按时间升序返回
func ListHistory(ctx context.Context) ([]*Message, error) {
	sessionID := GetCurrentSessionID(ctx)
	modelID := context.ContextValue(ctx, context.CurrentModelIDKey, context.DeepseekChat)
	histSize := context.ContextValue(ctx, context.HistSizeKey, 8)
	db, err := sqlite.OpenDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT id, role, content, tool_call_id, tool_calls, created_at,reasoning_content
		FROM messages
		WHERE session_id = ? AND model_id = ?
		ORDER BY id DESC
        LIMIT ?`, sessionID, modelID, histSize+2)
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
		var toolCallID, toolCalls sql.NullString
		if err := rows.Scan(&m.ID, &m.Role, &m.Content, &toolCallID, &toolCalls, &m.CreatedAt, &m.ReasoningContent); err != nil {
			return nil, fmt.Errorf("扫描消息失败: %w", err)
		}
		if toolCallID.Valid {
			m.ToolCallID = toolCallID.String
		}
		if toolCalls.Valid {
			var toolCallsData []ToolCall
			if err := json.Unmarshal([]byte(toolCalls.String), &toolCallsData); err == nil {
				m.ToolCalls = toolCallsData
			}
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
	histSize := context.ContextValue(ctx, context.HistSizeKey, 8)
	if histSize == 0 {
		return []Message{}, nil
	}

	sessionID := GetCurrentSessionID(ctx)
	modelID := context.ContextValue(ctx, context.CurrentModelIDKey, context.DeepseekChat)
	leftTokens := context.ContextValue(ctx, context.LeftTokensKey, 0)
	db, err := sqlite.OpenDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`
		SELECT id, role, content, tool_call_id, tool_calls, created_at, reasoning_content
		FROM messages
		WHERE session_id = ? AND model_id = ?
		ORDER BY id DESC
        LIMIT ?`, sessionID, modelID, histSize+2) // histSize + 2就可
	// 以，因为主要就是最后两个。注意我们按降低排的序：{100, 99, 98,
	// ...} 最大ID在前面应用LIMIT，总能把最新消息的找出来。但我们提交
	// 给大语言模型时，最新消息要在最后: {...,98, 99, 100}。
	if err != nil {
		return nil, fmt.Errorf("查询历史消息失败: %w", err)
	}

	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var m Message
		var toolCallID, toolCalls sql.NullString
		if err := rows.Scan(&m.ID, &m.Role, &m.Content, &toolCallID, &toolCalls, &m.CreatedAt, &m.ReasoningContent); err != nil {
			return nil, fmt.Errorf("扫描消息失败: %w", err)
		}
		if toolCallID.Valid {
			m.ToolCallID = toolCallID.String
		}
		if toolCalls.Valid {
			var toolCallsData []ToolCall
			if err := json.Unmarshal([]byte(toolCalls.String), &toolCallsData); err == nil {
				m.ToolCalls = toolCallsData
			}
		}
		tokens := m.GetTokens()
		leftTokens -= tokens
		if leftTokens <= tokens*2 { // 我们还要给后面的user消息留下些tokens。
			break
		}

		messages = append(messages, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历消息失败: %w", err)
	}

	// Cleanup
	messages = CleanupReverse(messages)
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
