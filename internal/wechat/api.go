package wechat

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Client 微信客户端API
type Client struct {
	config *Config
	bot    *Bot
	db     *sql.DB
	msgMgr *MessageManager
}

// NewClient 创建新的微信客户端
func NewClient(config *Config) (*Client, error) {
	// 确保数据库目录存在
	dbDir := filepath.Dir(config.DBPath)
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		return nil, wrapErr(err, "创建数据库目录失败")
	}

	// 打开数据库
	db, err := sql.Open("sqlite3", config.DBPath+"?_journal=WAL&_timeout=5000&_fk=1")
	if err != nil {
		return nil, wrapErr(err, "打开数据库失败")
	}

	// 创建Bot
	bot, err := NewBot(config)
	if err != nil {
		db.Close()
		return nil, wrapErr(err, "创建微信机器人失败")
	}

	// 创建消息管理器
	msgMgr := NewMessageManager(db, bot)

	return &Client{
		config: config,
		bot:    bot,
		db:     db,
		msgMgr: msgMgr,
	}, nil
}

// NewClientFromConfig 从默认配置文件创建客户端
func NewClientFromConfig() (*Client, error) {
	configPath := GetDefaultConfigPath()
	config, err := LoadConfig(configPath)
	if err != nil {
		// 如果配置文件不存在，使用默认配置
		if os.IsNotExist(err) {
			config = DefaultConfig()
			// 保存默认配置
			if err := SaveConfig(config, configPath); err != nil {
				return nil, wrapErr(err, "保存默认配置失败")
			}
		} else {
			return nil, wrapErr(err, "加载配置失败")
		}
	}

	return NewClient(config)
}

// Login 登录微信
func (c *Client) Login(ctx context.Context) error {
	if c.bot.IsLoggedIn() {
		return fmt.Errorf("已经登录")
	}

	return c.bot.SmartLogin(ctx)
}

// Logout 退出登录
func (c *Client) Logout(ctx context.Context) error {
	if !c.bot.IsLoggedIn() {
		return fmt.Errorf("未登录")
	}

	return c.bot.Logout(ctx)
}

// Status 获取状态
func (c *Client) Status(ctx context.Context) (map[string]any, error) {
	status := make(map[string]any)

	status["logged_in"] = c.bot.IsLoggedIn()

	if c.bot.IsLoggedIn() {
		if self, err := c.bot.GetCurrentUser(ctx); err == nil {
			status["user"] = self.NickName
			status["username"] = self.UserName
		}
	}

	status["account"] = c.config.Account
	status["mode"] = c.config.Mode

	// 获取未读消息数量
	if c.bot.IsLoggedIn() {
		messages, err := c.msgMgr.GetUnreadMessages(ctx, 1000)
		if err == nil {
			status["unread_count"] = len(messages)
		}
	}

	return status, nil
}

// GetMessages 获取消息
func (c *Client) GetMessages(ctx context.Context, unread bool, limit int) ([]Message, error) {
	if !c.bot.IsLoggedIn() {
		return nil, ErrNotLoggedIn
	}

	if unread {
		return c.msgMgr.GetUnreadMessages(ctx, limit)
	}
	return c.msgMgr.GetRecentMessages(ctx, limit)
}

// GetMessage 获取单条消息
func (c *Client) GetMessage(ctx context.Context, messageID string) (*Message, error) {
	if !c.bot.IsLoggedIn() {
		return nil, ErrNotLoggedIn
	}

	return c.msgMgr.GetMessageByID(ctx, messageID)
}

// SendMessage 发送消息
func (c *Client) SendMessage(ctx context.Context, to, content string) error {
	if !c.bot.IsLoggedIn() {
		return ErrNotLoggedIn
	}

	// 检查消息长度
	if len(content) > c.config.MaxMsgLength {
		return wrapErr(ErrMessageTooLong,
			fmt.Sprintf("消息长度超过限制: %d > %d", len(content), c.config.MaxMsgLength))
	}

	// 发送消息
	if err := c.bot.SendText(ctx, to, content); err != nil {
		return wrapErr(err, "发送消息失败")
	}

	// 保存发送的消息记录
	msg := &Message{
		WxMsgID:   "", // 微信消息ID会在发送后返回
		Direction: "outgoing",
		From:      "", // 会在发送时设置
		To:        to,
		Content:   content,
		Type:      "text",
		Status:    "sent",
		CreatedAt: time.Now(),
	}

	if err := c.msgMgr.SaveMessage(ctx, msg); err != nil {
		// 记录错误但不影响发送
		fmt.Fprintf(os.Stderr, "警告: 保存消息记录失败: %v\n", err)
	}

	return nil
}

// ReplyMessage 回复消息
func (c *Client) ReplyMessage(ctx context.Context, messageID, content string) error {
	if !c.bot.IsLoggedIn() {
		return ErrNotLoggedIn
	}

	// 获取原消息
	msg, err := c.msgMgr.GetMessageByID(ctx, messageID)
	if err != nil {
		return wrapErr(err, "获取原消息失败")
	}

	// 发送回复
	if err := c.SendMessage(ctx, msg.From, content); err != nil {
		return wrapErr(err, "发送回复失败")
	}

	// 标记为已回复
	if err := c.msgMgr.MarkAsReplied(ctx, messageID, content); err != nil {
		return wrapErr(err, "标记消息为已回复失败")
	}

	return nil
}

// MarkAsRead 标记消息为已读
func (c *Client) MarkAsRead(ctx context.Context, messageID string) error {
	if !c.bot.IsLoggedIn() {
		return ErrNotLoggedIn
	}

	return c.msgMgr.MarkAsRead(ctx, messageID)
}

// GetFriends 获取好友列表
func (c *Client) GetFriends(ctx context.Context) ([]map[string]string, error) {
	if !c.bot.IsLoggedIn() {
		return nil, ErrNotLoggedIn
	}

	wxFriends, err := c.bot.GetFriends(ctx, false)
	if err != nil {
		return nil, wrapErr(err, "获取好友列表失败")
	}

	var friends []map[string]string
	for _, f := range wxFriends {
		friend := map[string]string{
			"nickname": f.NickName,
			"username": f.UserName,
			"remark":   f.RemarkName,
		}
		friends = append(friends, friend)
	}

	return friends, nil
}

// GetGroups 获取群组列表
func (c *Client) GetGroups(ctx context.Context) ([]map[string]string, error) {
	if !c.bot.IsLoggedIn() {
		return nil, ErrNotLoggedIn
	}

	wxGroups, err := c.bot.GetGroups(ctx, false)
	if err != nil {
		return nil, wrapErr(err, "获取群组列表失败")
	}

	var groups []map[string]string
	for _, g := range wxGroups {
		group := map[string]string{
			"nickname": g.NickName,
			"username": g.UserName,
			"remark":   g.RemarkName,
		}
		groups = append(groups, group)
	}

	return groups, nil
}

// Close 关闭客户端
func (c *Client) Close() error {
	var errs []error

	if c.bot != nil {
		if err := c.bot.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if c.db != nil {
		if err := c.db.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("关闭客户端时发生错误: %v", errs)
	}

	return nil
}
