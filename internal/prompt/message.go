package prompt

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/session"
	"github.com/dscli/dscli/internal/sqlite"
	"github.com/dscli/dscli/internal/tokenizer"
)

// Message 扩展，支持工具调用（注意：Content 字段不再使用 omitempty）
type Message struct {
	ID               int64      `json:"-"`
	SessionID        int64      `json:"-"`
	ModelID          int64      `json:"-"`
	Content          string     `json:"content"` // 始终输出，即使为空字符串
	Role             string     `json:"role"`
	ReasoningContent string     `json:"reasoning_content,omitzero"`
	ToolCalls        []ToolCall `json:"tool_calls,omitzero"`   // 仅当有工具调用时输出
	ToolCallID       string     `json:"tool_call_id,omitzero"` // 仅当 role="tool" 时输出
	CreatedAt        time.Time  `json:"-"`
	tokens           int        `json:"-"`
	OK               bool       `json:"-"`
}

type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON 字符串
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

func (m *Message) SetTokens(tokens int) {
	m.tokens = tokens
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
			tokens INTEGER NOT NULL DEFAULT 0,
			FOREIGN KEY (session_id) REFERENCES sessions(id)
		)`,
		// FTS5 全文搜索虚拟表（独立维护，与 memories_fts 模式一致）
		`CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
			content
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
		// 增加 tokens
		`ALTER TABLE messages ADD COLUMN tokens INTEGER NOT NULL DEFAULT 0`,
	)

	// 升级迁移：为已有消息重建 FTS 索引（仅当 FTS 表为空且有消息时执行一次）
	sqlite.RegisterPostInitHook(populateMessagesFTS)
}

func ToolCallsID(tcs []ToolCall) string {
	if len(tcs) == 0 {
		return ""
	}
	return tcs[0].ID
}

// SaveMessages 保存消息，同时同步 FTS5 全文索引。
func SaveMessages(ctx context.Context, msgs ...Message) error {
	sessionID := session.GetCurrentSessionID(ctx)
	modelID := context.ContextValue(ctx, context.CurrentModelIDKey, context.DeepseekChat)

	for _, m := range msgs {
		if err := saveMessage(sessionID, modelID, m); err != nil {
			return err
		}
	}
	return nil
}

// saveMessage 保存单条消息及其 FTS 索引。
// 分词在 DB 操作之前完成，避免占用 DB 锁。
func saveMessage(sessionID, modelID int64, m Message) error {
	// 只对用户消息分词建索引（recall 检索目标就是用户消息）
	var tokens string
	if m.Role == "user" {
		tokens = tokenizer.Tokenize(m.Content)
	}

	id, err := insertMessage(sessionID, modelID, m)
	if err != nil {
		return err
	}
	if tokens != "" {
		if err := insertMessageFTS(id, tokens); err != nil {
			return err
		}
	}
	return nil
}

// insertMessage 插入一条消息到 messages 表，返回自动生成的 ID。
func insertMessage(sessionID, modelID int64, m Message) (int64, error) {
	var toolCallID, toolCalls sql.NullString
	if m.ToolCallID != "" {
		toolCallID.String = m.ToolCallID
		toolCallID.Valid = true
	}
	if len(m.ToolCalls) > 0 {
		data, err := outfmt.JSONMarshal(&m.ToolCalls)
		if err != nil {
			return 0, err
		}
		toolCalls.String = string(data)
		toolCalls.Valid = true
	}

	db, err := sqlite.OpenDB()
	if err != nil {
		return 0, err
	}
	defer db.Close()

	res, err := db.Exec(
		`INSERT INTO messages (session_id, role, content, tool_call_id, tool_calls, model_id, reasoning_content, tokens)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		sessionID, m.Role, m.Content, toolCallID, toolCalls, modelID, m.ReasoningContent, m.tokens,
	)
	if err != nil {
		return 0, fmt.Errorf("插入消息失败: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("获取消息ID失败: %w", err)
	}
	return id, nil
}

// insertMessageFTS 为指定消息建立 FTS5 全文索引（仅 content，不含 reasoning_content）。
// tokens 应为预分词结果（由 tokenizer.Tokenize 生成）。
func insertMessageFTS(id int64, tokens string) error {
	db, err := sqlite.OpenDB()
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = db.Exec(
		`INSERT INTO messages_fts(rowid, content) VALUES (?, ?)`,
		id, tokens,
	)
	if err != nil {
		return fmt.Errorf("创建全文索引失败: %w", err)
	}
	return nil
}

// populateMessagesFTS 是升级迁移钩子：当 messages 表有用户消息但 messages_fts 为空时，
// 为已有的用户消息重建 FTS5 全文索引（仅执行一次，仅索引 content，不含 reasoning_content）。
func populateMessagesFTS(db *sqlite.DB) error {
	// 检查 FTS 表是否已有数据（已迁移过则跳过）
	var ftsCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM messages_fts").Scan(&ftsCount); err != nil {
		return fmt.Errorf("populateMessagesFTS: 检查 FTS 表失败: %w", err)
	}
	if ftsCount > 0 {
		return nil // 已迁移，跳过
	}

	// 只迁移用户消息（recall 只检索用户消息，assistant/tool 无需索引）
	rows, err := db.Query("SELECT id, content FROM messages WHERE role = 'user'")
	if err != nil {
		return fmt.Errorf("populateMessagesFTS: 查询消息失败: %w", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var id int64
		var content string
		if err := rows.Scan(&id, &content); err != nil {
			return fmt.Errorf("populateMessagesFTS: 扫描消息失败: %w", err)
		}
		if _, err := db.Exec(
			`INSERT INTO messages_fts(rowid, content) VALUES (?, ?)`,
			id, tokenizer.Tokenize(content),
		); err != nil {
			return fmt.Errorf("populateMessagesFTS: 插入 FTS 失败 (id=%d): %w", id, err)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("populateMessagesFTS: 遍历消息失败: %w", err)
	}

	if count > 0 {
		outfmt.Debug("populateMessagesFTS: 已为 %d 条已有消息重建 FTS 索引\n", count)
	}
	return nil
}
