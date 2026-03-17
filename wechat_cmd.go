package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"gitcode.com/dscli/dscli/internal/wechat"
	"github.com/spf13/cobra"
)

func wechatPersistentPreRunE(cmd *cobra.Command, args []string) error {
	// 获取命令行标志
	simple, _ := cmd.Flags().GetBool("simple")
	markdown, _ := cmd.Flags().GetBool("markdown")
	org, _ := cmd.Flags().GetBool("org")

	// 设置输出格式
	var format string
	if simple {
		format = "simple"
	} else if markdown {
		format = "markdown"
	} else if org {
		format = "org"
	} else {
		format = "table"
	}

	// 将格式设置到上下文中
	ctx := cmd.Context()
	ctx = context.WithValue(ctx, WechatFormatKey, format)
	cmd.SetContext(ctx)

	return nil
}

func wechatLoginRunE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	client, err := wechat.NewClientFromConfig()
	if err != nil {
		return fmt.Errorf("创建微信客户端失败: %w", err)
	}
	defer client.Close()

	if err := client.Login(ctx); err != nil {
		return fmt.Errorf("微信登录失败: %w", err)
	}

	fmt.Println("✅ 登录成功")
	return nil
}

func wechatLogoutRunE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	client, err := wechat.NewClientFromConfig()
	if err != nil {
		return fmt.Errorf("创建微信客户端失败: %w", err)
	}
	defer client.Close()

	if err := client.Logout(ctx); err != nil {
		return fmt.Errorf("退出登录失败: %w", err)
	}

	fmt.Println("✅ 已退出登录")
	return nil
}

func wechatStatusRunE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	client, err := wechat.NewClientFromConfig()
	if err != nil {
		return fmt.Errorf("创建微信客户端失败: %w", err)
	}
	defer client.Close()

	status, err := client.Status(ctx)
	if err != nil {
		return fmt.Errorf("获取状态失败: %w", err)
	}

	fmt.Println("📱 微信状态")
	fmt.Println("─────────────────────────────")
	if loggedIn, ok := status["logged_in"].(bool); ok && loggedIn {
		fmt.Printf("状态:         已登录\n")
		if user, ok := status["user"].(string); ok {
			fmt.Printf("用户:         %s\n", user)
		}
		if unreadCount, ok := status["unread_count"].(int); ok {
			fmt.Printf("未读消息:     %d条\n", unreadCount)
		}
	} else {
		fmt.Printf("状态:         未登录\n")
	}
	if account, ok := status["account"].(string); ok {
		fmt.Printf("账号:         %s\n", account)
	}
	if mode, ok := status["mode"].(string); ok {
		fmt.Printf("模式:         %s\n", mode)
	}
	fmt.Println("─────────────────────────────")

	return nil
}

func wechatMessagesRunE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	unread, _ := cmd.Flags().GetBool("unread")
	limit, _ := cmd.Flags().GetInt("limit")

	client, err := wechat.NewClientFromConfig()
	if err != nil {
		return fmt.Errorf("创建微信客户端失败: %w", err)
	}
	defer client.Close()

	messages, err := client.GetMessages(ctx, unread, limit)
	if err != nil {
		return fmt.Errorf("获取消息失败: %w", err)
	}

	if len(messages) == 0 {
		if unread {
			fmt.Println("📭 没有未读消息")
		} else {
			fmt.Println("📭 没有消息")
		}
		return nil
	}

	// 从上下文中获取格式
	format := ContextValue(ctx, WechatFormatKey, "table")
	formatter := wechat.NewMessageFormatter(wechat.OutputFormat(format))
	fmt.Print(formatter.FormatMessages(messages))

	// 显示统计信息
	if unread {
		fmt.Printf("\n📱 共 %d 条未读消息\n", len(messages))
	} else {
		fmt.Printf("\n📱 共 %d 条消息\n", len(messages))
	}

	return nil
}

func wechatMessageRunE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	messageID := args[0]

	client, err := wechat.NewClientFromConfig()
	if err != nil {
		return fmt.Errorf("创建微信客户端失败: %w", err)
	}
	defer client.Close()

	msg, err := client.GetMessage(ctx, messageID)
	if err != nil {
		return fmt.Errorf("获取消息失败: %w", err)
	}

	// 从上下文中获取格式
	format := ContextValue(ctx, WechatFormatKey, "table")
	formatter := wechat.NewMessageFormatter(wechat.OutputFormat(format))
	fmt.Print(formatter.FormatMessageDetail(msg))

	return nil
}

func wechatSendRunE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	to := args[0]
	text, _ := cmd.Flags().GetString("text")
	stdin, _ := cmd.Flags().GetBool("stdin")

	content := text

	// 从stdin读取（如果提供了--stdin）
	if stdin {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("读取标准输入失败: %w", err)
		}
		content = string(data)
	}

	if content == "" {
		return fmt.Errorf("消息内容不能为空，请使用--text参数或--stdin")
	}

	client, err := wechat.NewClientFromConfig()
	if err != nil {
		return fmt.Errorf("创建微信客户端失败: %w", err)
	}
	defer client.Close()

	if err := client.SendMessage(ctx, to, content); err != nil {
		return fmt.Errorf("发送消息失败: %w", err)
	}

	fmt.Println("✅ 消息已发送")
	return nil
}

func wechatReplyRunE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	messageID := args[0]
	text, _ := cmd.Flags().GetString("text")
	stdin, _ := cmd.Flags().GetBool("stdin")

	content := text

	// 从stdin读取（如果提供了--stdin）
	if stdin {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("读取标准输入失败: %w", err)
		}
		content = string(data)
	}

	if content == "" {
		return fmt.Errorf("回复内容不能为空，请使用--text参数或--stdin")
	}

	client, err := wechat.NewClientFromConfig()
	if err != nil {
		return fmt.Errorf("创建微信客户端失败: %w", err)
	}
	defer client.Close()

	if err := client.ReplyMessage(ctx, messageID, content); err != nil {
		return fmt.Errorf("回复消息失败: %w", err)
	}

	fmt.Println("✅ 回复已发送")
	return nil
}

func wechatMarkReadRunE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	messageID := args[0]

	client, err := wechat.NewClientFromConfig()
	if err != nil {
		return fmt.Errorf("创建微信客户端失败: %w", err)
	}
	defer client.Close()

	if err := client.MarkAsRead(ctx, messageID); err != nil {
		return fmt.Errorf("标记消息为已读失败: %w", err)
	}

	fmt.Println("✅ 消息已标记为已读")
	return nil
}

func wechatFriendsRunE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	client, err := wechat.NewClientFromConfig()
	if err != nil {
		return fmt.Errorf("创建微信客户端失败: %w", err)
	}
	defer client.Close()

	friends, err := client.GetFriends(ctx)
	if err != nil {
		return fmt.Errorf("获取好友列表失败: %w", err)
	}

	if len(friends) == 0 {
		fmt.Println("👥 没有好友")
		return nil
	}

	fmt.Println("👥 好友列表")
	fmt.Println("─────────────────────────────")
	for i, f := range friends {
		nickname := f["nickname"]
		remark := f["remark"]
		if remark != "" {
			fmt.Printf("%3d. %s (%s)\n", i+1, remark, nickname)
		} else {
			fmt.Printf("%3d. %s\n", i+1, nickname)
		}
	}
	fmt.Println("─────────────────────────────")
	fmt.Printf("共 %d 位好友\n", len(friends))

	return nil
}

func wechatGroupsRunE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	client, err := wechat.NewClientFromConfig()
	if err != nil {
		return fmt.Errorf("创建微信客户端失败: %w", err)
	}
	defer client.Close()

	groups, err := client.GetGroups(ctx)
	if err != nil {
		return fmt.Errorf("获取群组列表失败: %w", err)
	}

	if len(groups) == 0 {
		fmt.Println("👥 没有群组")
		return nil
	}

	fmt.Println("👥 群组列表")
	fmt.Println("─────────────────────────────")
	for i, g := range groups {
		nickname := g["nickname"]
		remark := g["remark"]
		if remark != "" && remark != nickname {
			fmt.Printf("%3d. %s (%s)\n", i+1, remark, nickname)
		} else {
			fmt.Printf("%3d. %s\n", i+1, nickname)
		}
	}
	fmt.Println("─────────────────────────────")
	fmt.Printf("共 %d 个群组\n", len(groups))

	return nil
}

func wechatConfigRunE(cmd *cobra.Command, args []string) error {
	configPath, _ := cmd.Flags().GetString("config")
	if configPath == "" {
		configPath = wechat.GetDefaultConfigPath()
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// 创建默认配置
		config := wechat.DefaultConfig()
		if err := wechat.SaveConfig(config, configPath); err != nil {
			return fmt.Errorf("创建默认配置失败: %w", err)
		}
		fmt.Printf("✅ 已创建默认配置文件: %s\n", configPath)
	} else {
		// 显示现有配置
		config, err := wechat.LoadConfig(configPath)
		if err != nil {
			return fmt.Errorf("加载配置失败: %w", err)
		}

		fmt.Println("⚙️  当前配置")
		fmt.Println("─────────────────────────────")
		fmt.Printf("配置文件:     %s\n", configPath)
		fmt.Printf("账号:         %s\n", config.Account)
		fmt.Printf("模式:         %s\n", config.Mode)
		fmt.Printf("数据库:       %s\n", config.DBPath)
		fmt.Printf("自动登录:     %v\n", config.AutoLogin)
		fmt.Printf("免扫码登录:   %v\n", config.PushLogin)
		fmt.Printf("回复延迟:     %dms\n", config.ReplyDelay)
		fmt.Printf("最大消息长度: %d\n", config.MaxMsgLength)
		fmt.Printf("白名单好友:   %d人\n", len(config.AllowedFriends))
		fmt.Printf("黑名单用户:   %d人\n", len(config.BlockedUsers))
		fmt.Println("─────────────────────────────")
	}

	return nil
}
