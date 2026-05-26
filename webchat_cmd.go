package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"gitcode.com/dscli/dscli/internal/lp"
	"github.com/spf13/cobra"
)

func init() {
	webchatCmd := AddRootCommand(&cobra.Command{
		Use:   "webchat [message]",
		Short: "通过 Chrome 浏览器与 DeepSeek Web 聊天（免费，不支持 tool use）",
		Long: `通过 Chrome 浏览器与 https://chat.deepseek.com 交互。

首次使用会自动打开浏览器窗口要求登录（登录状态持久保存）。

发送消息：
  dscli webchat "什么是闭包？"
  echo "review 这段代码" | dscli webchat --input -

注意：Web 版不支持函数调用（tool use），仅适用于问专家、code review 等
无需工具的简单场景。`,
		Args: cobra.MaximumNArgs(1),
		RunE: webchatRunE,
	})

	webchatCmd.Flags().String("input", "", "从文件读取消息（使用 - 表示从 stdin 读取）")
}

func webchatRunE(cmd *cobra.Command, args []string) error {
	message, err := gatherWebchatInput(cmd, args)
	if err != nil {
		return err
	}

	ctx := cmd.Context()

	fmt.Fprintf(os.Stderr, "📤 发送到 DeepSeek Web...\n")
	startTime := time.Now()

	response, err := lp.WebChat(ctx, message)
	if errors.Is(err, lp.ErrLoginRequired) {
		fmt.Fprintln(os.Stderr, "🔐 未登录，自动打开浏览器登录窗口...")
		fmt.Fprintln(os.Stderr, "   请在弹出的浏览器窗口中完成登录。")
		if loginErr := lp.WebChatLogin(ctx); loginErr != nil {
			return fmt.Errorf("登录失败: %w", loginErr)
		}
		// Retry after login.
		fmt.Fprintf(os.Stderr, "📤 重新发送到 DeepSeek Web...\n")
		startTime = time.Now()
		response, err = lp.WebChat(ctx, message)
	}
	if err != nil {
		return fmt.Errorf("webchat 失败: %w", err)
	}

	elapsed := time.Since(startTime)
	fmt.Fprintf(os.Stderr, "📥 收到回复 (%.1fs)\n\n", elapsed.Seconds())
	fmt.Println(response)

	return nil
}

// gatherWebchatInput collects the message from args or --input flag.
// Priority: positional args > --input flag (file path or "-" for stdin).
func gatherWebchatInput(cmd *cobra.Command, args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}

	input, _ := cmd.Flags().GetString("input")
	if input == "" {
		return "", fmt.Errorf("请提供消息，或使用 --input 从文件/stdin 读取")
	}

	if input == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("读取 stdin 失败: %w", err)
		}
		message := strings.TrimSpace(string(data))
		if message == "" {
			return "", fmt.Errorf("stdin 为空")
		}
		return message, nil
	}

	data, err := os.ReadFile(input)
	if err != nil {
		return "", fmt.Errorf("读取输入文件 %s 失败: %w", input, err)
	}
	return strings.TrimSpace(string(data)), nil
}
