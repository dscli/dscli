package wechat

import (
	"context"
	"fmt"

	"gitcode.com/dscli/dscli/internal/outfmt"
	"github.com/eatmoreapple/openwechat"
)

// Bot 微信机器人封装
type Bot struct {
	bot     *openwechat.Bot
	storage *SQLiteHotReloadStorage
	config  *Config
	self    *openwechat.Self
}

// NewBot 创建新的微信机器人
func NewBot(config *Config) (*Bot, error) {
	if err := config.Validate(); err != nil {
		return nil, wrapErr(err, "配置无效")
	}

	// 创建存储
	storage, err := NewSQLiteHotReloadStorage(config.DBPath, config.Account)
	if err != nil {
		return nil, wrapErr(err, "创建存储失败")
	}

	// 创建openwechat bot
	var bot *openwechat.Bot
	if config.Mode == "desktop" {
		bot = openwechat.DefaultBot(openwechat.Desktop)
	} else {
		bot = openwechat.DefaultBot()
	}

	// 设置二维码回调（控制台打印）
	bot.UUIDCallback = openwechat.PrintlnQrcodeUrl

	return &Bot{
		bot:     bot,
		storage: storage,
		config:  config,
	}, nil
}

// SmartLogin 智能登录：尝试最优登录方式
func (b *Bot) SmartLogin(ctx context.Context) error {
	// 检查上下文是否已取消
	if err := ctx.Err(); err != nil {
		return wrapErr(err, "上下文已取消")
	}

	// 如果配置了优先使用免扫码登录
	if b.config.PushLogin {
		// 尝试免扫码登录
		err := b.bot.PushLogin(b.storage, openwechat.NewRetryLoginOption())
		if err == nil {
			return b.afterLogin(ctx)
		}
	}

	// 尝试热登录
	err := b.bot.HotLogin(b.storage, openwechat.NewRetryLoginOption())
	if err == nil {
		return b.afterLogin(ctx)
	}

	// 都失败，需要扫码登录
	outfmt.Println("⚠️  需要扫码登录...")
	outfmt.Println("请使用微信扫描二维码登录")

	// 设置更友好的二维码显示
	b.bot.UUIDCallback = func(uuid string) {
		qrURL := "https://login.weixin.qq.com/l/" + uuid
		outfmt.Printf("\n📱 请扫码登录: %s\n", qrURL)
		outfmt.Println("或者打开以下链接扫描二维码:")
		outfmt.Println(qrURL)
	}

	// 执行扫码登录
	if err := b.bot.Login(); err != nil {
		return wrapErr(err, "扫码登录失败")
	}

	return b.afterLogin(ctx)
}

// afterLogin 登录后的处理
func (b *Bot) afterLogin(ctx context.Context) error {
	// 检查上下文是否已取消
	if err := ctx.Err(); err != nil {
		return wrapErr(err, "上下文已取消")
	}

	// 获取当前用户
	self, err := b.bot.GetCurrentUser()
	if err != nil {
		return wrapErr(err, "获取用户信息失败")
	}

	b.self = self
	outfmt.Printf("✅ 登录成功: %s\n", self.NickName)

	// 设置消息处理器（只接收，不自动回复）
	b.bot.MessageHandler = func(msg *openwechat.Message) {
		// 这里只记录收到消息，不自动回复
		// 回复由AI通过命令行控制
		outfmt.Printf("📨 收到消息: %s\n", msg.Content)
	}

	return nil
}

// IsLoggedIn 检查是否已登录
func (b *Bot) IsLoggedIn() bool {
	if b.self == nil {
		return false
	}
	return b.bot.Alive()
}

// GetCurrentUser 获取当前用户信息
func (b *Bot) GetCurrentUser(ctx context.Context) (*openwechat.Self, error) {
	if err := ctx.Err(); err != nil {
		return nil, wrapErr(err, "上下文已取消")
	}

	if b.self == nil {
		return nil, ErrNotLoggedIn
	}
	return b.self, nil
}

// Logout 退出登录
func (b *Bot) Logout(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return wrapErr(err, "上下文已取消")
	}

	if b.storage != nil {
		defer b.storage.Close()
	}

	if b.bot != nil {
		if err := b.bot.Logout(); err != nil {
			return wrapErr(err, "退出登录失败")
		}
	}

	b.self = nil
	outfmt.Println("✅ 已退出登录")
	return nil
}

// Block 阻塞等待消息
func (b *Bot) Block(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return wrapErr(err, "上下文已取消")
	}

	if !b.IsLoggedIn() {
		return ErrNotLoggedIn
	}
	return b.bot.Block()
}

// GetFriends 获取好友列表
func (b *Bot) GetFriends(ctx context.Context, refresh bool) (openwechat.Friends, error) {
	if err := ctx.Err(); err != nil {
		return nil, wrapErr(err, "上下文已取消")
	}

	if !b.IsLoggedIn() {
		return nil, ErrNotLoggedIn
	}

	friends, err := b.self.Friends(refresh)
	if err != nil {
		return nil, wrapErr(err, "获取好友列表失败")
	}

	return friends, nil
}

// GetGroups 获取群组列表
func (b *Bot) GetGroups(ctx context.Context, refresh bool) (openwechat.Groups, error) {
	if err := ctx.Err(); err != nil {
		return nil, wrapErr(err, "上下文已取消")
	}

	if !b.IsLoggedIn() {
		return nil, ErrNotLoggedIn
	}

	groups, err := b.self.Groups(refresh)
	if err != nil {
		return nil, wrapErr(err, "获取群组列表失败")
	}

	return groups, nil
}

// GetMps 获取公众号列表
func (b *Bot) GetMps(ctx context.Context, refresh bool) (openwechat.Mps, error) {
	if err := ctx.Err(); err != nil {
		return nil, wrapErr(err, "上下文已取消")
	}

	if !b.IsLoggedIn() {
		return nil, ErrNotLoggedIn
	}

	mps, err := b.self.Mps(refresh)
	if err != nil {
		return nil, wrapErr(err, "获取公众号列表失败")
	}

	return mps, nil
}

// SendText 发送文本消息
func (b *Bot) SendText(ctx context.Context, to, content string) error {
	if err := ctx.Err(); err != nil {
		return wrapErr(err, "上下文已取消")
	}

	if !b.IsLoggedIn() {
		return ErrNotLoggedIn
	}

	// 检查消息长度
	if len(content) > b.config.MaxMsgLength {
		return wrapErr(ErrMessageTooLong, fmt.Sprintf("消息长度超过限制: %d > %d", len(content), b.config.MaxMsgLength))
	}

	// 查找联系人（这里简化处理，实际需要根据to查找具体联系人）
	// 在实际实现中，需要根据to参数查找好友、群组或公众号
	// 这里先返回一个占位错误
	return fmt.Errorf("发送消息功能待实现: to=%s, content=%s", to, truncate(content, 50))
}

// Close 关闭资源
func (b *Bot) Close() error {
	if b.storage != nil {
		return b.storage.Close()
	}
	return nil
}

// truncate 截断字符串，辅助函数
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
