package prompt

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/session"
	"github.com/dscli/dscli/internal/sqlite"
	"github.com/dscli/dscli/internal/tokenizer"
	"github.com/nanjj/clog"
)

// Message 扩展，支持工具调用（注意：Content 字段不再使用 omitempty）
type Message struct {
	ID               int64          `json:"-"`
	SessionID        int64          `json:"-"`
	ModelID          int64          `json:"-"`
	ConversationID   string         `json:"-"`       // API 响应 ID（与 WebChat 会话 ID 同源），仅存库不序列化
	Content          string         `json:"content"` // 纯文本（显示/FTS/存储用），始终输出
	ContentBlocks    []ContentBlock `json:"-"`       // 图片等块内容（有值时 content 序列化为块数组）
	Role             string         `json:"role"`
	ReasoningContent string         `json:"reasoning_content,omitzero"`
	ToolCalls        []ToolCall     `json:"tool_calls,omitzero"`   // 仅当有工具调用时输出
	ToolCallID       string         `json:"tool_call_id,omitzero"` // 仅当 role="tool" 时输出
	CreatedAt        time.Time      `json:"-"`
	tokens           int            `json:"-"`
	OK               bool           `json:"-"` // 双语义：history 配对完整性（JudgeHistory/CleanupReverse 重算）或 DSML 格式严格合规（dsml.ParseDSMLMessage 判定）；无调用且无违规亦为 true（ParseDSMLMessage）；均不序列化不落库
}

// MarshalJSON 输出 content 为字符串（无图片）或块数组（有图片）。
// 块数组用 ContentBlock 的固定字段顺序序列化，保证 KV 缓存前缀稳定。
func (m Message) MarshalJSON() ([]byte, error) {
	if len(m.ContentBlocks) == 0 {
		type plain Message
		return json.Marshal(plain(m))
	}
	return json.Marshal(struct {
		Content          any        `json:"content"`
		Role             string     `json:"role"`
		ReasoningContent string     `json:"reasoning_content,omitzero"`
		ToolCalls        []ToolCall `json:"tool_calls,omitzero"`
		ToolCallID       string     `json:"tool_call_id,omitzero"`
	}{
		Content:          m.ContentBlocks,
		Role:             m.Role,
		ReasoningContent: m.ReasoningContent,
		ToolCalls:        m.ToolCalls,
		ToolCallID:       m.ToolCallID,
	})
}

// UnmarshalJSON 兼容 content 的两种形态：字符串或块数组。
// 块数组时提取 text 块拼接为 Content（纯文本），便于显示与检索。
func (m *Message) UnmarshalJSON(data []byte) error {
	var wire struct {
		Content          json.RawMessage `json:"content"`
		Role             string          `json:"role"`
		ReasoningContent string          `json:"reasoning_content"`
		ToolCalls        []ToolCall      `json:"tool_calls"`
		ToolCallID       string          `json:"tool_call_id"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	m.Role = wire.Role
	m.ReasoningContent = wire.ReasoningContent
	m.ToolCalls = wire.ToolCalls
	m.ToolCallID = wire.ToolCallID
	if len(wire.Content) == 0 {
		return nil
	}
	var text string
	if err := json.Unmarshal(wire.Content, &text); err == nil {
		m.Content = text
		return nil
	}
	var blocks []ContentBlock
	if err := json.Unmarshal(wire.Content, &blocks); err != nil {
		return fmt.Errorf("content 既非字符串也非块数组: %w", err)
	}
	m.ContentBlocks = blocks
	var s strings.Builder
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			if s.Len() > 0 {
				s.WriteString("\n")
			}
			s.WriteString(b.Text)
		}
	}
	m.Content = s.String()
	return nil
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
			conversation_id TEXT,
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
		// 增加 content blocks（图片等块内容，JSON 数组；为 NULL 表示纯文本消息）
		`ALTER TABLE messages ADD COLUMN content_blocks TEXT`,
		// 增加 conversation ID（API 响应 ID，与 WebChat 会话 ID 同源）
		`ALTER TABLE messages ADD COLUMN conversation_id TEXT`,
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
	span, ctx := clog.StartSpanFromContext(ctx, "SaveMessages")
	defer span.Finish()
	sessionID := session.GetCurrentSessionID(ctx)
	modelID := context.ContextValue(ctx, context.CurrentModelIDKey, context.DeepseekChat)

	for _, m := range msgs {
		if err := saveMessage(ctx, sessionID, modelID, m); err != nil {
			return err
		}
	}
	return nil
}

// saveMessage 保存单条消息及其 FTS 索引。
// 分词在 DB 操作之前完成，避免占用 DB 锁。
func saveMessage(ctx context.Context, sessionID, modelID int64, m Message) error {
	span, ctx := clog.StartSpanFromContext(ctx, "saveMessage")
	defer span.Finish()

	// 只对用户消息分词建索引（recall 检索目标就是用户消息）
	var tokens string
	if m.Role == "user" {
		tokens = tokenizer.Tokenize(m.Content)
	}

	id, err := insertMessage(ctx, sessionID, modelID, m)
	if err != nil {
		return err
	}
	if tokens != "" {
		if err := insertMessageFTS(ctx, id, tokens); err != nil {
			return err
		}
	}
	return nil
}

// insertMessage 插入一条消息到 messages 表，返回自动生成的 ID。
func insertMessage(ctx context.Context, sessionID, modelID int64, m Message) (int64, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "insertMessage")
	defer span.Finish()

	var toolCallID, toolCalls, contentBlocks sql.NullString
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
	if len(m.ContentBlocks) > 0 {
		data, err := BlocksToJSON(m.ContentBlocks)
		if err != nil {
			return 0, err
		}
		contentBlocks.String = data
		contentBlocks.Valid = true
	}

	db, err := sqlite.OpenDB(ctx)
	if err != nil {
		return 0, err
	}

	defer db.Close(ctx)

	var conversationID sql.NullString
	if m.ConversationID != "" {
		conversationID.String = m.ConversationID
		conversationID.Valid = true
	}

	res, err := db.Exec(
		`INSERT INTO messages (session_id, role, content, tool_call_id, tool_calls, model_id, reasoning_content, tokens, content_blocks, conversation_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sessionID, m.Role, m.Content, toolCallID, toolCalls, modelID, m.ReasoningContent, m.tokens, contentBlocks, conversationID,
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
func insertMessageFTS(ctx context.Context, id int64, tokens string) error {
	span, ctx := clog.StartSpanFromContext(ctx, "insertMessageFTS")
	defer span.Finish()

	db, err := sqlite.OpenDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close(ctx)

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
