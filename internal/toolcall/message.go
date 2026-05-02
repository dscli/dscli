package toolcall

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/sqlite"
)

// Message 扩展，支持工具调用（注意：Content 字段不再使用 omitempty）
type Message struct {
	ID               int64      `json:"-"`
	SessionID        int64      `json:"-"`
	ModelID          int64      `json:"-"`
	Role             string     `json:"role"`
	ReasoningContent string     `json:"reasoning_content"`
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

// SaveMessages 保存消息（事务）
func SaveMessages(ctx context.Context, msgs ...Message) error {
	sessionID := GetCurrentSessionID(ctx)
	modelID := context.ContextValue(ctx, context.CurrentModelIDKey, context.DeepseekChat)
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
			var data []byte
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
