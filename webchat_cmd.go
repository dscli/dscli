package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/nanjj/clog"

	"github.com/dscli/dscli/internal/lp"
	"github.com/dscli/dscli/internal/outfmt"
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
  echo "review 这段代码" | dscli webchat
  echo "识别图中文字" | dscli webchat --mode flash --attach screenshot.png

继续会话（--keep）：
  dscli webchat --keep "第一个问题"            # 继续最近一次会话
  dscli webchat --keep=<会话ID> "继续讨论..."   # 继续指定会话
  dscli webchat --keep=<会话URL> "继续讨论..."  # 继续浏览器中打开的会话
  dscli webchat --keep=list                     # 列出所有已保存会话
每次回复都会把会话 ID 打印到 stderr（格式 keep:<id>），可直接作为 --keep 参数使用。

模式（--mode）：
  pro    专家模式（V4 Pro，默认），深度思考
  flash  快速模式（V4 Flash），深度思考 + 智能搜索 + 图片上传
  vision 识图模式（V4 Vision），深度思考 + 图片上传

--keep 且未指定 --mode 时保留原会话模式。

附件（--attach，可多次指定，仅 flash/vision 模式支持）：
  dscli webchat --mode vision --attach screenshot.png "这张截图说明了什么？"

上传限制：最多 50 个文件、共 100MB，仅识别图片中的文字。

注意：Web 版不支持函数调用（tool use），仅适用于问专家、code review 等
无需工具的简单场景。`,
		Args: cobra.MaximumNArgs(1),
		RunE: webchatRunE,
	})

	// --input defaults to "-": piped stdin is the primary non-arg input
	// channel (echo "msg" | dscli webchat). A terminal stdin is rejected in
	// gatherWebchatInput with a helpful error instead of hanging on EOF.
	webchatCmd.Flags().String("input", "-", "从文件读取消息（默认 - 从 stdin 管道读取；终端下请提供位置参数或 --input 文件）")
	// --keep is a custom string flag with NoOptDefVal="last": a bare
	// "--keep" continues the most recent conversation (backwards compatible
	// with the old boolean flag and the "--keep 消息" usage, where the next
	// argument remains the message), while "--keep=<id|url>" targets a
	// specific conversation and "--keep=list" lists saved ones.
	var keep string
	keepFlag := webchatCmd.Flags().VarPF(&keepValue{&keep}, "keep", "", "继续会话：--keep（最近一次）| --keep=<会话ID|会话URL> | --keep=list（列出已保存会话）；默认开新对话")
	keepFlag.NoOptDefVal = "last"
	webchatCmd.Flags().String("mode", "", "聊天模式: pro (专家/V4 Pro), flash (快速/V4 Flash), vision (识图/V4 Vision)；默认 pro，--keep 时保留原模式")
	// --attach accepts any user-readable path (absolute included): the CLI
	// is human-driven and the operator can already read those files. The
	// ask_expert TOOL is LLM-driven and sandboxes paths to the project
	// directory, the user's home (~/ or $HOME absolute), or the system
	// temp dir (/tmp) instead (verifySafePath), since the model is
	// untrusted.
	webchatCmd.Flags().StringSlice("attach", nil, "附件图片路径，可多次指定（仅 flash/vision 模式支持）")
}

// keepValue is a pflag.Value for --keep. Type() returns "string" so
// cmd.Flags().GetString("keep") works; the NoOptDefVal semantics (bare
// --keep = "last") are set at registration time.
type keepValue struct{ v *string }

func (k keepValue) String() string {
	if k.v == nil || *k.v == "" {
		return ""
	}
	return *k.v
}

func (k keepValue) Set(s string) error { *k.v = s; return nil }

func (k keepValue) Type() string { return "string" }

func webchatRunE(cmd *cobra.Command, args []string) error {
	span, ctx := clog.StartSpanFromContext(cmd.Context(), "webchatRunE")
	defer span.Finish()

	keep, _ := cmd.Flags().GetString("keep")
	if keep == "list" {
		return webchatListConversations()
	}

	message, err := gatherWebchatInput(cmd, args)
	if err != nil {
		return err
	}

	modeStr, _ := cmd.Flags().GetString("mode")
	attach, _ := cmd.Flags().GetStringSlice("attach")

	opts := lp.WebChatOptions{
		Mode:        lp.Mode(modeStr),
		Attachments: attach,
		Keep:        keep,
	}
	var result lp.WebChatResult
	startTime := time.Now()

	outfmt.Printf("📤 发送到 DeepSeek Web ...\n")
	result, err = lp.WebChatWithOptions(ctx, message, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "webchat 失败: %v\n", err)
		return nil
	}

	elapsed := time.Since(startTime)
	outfmt.Printf("📥 收到回复 (%.1fs)\n\n", elapsed.Seconds())
	fmt.Println(result.Content)

	// Surface the conversation ID on stderr (never stdout, which carries the
	// reply and may be redirected): keep:<id> is directly usable as --keep.
	if result.URL != "" {
		outfmt.Println(formatConversationHint(result.URL))
	}

	return nil
}

// formatConversationHint renders the stderr hint shown after a successful
// reply. It prints the ID in keep:<id> form — copy-paste ready for --keep —
// and falls back to the raw URL when the ID cannot be extracted.
func formatConversationHint(url string) string {
	if url == "" {
		return ""
	}
	if id := lp.ConversationIDFromURL(url); id != "" {
		return fmt.Sprintf("📋 会话已保存: keep:%s\n   继续对话: dscli webchat --keep=%s \"你的问题\"", id, id)
	}
	return "📋 会话 URL: " + url
}

// webchatListConversations prints the saved conversation registry.
func webchatListConversations() error {
	fmt.Println("ℹ️  --keep=list 仅列出已保存会话，不发送消息。")
	convs, err := lp.ListConversations()
	if err != nil {
		return err
	}
	if len(convs) == 0 {
		fmt.Println("暂无已保存会话。先发一条消息，或用 --keep=<会话URL> 登记浏览器中打开的会话。")
		return nil
	}
	fmt.Println("已保存会话（最新在前），ID 可直接用于 --keep=<ID>：")
	for _, c := range convs {
		mode := string(c.Mode)
		if mode == "" {
			mode = "-"
		}
		fmt.Printf("  %-36s  [%s]  %s\n", c.ID, mode, c.UpdatedAt)
		fmt.Printf("      %s\n", c.URL)
	}
	return nil
}

// gatherWebchatInput collects the message from args or --input flag.
// Priority: positional args > --input flag (file path or "-" for stdin).
// The flag defaults to "-", so a piped stdin (echo ... | dscli webchat)
// works without any argument; a terminal stdin is rejected with a helpful
// error instead of hanging on EOF.
func gatherWebchatInput(cmd *cobra.Command, args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}

	input, _ := cmd.Flags().GetString("input")
	if input == "" {
		input = "-" // explicit --input "" behaves like the default
	}

	if input == "-" {
		if isTerminal(os.Stdin) {
			return "", fmt.Errorf("请提供消息（位置参数或 --input 文件），或通过管道输入，例如: echo '消息' | dscli webchat")
		}
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
