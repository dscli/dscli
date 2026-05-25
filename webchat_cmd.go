package main

import (
	"context"
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
		Short: "通过浏览器与 DeepSeek Web 聊天（免费，不支持 tool use）",
		Long: `通过 lightpanda 浏览器与 https://chat.deepseek.com 交互。

使用前需先登录一次：
  dscli webchat --setup

然后可以发送消息：
  dscli webchat "什么是闭包？"
  echo "review 这段代码" | dscli webchat --input -
  cat question.txt | dscli webchat --input -

注意：Web 版不支持函数调用（tool use），仅适用于问专家、code review 等
无需工具的简单场景。`,
		Args: cobra.MaximumNArgs(1),
		RunE: webchatRunE,
	})

	webchatCmd.Flags().Bool("input", false, "从 stdin 读取消息（使用 - 作为占位参数）")
	webchatCmd.Flags().Int("timeout", 120, "超时时间（秒）")
	webchatCmd.Flags().Bool("setup", false, "设置 DeepSeek Web 登录 cookies（一次性操作）")
}

func webchatRunE(cmd *cobra.Command, args []string) error {
	setup, _ := cmd.Flags().GetBool("setup")
	if setup {
		return webchatSetup()
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

// webchatSetup guides the user through one-time cookie export from their
// browser and saves the configuration for subsequent webchat calls.
func webchatSetup() error {
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║           DeepSeek Web 登录设置（一次性操作）               ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("步骤 1：在 Chrome/Firefox 中打开 https://chat.deepseek.com 并登录")
	fmt.Println()
	fmt.Println("步骤 2：登录后，按 F12 打开开发者工具，切换到 Console 标签页")
	fmt.Println()
	fmt.Println("步骤 3：粘贴以下 JS 代码并回车运行，cookies 将自动复制到剪贴板：")
	fmt.Println()
	fmt.Println("  ────────────────────────────────────────────────────────")
	fmt.Println("  // Copy cookies to clipboard in Lightpanda JSON format")
	fmt.Println("  copy(JSON.stringify(")
	fmt.Println("    document.cookie.split('; ').filter(Boolean).map(c => {")
	fmt.Println("      const eq = c.indexOf('=');")
	fmt.Println("      return {")
	fmt.Println("        name:   c.slice(0, eq),")
	fmt.Println("        value:  c.slice(eq + 1),")
	fmt.Println("        domain: '.deepseek.com',")
	fmt.Println("        path:   '/'")
	fmt.Println("      };")
	fmt.Println("    })")
	fmt.Println("  ));")
	fmt.Println("  ────────────────────────────────────────────────────────")
	fmt.Println()
	fmt.Println("步骤 4：将剪贴板中的 JSON 保存到文件：")
	fmt.Println()
	fmt.Printf("  ~/.dscli/deepseek-cookies.json\n")
	fmt.Println()
	fmt.Println("步骤 5：运行以下命令启用配置：")
	fmt.Println()
	fmt.Println("  dscli config set lightpanda-cookie-file ~/.dscli/deepseek-cookies.json")
	fmt.Println("  dscli config set lightpanda-storage-engine sqlite")
	fmt.Println("  dscli config set lightpanda-storage-sqlite-path ~/.dscli/lightpanda.db")
	fmt.Println()
	fmt.Println("完成后即可使用 dscli webchat 发送消息！")

	return nil
}
