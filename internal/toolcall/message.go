package toolcall

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/sqlite"
)

const (
	DeepseekChat     = int64(0)
	DeepseekReasoner = int64(1)
)

var (
	ModelDeepseekChat     = context.Getenv("MODEL_DEEPSEEK_CHAT", "deepseek-chat")
	ModelDeepseekReasoner = context.Getenv("MODEL_DEEPSEEK_REASONER", "deepseek-reasoner")
)

// Message 扩展，支持工具调用（注意：Content 字段不再使用 omitempty）
type Message struct {
	ID               int64      `json:"-"`
	SessionID        int64      `json:"-"`
	ModelID          int64      `json:"-"`
	Role             string     `json:"role"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	Content          string     `json:"content"`                // 始终输出，即使为空字符串
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`   // 仅当有工具调用时输出
	ToolCallID       string     `json:"tool_call_id,omitempty"` // 仅当 role="tool" 时输出
	CreatedAt        time.Time  `json:"-"`
	tokens           int        `json:"-"`
	OK               bool       `json:"-"`
}

func (m *Message) GetTokens() int {
	if m.tokens != 0 {
		return m.tokens
	}
	b, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}

	m.tokens = len([]rune(string(b))) / 2
	return m.tokens
}

func init() {
	sqlite.RegisterTableSchema(
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

	sqlite.RegisterIndexSchema(
		// 创建索引
		`CREATE INDEX IF NOT EXISTS idx_messages_session_id ON messages(session_id)`,
	)

	sqlite.RegisterUpgradeSchema(
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

// UpdateContent update message content
func UpdateContent(ctx context.Context, id int64, content string) (err error) {
	sessionID := context.ContextValue(ctx, context.CurrentSessionIDKey, int64(0))
	modelID := context.ContextValue(ctx, context.CurrentModelIDKey, DeepseekChat)
	db, err := sqlite.OpenDB()
	if err != nil {
		return
	}
	defer db.Close()
	res, err := db.ExecContext(ctx,
		`UPDATE messages SET content = ? WHERE id = ? AND session_id = ? AND model_id = ?`,
		content, id, sessionID, modelID)
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

func ToSQLNullString(tcs []ToolCall) (toolCalls sql.NullString) {
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
	sessionID := context.ContextValue(ctx, context.CurrentSessionIDKey, int64(0))
	modelID := context.ContextValue(ctx, context.CurrentModelIDKey, DeepseekChat)
	db, err := sqlite.OpenDB()
	if err != nil {
		return
	}
	defer db.Close()
	toolCalls := ToSQLNullString(tcs)
	res, err := db.ExecContext(ctx,
		`UPDATE messages SET tool_calls = ? WHERE id = ? AND session_id = ? AND model_id = ?`,
		&toolCalls, id, sessionID, modelID)
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
	sessionID := context.ContextValue(ctx, context.CurrentSessionIDKey, int64(0))
	modelID := context.ContextValue(ctx, context.CurrentModelIDKey, DeepseekChat)
	db, err := sqlite.OpenDB()
	if err != nil {
		return
	}
	defer db.Close()
	_, err = db.ExecContext(ctx,
		`UPDATE messages SET session_id = 0 WHERE id = ? AND session_id = ? and model_id = ?`,
		id, sessionID, modelID)
	if err != nil {
		return
	}
	return
}

func ShowMessage(ctx context.Context, id int64) (message *Message, err error) {
	sessionID := context.ContextValue(ctx, context.CurrentSessionIDKey, int64(0))
	modelID := context.ContextValue(ctx, context.CurrentModelIDKey, DeepseekChat)
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
		`session_id = ? AND model_id = ? AND id = ?`, sessionID, modelID, id).Scan(&message.ID,
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
	sessionID := context.ContextValue(ctx, context.CurrentSessionIDKey, int64(0))
	modelID := context.ContextValue(ctx, context.CurrentModelIDKey, DeepseekChat)
	histSize := context.ContextValue(ctx, context.HistSizeKey, 8)
	db, err := sqlite.OpenDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`
		SELECT id, role, content, tool_call_id, tool_calls, created_at
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

	var messages []*Message
	for rows.Next() {
		m := &Message{}
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
	sessionID := context.ContextValue(ctx, context.CurrentSessionIDKey, int64(0))
	modelID := context.ContextValue(ctx, context.CurrentModelIDKey, DeepseekChat)
	histSize := context.ContextValue(ctx, context.HistSizeKey, 8)
	leftTokens := context.ContextValue(ctx, context.LeftTokensKey, 0)
	db, err := sqlite.OpenDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`
		SELECT id, role, content, tool_call_id, tool_calls, created_at
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

// SaveMessages 保存消息（事务）
func SaveMessages(ctx context.Context, msgs ...Message) error {
	sessionID := context.ContextValue(ctx, context.CurrentSessionIDKey, int64(0))
	modelID := context.ContextValue(ctx, context.CurrentModelIDKey, DeepseekChat)
	db, err := sqlite.OpenDB()
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
			data, err = outfmt.JSONMarshal(&m.ToolCalls)
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
