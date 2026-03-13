package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"time"
	"unicode/utf8"
)

// Message 扩展，支持工具调用（注意：Content 字段不再使用 omitempty）
type Message struct {
	ID               int        `json:"-"`
	Role             string     `json:"role"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	Content          string     `json:"content"`                // 始终输出，即使为空字符串
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`   // 仅当有工具调用时输出
	ToolCallID       string     `json:"tool_call_id,omitempty"` // 仅当 role="tool" 时输出
	CreatedAt        time.Time  `json:"-"`
}

func init() {
	RegisterTableSchema(
		// 消息表
		`CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id INTEGER NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			tool_call_id TEXT,
			tool_calls TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
            model_id INTEGER NOT NULL DEFAULT 0,
    		reasoning_content TEXT,
			FOREIGN KEY (session_id) REFERENCES sessions(id)
		)`,
	)

	RegisterIndexSchema(
		// 创建索引
		`CREATE INDEX IF NOT EXISTS idx_messages_session_id ON messages(session_id)`,
	)

	RegisterUpgradeSchema(
		// 增加 model ID
		`ALTER TABLE messages ADD COLUMN model_id INTEGER NOT NULL DEFAULT 0`,
		// 增加 reasoning content
		`ALTER TABLE messages ADD COLUMN reasoning_content TEXT`,
	)
}

func ToolCallsID(tcs []ToolCall) string {
	if len(tcs) == 0 {
		return ""
	}
	return tcs[0].ID
}

// UpdateHistory

func UpdateHistory(ctx context.Context, id int64) (err error) {
	db, err := OpenDB()
	if err != nil {
		return
	}
	defer db.Close()
	_, err = db.ExecContext(ctx, `UPDATE messages SET session_id = 0 WHERE id = ?`, id)
	if err != nil {
		return
	}
	return
}

// LoadHistory 加载指定会话的所有历史消息，按时间升序返回
func LoadHistory(ctx context.Context) ([]Message, error) {
	sessionID := ContextValue(ctx, CurrentSessionID, int64(0))
	modelID := ContextValue(ctx, CurrentModelID, DeepseekChat)
	histSize := ContextValue(ctx, HistSize, 8)
	db, err := OpenDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`
		SELECT id, role, content, tool_call_id, tool_calls, created_at
		FROM messages
		WHERE session_id = ? AND model_id = ?
		ORDER BY id DESC
        LIMIT ?`, sessionID, modelID, histSize*2)
	if err != nil {
		return nil, fmt.Errorf("查询历史消息失败: %w", err)
	}
	defer rows.Close()

	var messages []Message
	tokens := 0
	for rows.Next() {
		var m Message
		var toolCallID, toolCalls sql.NullString
		if err := rows.Scan(&m.ID, &m.Role, &m.Content, &toolCallID, &toolCalls, &m.CreatedAt); err != nil {
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

		tokens += utf8.RuneCountInString(m.Content) / 2
		if tokens > 131072 {
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
		if flag && m.Role != "assistant" {
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

// SaveMessages 保存消息（事务）
func SaveMessages(ctx context.Context, msgs ...Message) error {
	sessionID := ContextValue(ctx, CurrentSessionID, int64(0))
	modelID := ContextValue(ctx, CurrentModelID, DeepseekChat)
	db, err := OpenDB()
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("开始事务失败: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO messages (session_id, role, content, tool_call_id, tool_calls, model_id, reasoning_content)
		VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("准备语句失败: %w", err)
	}
	defer stmt.Close()

	for _, m := range msgs {
		var toolCallID, toolCalls sql.NullString
		if m.ToolCallID != "" {
			toolCallID.String = m.ToolCallID
			toolCallID.Valid = true
		}
		if len(m.ToolCalls) > 0 {
			var data json.RawMessage
			data, err = JSONMarshal(&m.ToolCalls)
			if err != nil {
				return err
			}
			toolCalls.String = string(data)
			toolCalls.Valid = true
		}
		if _, err := stmt.Exec(sessionID, m.Role, m.Content, toolCallID, toolCalls, modelID, m.ReasoningContent); err != nil {
			return fmt.Errorf("插入消息失败: %w", err)
		}
	}

	// 更新会话的更新时间
	if _, err := tx.Exec("UPDATE sessions SET updated_at = CURRENT_TIMESTAMP WHERE id = ?", sessionID); err != nil {
		return fmt.Errorf("更新会话时间失败: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	return nil
}
