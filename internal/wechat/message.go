package wechat

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/eatmoreapple/openwechat"
)

// Message 微信消息结构
type Message struct {
	ID        string    `json:"id"`
	WxMsgID   string    `json:"wx_msg_id"`
	Direction string    `json:"direction"` // "incoming" 或 "outgoing"
	From      string    `json:"from"`
	To        string    `json:"to"`
	Content   string    `json:"content"`
	Type      string    `json:"type"`   // "text", "image", "voice", etc.
	Status    string    `json:"status"` // "unread", "read", "replied", "archived"
	CreatedAt time.Time `json:"created_at"`
	RepliedAt time.Time `json:"replied_at"`
}

// MessageManager 消息管理器
type MessageManager struct {
	db  *sql.DB
	bot *Bot
}

// NewMessageManager 创建消息管理器
func NewMessageManager(db *sql.DB, bot *Bot) *MessageManager {
	return &MessageManager{
		db:  db,
		bot: bot,
	}
}

// SaveMessage 保存消息到数据库
func (m *MessageManager) SaveMessage(ctx context.Context, msg *Message) error {
	if err := ctx.Err(); err != nil {
		return wrapErr(err, "上下文已取消")
	}

	// 获取当前会话ID
	var sessionID int64
	err := m.db.QueryRowContext(ctx, `
		SELECT id FROM wechat_sessions 
		WHERE account = ? AND is_active = 1 
		ORDER BY updated_at DESC LIMIT 1
	`, m.bot.config.Account).Scan(&sessionID)
	if err != nil {
		return wrapErr(err, "获取会话ID失败")
	}

	_, err = m.db.ExecContext(ctx, `
		INSERT INTO wechat_messages 
		(session_id, wx_msg_id, direction, from_user, to_user, content, msg_type, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, sessionID, msg.WxMsgID, msg.Direction, msg.From, msg.To, msg.Content, msg.Type, msg.Status, msg.CreatedAt)
	if err != nil {
		return wrapErr(err, "保存消息失败")
	}

	return nil
}

// GetUnreadMessages 获取未读消息
func (m *MessageManager) GetUnreadMessages(ctx context.Context, limit int) ([]Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, wrapErr(err, "上下文已取消")
	}

	var sessionID int64
	err := m.db.QueryRowContext(ctx, `
		SELECT id FROM wechat_sessions 
		WHERE account = ? AND is_active = 1 
		ORDER BY updated_at DESC LIMIT 1
	`, m.bot.config.Account).Scan(&sessionID)

	if err == sql.ErrNoRows {
		return []Message{}, nil
	}
	if err != nil {
		return nil, wrapErr(err, "获取会话ID失败")
	}

	rows, err := m.db.QueryContext(ctx, `
		SELECT id, wx_msg_id, direction, from_user, to_user, content, msg_type, status, created_at
		FROM wechat_messages
		WHERE session_id = ? AND status = 'unread'
		ORDER BY created_at DESC
		LIMIT ?
	`, sessionID, limit)
	if err != nil {
		return nil, wrapErr(err, "查询未读消息失败")
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var msg Message
		var dbID int64
		err := rows.Scan(&dbID, &msg.WxMsgID, &msg.Direction, &msg.From, &msg.To,
			&msg.Content, &msg.Type, &msg.Status, &msg.CreatedAt)
		if err != nil {
			return nil, wrapErr(err, "扫描消息数据失败")
		}
		msg.ID = fmt.Sprintf("%d", dbID)
		messages = append(messages, msg)
	}

	return messages, nil
}

// GetRecentMessages 获取最近消息
func (m *MessageManager) GetRecentMessages(ctx context.Context, limit int) ([]Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, wrapErr(err, "上下文已取消")
	}

	var sessionID int64
	err := m.db.QueryRowContext(ctx, `
		SELECT id FROM wechat_sessions 
		WHERE account = ? AND is_active = 1 
		ORDER BY updated_at DESC LIMIT 1
	`, m.bot.config.Account).Scan(&sessionID)

	if err == sql.ErrNoRows {
		return []Message{}, nil
	}
	if err != nil {
		return nil, wrapErr(err, "获取会话ID失败")
	}

	rows, err := m.db.QueryContext(ctx, `
		SELECT id, wx_msg_id, direction, from_user, to_user, content, msg_type, status, created_at
		FROM wechat_messages
		WHERE session_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, sessionID, limit)
	if err != nil {
		return nil, wrapErr(err, "查询最近消息失败")
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var msg Message
		var dbID int64
		err := rows.Scan(&dbID, &msg.WxMsgID, &msg.Direction, &msg.From, &msg.To,
			&msg.Content, &msg.Type, &msg.Status, &msg.CreatedAt)
		if err != nil {
			return nil, wrapErr(err, "扫描消息数据失败")
		}
		msg.ID = fmt.Sprintf("%d", dbID)
		messages = append(messages, msg)
	}

	return messages, nil
}

// MarkAsRead 标记消息为已读
func (m *MessageManager) MarkAsRead(ctx context.Context, messageID string) error {
	if err := ctx.Err(); err != nil {
		return wrapErr(err, "上下文已取消")
	}

	_, err := m.db.ExecContext(ctx, `
		UPDATE wechat_messages 
		SET status = 'read'
		WHERE id = ?
	`, messageID)
	if err != nil {
		return wrapErr(err, "标记消息为已读失败")
	}

	return nil
}

// MarkAsReplied 标记消息为已回复
func (m *MessageManager) MarkAsReplied(ctx context.Context, messageID, replyContent string) error {
	if err := ctx.Err(); err != nil {
		return wrapErr(err, "上下文已取消")
	}

	_, err := m.db.ExecContext(ctx, `
		UPDATE wechat_messages 
		SET status = 'replied', reply_content = ?, replied_at = ?
		WHERE id = ?
	`, replyContent, time.Now(), messageID)
	if err != nil {
		return wrapErr(err, "标记消息为已回复失败")
	}

	return nil
}

// GetMessageByID 根据ID获取消息
func (m *MessageManager) GetMessageByID(ctx context.Context, messageID string) (*Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, wrapErr(err, "上下文已取消")
	}

	var msg Message
	var dbID int64

	err := m.db.QueryRowContext(ctx, `
		SELECT id, wx_msg_id, direction, from_user, to_user, content, msg_type, status, created_at
		FROM wechat_messages
		WHERE id = ?
	`, messageID).Scan(&dbID, &msg.WxMsgID, &msg.Direction, &msg.From, &msg.To,
		&msg.Content, &msg.Type, &msg.Status, &msg.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, ErrMessageNotFound
	}
	if err != nil {
		return nil, wrapErr(err, "查询消息失败")
	}

	msg.ID = fmt.Sprintf("%d", dbID)
	return &msg, nil
}

// ConvertOpenWechatMessage 转换openwechat消息到内部格式
func ConvertOpenWechatMessage(ctx context.Context, wxMsg *openwechat.Message, direction string) (*Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, wrapErr(err, "上下文已取消")
	}

	msg := &Message{
		WxMsgID:   wxMsg.MsgId, // MsgId是字符串类型
		Direction: direction,
		CreatedAt: time.Now(),
		Status:    "unread",
	}

	// 获取发送者信息
	if sender, err := wxMsg.Sender(); err == nil {
		msg.From = sender.NickName
	}

	// 获取接收者信息
	if receiver, err := wxMsg.Receiver(); err == nil {
		msg.To = receiver.NickName
	}

	// 设置消息类型和内容
	if wxMsg.IsText() {
		msg.Type = "text"
		msg.Content = wxMsg.Content
	} else if wxMsg.IsPicture() {
		msg.Type = "image"
		msg.Content = "[图片消息]"
	} else if wxMsg.IsVoice() {
		msg.Type = "voice"
		msg.Content = "[语音消息]"
	} else if wxMsg.IsVideo() {
		msg.Type = "video"
		msg.Content = "[视频消息]"
	} else if wxMsg.IsCard() {
		msg.Type = "card"
		msg.Content = "[名片消息]"
	} else {
		msg.Type = "unknown"
		msg.Content = wxMsg.Content
	}

	return msg, nil
}
