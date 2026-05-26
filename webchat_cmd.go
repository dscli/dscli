package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"gitcode.com/dscli/dscli/internal/config"
	"gitcode.com/dscli/dscli/internal/lp"
	"github.com/spf13/cobra"
)

func init() {
	webchatCmd := AddRootCommand(&cobra.Command{
		Use:   "webchat [message]",
		Short: "通过浏览器与 DeepSeek Web 聊天（免费，不支持 tool use）",
		Long: `通过 lightpanda 浏览器与 https://chat.deepseek.com 交互。

使用前需先登录一次：
  dscli webchat --login

然后可以发送消息：
  dscli webchat "什么是闭包？"
  echo "review 这段代码" | dscli webchat --input -

注意：Web 版不支持函数调用（tool use），仅适用于问专家、code review 等
无需工具的简单场景。`,
		Args: cobra.MaximumNArgs(1),
		RunE: webchatRunE,
	})

	webchatCmd.Flags().Bool("input", false, "从 stdin 读取消息（使用 - 作为占位参数）")
	webchatCmd.Flags().Int("timeout", 120, "超时时间（秒）")
	webchatCmd.Flags().Bool("login", false, "登录 DeepSeek（使用 Chrome 自动登录，手机号+验证码）")
	webchatCmd.Flags().String("phone", "13910969806", "登录手机号")
	webchatCmd.Flags().String("login-browser", "chrome", "登录使用的浏览器: chrome (默认) 或 lightpanda")
	webchatCmd.Flags().Bool("setup", false, "（已废弃）请使用 --login 替代")
}

func webchatRunE(cmd *cobra.Command, args []string) error {
	login, _ := cmd.Flags().GetBool("login")
	if login {
		return webchatLogin(cmd)
	}

	setup, _ := cmd.Flags().GetBool("setup")
	if setup {
		fmt.Fprintln(os.Stderr, "⚠️  --setup 已废弃，请使用 dscli webchat --login")
		return webchatLogin(cmd)
	}

	useStdin, _ := cmd.Flags().GetBool("input")
	timeout, _ := cmd.Flags().GetInt("timeout")

	var message string
	switch {
	case useStdin:
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("读取 stdin 失败: %w", err)
		}
		message = strings.TrimSpace(string(data))
		if message == "" {
			return fmt.Errorf("stdin 为空")
		}
	case len(args) == 1:
		message = args[0]
	default:
		return fmt.Errorf("请提供消息，或使用 --input 从 stdin 读取")
	}

	ctx := cmd.Context()
	if timeout > 0 {
		var cancel func()
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()
	}

	fmt.Fprintf(os.Stderr, "📤 发送到 DeepSeek Web...\n")
	startTime := time.Now()

	response, err := lp.WebChat(ctx, message)
	if err != nil {
		return fmt.Errorf("webchat 失败: %w", err)
	}

	elapsed := time.Since(startTime)
	fmt.Fprintf(os.Stderr, "📥 收到回复 (%.1fs)\n\n", elapsed.Seconds())
	fmt.Println(response)

	return nil
}

// webchatLogin implements the DeepSeek login flow.
func webchatLogin(cmd *cobra.Command) error {
	phone, _ := cmd.Flags().GetString("phone")
	browser, _ := cmd.Flags().GetString("login-browser")

	fmt.Fprintln(os.Stderr, "╔══════════════════════════════════════════════════════════════╗")
	fmt.Fprintln(os.Stderr, "║        DeepSeek Web 自动登录                               ║")
	fmt.Fprintln(os.Stderr, "╚══════════════════════════════════════════════════════════════╝")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "📱 手机号: %s\n", phone)
	fmt.Fprintf(os.Stderr, "🌐 浏览器: %s\n", browser)
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "流程：自动打开登录页 → 输入手机号 → 发送验证码 → 输入验证码 → 登录")
	fmt.Fprintln(os.Stderr)

	ctx := cmd.Context()

	var err error
	switch browser {
	case "chrome", "chromium":
		err = lp.DeepSeekLoginChrome(ctx, phone, lp.ReadCodeFromStdin)
	case "lightpanda", "lp":
		err = lp.DeepSeekLogin(ctx, phone, lp.ReadCodeFromStdin)
	default:
		return fmt.Errorf("不支持的浏览器: %s (可选: chrome, lightpanda)", browser)
	}
	if err != nil {
		return fmt.Errorf("登录失败: %w", err)
	}

	// Auto-configure the required settings for subsequent webchat calls.
	cookiePath := lp.DefaultCookiePath()
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "⚙️  正在配置 Lightpanda 以使用已保存的 cookies...")

	config.Set("lightpanda-cookie-file", cookiePath)
	config.Set("lightpanda-storage-engine", "sqlite")

	homeDir, _ := os.UserHomeDir()
	dbPath := homeDir + "/.dscli/lightpanda.db"
	config.Set("lightpanda-storage-sqlite-path", dbPath)

	fmt.Fprintf(os.Stderr, "✅ 配置完成。现在可以使用 dscli webchat 发送消息了！\n")

	return nil
}