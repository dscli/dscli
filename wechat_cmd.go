package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"gitcode.com/dscli/dscli/internal/wechat"
	"github.com/spf13/cobra"
)

// var (
// 	wechatFormat     string
// 	wechatSimple     bool
// 	wechatMarkdown   bool
// 	wechatOrg        bool
// 	wechatUnread     bool
// 	wechatLimit      int
// 	wechatText       string
// 	wechatStdin      bool
// 	wechatConfigPath string
// )

func init() {
	wechatCmd := AddRootCommand(&cobra.Command{
		Use:   "wechat",
		Short: "微信AI工具接口（供dscli chat使用）",
		Long: `微信AI工具接口 - 为AI提供微信交互能力

这个工具主要供 dscli chat 使用，让AI能够通过微信与人类进行交互。
支持智能登录、消息收发、联系人管理等功能。`,
		PersistentPreRunE: wechatPersistentPreRunE,
	})
	// 将wechat命令添加到根命令
	AddRootCommand(wechatCmd)

	// 添加子命令
	_ = AddCommand(wechatCmd, &cobra.Command{
		Use:   "login",
		Short: "登录微信",
		Long: `智能登录微信，自动尝试最优登录方式：
1. 优先尝试免扫码登录（PushLogin）
2. 失败则尝试热登录（HotLogin）
3. 最后使用扫码登录`,
		RunE: wechatLoginRunE,
	})
	_ = AddCommand(wechatCmd, &cobra.Command{
		Use:   "logout",
		Short: "退出登录",
		RunE:  wechatLogoutRunE,
	})

	_ = AddCommand(wechatCmd, &cobra.Command{
		Use:   "status",
		Short: "查看微信状态",
		RunE:  wechatStatusRunE,
	})

	wechatMessagesCmd := AddCommand(wechatCmd, &cobra.Command{
		Use:   "messages",
		Short: "查看消息",
		Long: `查看微信消息，支持多种输出格式：
- 默认：表格格式（人类友好）
- --simple：制表符分隔（AI友好）
- --markdown：Markdown表格
- --org：Org mode表格`,
		RunE: wechatMessagesRunE,
	})
	_ = AddCommand(wechatCmd, &cobra.Command{
		Use:   "message <消息ID>",
		Short: "查看单条消息详情",
		Args:  cobra.ExactArgs(1),
		RunE:  wechatMessageRunE,
	})
	wechatSendCmd := AddCommand(wechatCmd, &cobra.Command{
		Use:   "send <微信号/昵称>",
		Short: "发送消息",
		Long: `发送消息给指定的微信号或昵称。
支持从命令行参数或标准输入读取消息内容。`,
		Args: cobra.ExactArgs(1),
		RunE: wechatSendRunE,
	})
	wechatReplyCmd := AddCommand(wechatCmd, &cobra.Command{
		Use:   "reply <消息ID>",
		Short: "回复消息",
		Long: `回复指定的消息。
支持从命令行参数或标准输入读取回复内容。`,
		Args: cobra.ExactArgs(1),
		RunE: wechatReplyRunE,
	})
	_ = AddCommand(wechatCmd, &cobra.Command{
		Use:   "mark-read <消息ID>",
		Short: "标记消息为已读",
		Args:  cobra.ExactArgs(1),
		RunE:  wechatMarkReadRunE,
	})
	_ = AddCommand(wechatCmd, &cobra.Command{
		Use:   "friends",
		Short: "查看好友列表",
		RunE:  wechatFriendsRunE,
	})
	_ = AddCommand(wechatCmd, &cobra.Command{
		Use:   "groups",
		Short: "查看群组列表",
		RunE:  wechatGroupsRunE,
	})
	wechatConfigCmd := AddCommand(wechatCmd, &cobra.Command{
		Use:   "config",
		Short: "管理配置",
		Long: `管理微信客户端配置。
如果不指定配置文件路径，使用默认路径：~/.dscli/wechat.json`,
		RunE: wechatConfigRunE,
	})

	// 全局标志
	wechatCmd.PersistentFlags().String("format", "table", "输出格式: simple, table, markdown, org")
	wechatCmd.PersistentFlags().Bool("simple", false, "简洁格式（制表符分隔）")
	wechatCmd.PersistentFlags().Bool("markdown", false, "Markdown格式")
	wechatCmd.PersistentFlags().Bool("org", false, "Org mode格式")
	wechatCmd.PersistentFlags().String("config", "", "配置文件路径")

	// messages命令标志
	wechatMessagesCmd.Flags().Bool("unread", false, "只显示未读消息")
	wechatMessagesCmd.Flags().Int("limit", 50, "显示消息数量限制")

	// send/reply命令标志
	wechatSendCmd.Flags().String("text", "t", "消息内容")
	wechatSendCmd.Flags().Bool("stdin", false, "从标准输入读取消息内容")
	wechatReplyCmd.Flags().String("text", "t", "回复内容")
	wechatReplyCmd.Flags().Bool("stdin", false, "从标准输入读取回复内容")

	// config命令标志
	wechatConfigCmd.Flags().String("config", "", "配置文件路径")
}

func wechatPersistentPreRunE(cmd *cobra.Command, args []string) (err error) {
	wechatSimple, err := cmd.Flags().GetBool("simple")
	if err != nil {
		return
	}

	wechatMarkdown, err := cmd.Flags().GetBool("markdown")
	if err != nil {
		return
	}

	wechatOrg, err := cmd.Flags().GetBool("org")
	if err != nil {
		return
	}

	wechatFormat := ""
	// 设置输出格式
	if wechatSimple {
		wechatFormat = "simple"
	} else if wechatMarkdown {
		wechatFormat = "markdown"
	} else if wechatOrg {
		wechatFormat = "org"
	}

	if wechatFormat == "" {
		wechatFormat = "simple"
	}

	ctx := cmd.Context()
	ctx = context.WithValue(ctx, WechatFormatKey, wechatFormat)
	cmd.SetContext(ctx)
	return nil
}

func wechatConfigRunE(cmd *cobra.Command, args []string) (err error) {
	wechatConfigPath, err := cmd.Flags().GetString("config")
	if err != nil {
		return
	}
	if wechatConfigPath == "" {
		wechatConfigPath = wechat.GetDefaultConfigPath()
	}

	if _, err := os.Stat(wechatConfigPath); os.IsNotExist(err) {
		// 创建默认配置
		config := wechat.DefaultConfig()
		if err := wechat.SaveConfig(config, wechatConfigPath); err != nil {
			return fmt.Errorf("创建默认配置失败: %w", err)
		}
		fmt.Printf("✅ 已创建默认配置文件: %s\n", wechatConfigPath)
	} else {
		// 显示现有配置
		config, err := wechat.LoadConfig(wechatConfigPath)
		if err != nil {
			return fmt.Errorf("加载配置失败: %w", err)
		}

		fmt.Println("⚙️  当前配置")
		fmt.Println("─────────────────────────────")
		fmt.Printf("配置文件:     %s\n", wechatConfigPath)
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

func wechatLoginRunE(cmd *cobra.Command, args []string) error {
	client, err := wechat.NewClientFromConfig()
	if err != nil {
		return fmt.Errorf("创建微信客户端失败: %w", err)
	}
	defer client.Close()

	if err := client.Login(); err != nil {
		return fmt.Errorf("微信登录失败: %w", err)
	}

	fmt.Println("✅ 登录成功")
	return nil
}

func wechatStatusRunE(cmd *cobra.Command, args []string) error {
	client, err := wechat.NewClientFromConfig()
	if err != nil {
		return fmt.Errorf("创建微信客户端失败: %w", err)
	}
	defer client.Close()

	status, err := client.Status()
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

func wechatMessagesRunE(cmd *cobra.Command, args []string) (err error) {
	client, err := wechat.NewClientFromConfig()
	if err != nil {
		return fmt.Errorf("创建微信客户端失败: %w", err)
	}
	defer client.Close()
	wechatUnread, err := cmd.Flags().GetBool("unread")
	if err != nil {
		return
	}
	wechatLimit, err := cmd.Flags().GetInt("limit")
	if err != nil {
		return
	}
	ctx := cmd.Context()
	wechatFormat := ContextValue(ctx, WechatFormatKey, "simple")

	messages, err := client.GetMessages(wechatUnread, wechatLimit)
	if err != nil {
		return fmt.Errorf("获取消息失败: %w", err)
	}

	if len(messages) == 0 {
		if wechatUnread {
			fmt.Println("📭 没有未读消息")
		} else {
			fmt.Println("📭 没有消息")
		}
		return nil
	}

	formatter := wechat.NewMessageFormatter(wechat.OutputFormat(wechatFormat))
	fmt.Print(formatter.FormatMessages(messages))

	// 显示统计信息
	if wechatUnread {
		fmt.Printf("\n📱 共 %d 条未读消息\n", len(messages))
	} else {
		fmt.Printf("\n📱 共 %d 条消息\n", len(messages))
	}

	return nil
}

func wechatMessageRunE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	wechatFormat := ContextValue(ctx, WechatFormatKey, "simple")
	messageID := args[0]

	client, err := wechat.NewClientFromConfig()
	if err != nil {
		return fmt.Errorf("创建微信客户端失败: %w", err)
	}
	defer client.Close()

	msg, err := client.GetMessage(messageID)
	if err != nil {
		return fmt.Errorf("获取消息失败: %w", err)
	}

	formatter := wechat.NewMessageFormatter(wechat.OutputFormat(wechatFormat))
	fmt.Print(formatter.FormatMessageDetail(msg))

	return nil
}

func wechatGroupsRunE(cmd *cobra.Command, args []string) error {
	client, err := wechat.NewClientFromConfig()
	if err != nil {
		return fmt.Errorf("创建微信客户端失败: %w", err)
	}
	defer client.Close()

	groups, err := client.GetGroups()
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

func wechatLogoutRunE(cmd *cobra.Command, args []string) error {
	client, err := wechat.NewClientFromConfig()
	if err != nil {
		return fmt.Errorf("创建微信客户端失败: %w", err)
	}
	defer client.Close()

	if err := client.Logout(); err != nil {
		return fmt.Errorf("退出登录失败: %w", err)
	}

	fmt.Println("✅ 已退出登录")
	return nil
}

func wechatSendRunE(cmd *cobra.Command, args []string) (err error) {
	wechatText, err := cmd.Flags().GetString("text")
	if err != nil {
		return
	}
	wechatStdin, err := cmd.Flags().GetBool("stdin")
	if err != nil {
		return
	}
	to := args[0]
	content := wechatText

	// 从stdin读取（如果提供了--stdin）
	if wechatStdin {
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

	if err := client.SendMessage(to, content); err != nil {
		return fmt.Errorf("发送消息失败: %w", err)
	}

	fmt.Println("✅ 消息已发送")
	return nil
}

func wechatReplyRunE(cmd *cobra.Command, args []string) (err error) {
	messageID := args[0]
	wechatStdin, err := cmd.Flags().GetBool("stdin")
	if err != nil {
		return
	}
	wechatText, err := cmd.Flags().GetString("text")
	if err != nil {
		return
	}
	content := wechatText

	// 从stdin读取（如果提供了--stdin）
	if wechatStdin {
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

	if err := client.ReplyMessage(messageID, content); err != nil {
		return fmt.Errorf("回复消息失败: %w", err)
	}

	fmt.Println("✅ 回复已发送")
	return nil
}

func wechatMarkReadRunE(cmd *cobra.Command, args []string) error {
	messageID := args[0]

	client, err := wechat.NewClientFromConfig()
	if err != nil {
		return fmt.Errorf("创建微信客户端失败: %w", err)
	}
	defer client.Close()

	if err := client.MarkAsRead(messageID); err != nil {
		return fmt.Errorf("标记消息为已读失败: %w", err)
	}

	fmt.Println("✅ 消息已标记为已读")
	return nil
}

func wechatFriendsRunE(cmd *cobra.Command, args []string) error {
	client, err := wechat.NewClientFromConfig()
	if err != nil {
		return fmt.Errorf("创建微信客户端失败: %w", err)
	}
	defer client.Close()

	friends, err := client.GetFriends()
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
