package prompt

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/session"
	"gitcode.com/dscli/dscli/internal/sqlite"
	"gitcode.com/dscli/dscli/internal/tokenizer"
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

// SaveMessages 保存消息（事务），同时同步 FTS5 全文索引。
// 分词在 OpenDB 之前完成，避免占用 DB 锁。
func SaveMessages(ctx context.Context, msgs ...Message) error {
	sessionID := session.GetCurrentSessionID(ctx)
	modelID := context.ContextValue(ctx, context.CurrentModelIDKey, context.DeepseekChat)

	// 预处理：分词在 DB 事务之外完成，减少锁持有时间。
	type item struct {
		msg    Message
		tokens string // 预分词结果，空表示不需要 FTS 索引
	}
	items := make([]item, len(msgs))
	for i, m := range msgs {
		items[i].msg = m
		if m.Role == "user" || (m.Role == "assistant" && len(m.ToolCalls) == 0) {
			items[i].tokens = tokenizer.Tokenize(m.Content)
		}
	}

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

	for _, it := range items {
		id, err := saveMessage(tx, sessionID, modelID, it.msg)
		if err != nil {
			return err
		}
		if it.tokens != "" {
			if err := saveMessageFTS(tx, id, it.tokens); err != nil {
				return err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务失败: %w", err)
	}
	return nil
}

// saveMessage 插入一条消息到 messages 表，返回自动生成的 ID。
func saveMessage(tx *sql.Tx, sessionID, modelID int64, m Message) (int64, error) {
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

	res, err := tx.Exec(
		`INSERT INTO messages (session_id, role, content, tool_call_id, tool_calls, model_id, reasoning_content)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sessionID, m.Role, m.Content, toolCallID, toolCalls, modelID, m.ReasoningContent,
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

// saveMessageFTS 为指定消息建立 FTS5 全文索引（仅 content，不含 reasoning_content）。
// tokens 应为预分词结果（由 tokenizer.Tokenize 生成）。
func saveMessageFTS(tx *sql.Tx, id int64, tokens string) error {
	_, err := tx.Exec(
		`INSERT INTO messages_fts(rowid, content) VALUES (?, ?)`,
		id, tokens,
	)
	if err != nil {
		return fmt.Errorf("创建全文索引失败: %w", err)
	}
	return nil
}

// populateMessagesFTS 是升级迁移钩子：当 messages 表有数据但 messages_fts 为空时，
// 为所有已有消息重建 FTS5 全文索引（仅执行一次，仅索引 content，不含 reasoning_content）。
func populateMessagesFTS(db *sqlite.DB) error {
	// 检查 FTS 表是否已有数据（已迁移过则跳过）
	var ftsCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM messages_fts").Scan(&ftsCount); err != nil {
		return fmt.Errorf("populateMessagesFTS: 检查 FTS 表失败: %w", err)
	}
	if ftsCount > 0 {
		return nil // 已迁移，跳过
	}

	// 检查 messages 表是否有数据
	var msgCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM messages").Scan(&msgCount); err != nil {
		return fmt.Errorf("populateMessagesFTS: 检查 messages 表失败: %w", err)
	}
	if msgCount == 0 {
		return nil // 无消息，无需迁移
	}

	// 逐条迁移：读取 id, content，分词后写入 FTS
	rows, err := db.Query("SELECT id, content FROM messages")
	if err != nil {
		return fmt.Errorf("populateMessagesFTS: 查询消息失败: %w", err)
	}
	defer rows.Close()

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("populateMessagesFTS: 开始事务失败: %w", err)
	}
	defer tx.Rollback()

	count := 0
	for rows.Next() {
		var id int64
		var content string
		if err := rows.Scan(&id, &content); err != nil {
			return fmt.Errorf("populateMessagesFTS: 扫描消息失败: %w", err)
		}
		if _, err := tx.Exec(
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

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("populateMessagesFTS: 提交事务失败: %w", err)
	}

	outfmt.Debug("populateMessagesFTS: 已为 %d 条已有消息重建 FTS 索引\n", count)
	return nil
}