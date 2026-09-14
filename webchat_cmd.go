package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/nanjj/clog"

	"github.com/dscli/dscli/internal/keeprunning"
	"github.com/dscli/dscli/internal/lp"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/spf13/cobra"
)

func init() {
	webchatCmd := AddRootCommand(&cobra.Command{
		Use:   "webchat [message]",
		Short: "通过 Chrome 浏览器与 DeepSeek Web 聊天（免费；支持角色与工具调用）",
		Long: `通过 Chrome 浏览器与 https://chat.deepseek.com 交互。

首次使用会自动打开浏览器窗口要求登录（登录状态持久保存）。

发送消息：
  dscli webchat "什么是闭包？"
  echo "review 这段代码" | dscli webchat
  echo "识别图中文字" | dscli webchat --attach screenshot.png

继续会话（--keep=<会话ID|会话URL|last>；last = 最近一次会话）：
  dscli webchat --keep=last "第一个问题"           # 继续最近一次会话
  dscli webchat --keep=<会话ID> "继续讨论..."       # 继续指定会话
  dscli webchat --keep=<会话URL> "继续讨论..."      # 继续浏览器中打开的会话
  dscli webchat --keep=list                         # 列出所有已保存会话
--keep 需要显式取值（--keep <值> 与 --keep=<值> 等价，与 dscli chat 的取值语法一致）：
消息写在取值之后的位置参数或经 --input/stdin 传入；直接跟在 --keep 后面的文本会被
当成会话取值，而不是消息。
续会话不会再次注入角色提示词（第一轮已注入）。
每次回复都会把会话 ID 打印到 stderr（格式 keep:<id>），可直接作为 --keep 参数使用。

角色（--role，与 dscli chat 一致；默认空 = 纯聊天）：
  dscli webchat --role review "review 最近的提交"     # code review 角色
  dscli webchat --role expert "分析这个架构问题"       # 领域专家角色
  dscli webchat --role dev "实现一个功能"              # 开发助手（<shell> 块通道）
  dscli webchat --role architect "设计并编排实现"      # 架构师角色
  dscli webchat "随便聊聊"                           # 默认纯聊天：无角色注入
非空角色会前置角色提示词。

<shell> 块通道（webchat 的工具通道，始终启用；工具调用与角色注入解耦）：
  dscli webchat --role dev "跑一下 make dev-test"
dev 角色会话由角色提示词注册 <shell> 块协议：模型每轮回复一个 bash 脚本块，
dscli 本地执行（默认 120s、上限 1800s 超时；破坏性命令拦截：rm -rf、sudo、
curl/wget 外传等被拒绝）并把 stdout+stderr 合并后以附件 scriptN.txt 回填到
同一会话（同 code_dev 工具）。无角色会话需要该协议时，可把协议文档随消息自行
注入（位置参数或 --input 文件）。这是远程模型在本地执行命令的会话：发送前会
打印警告；仍建议在可信工作目录使用。WebChat 不执行 DSML 工具调用：<shell> 块
是唯一工具通道。

附件（--attach，可多次指定）：
  dscli webchat --attach screenshot.png "这张截图说明了什么？"

上传限制：最多 50 个文件、共 100MB；支持图片、文本与 PDF 文件。
网站按文件扩展名决定是否接收：扩展名不受支持（如 .gitignore）或没有扩展名
（如 Makefile）的附件会自动以「原名 + .txt」上传（内容不变，仅上传名变化）。`,
		RunE: webchatRunE,
	})

	// --input defaults to "-": piped stdin is the primary non-arg input
	// channel (echo "msg" | dscli webchat). A terminal stdin is rejected in
	// gatherWebchatInput with a helpful error instead of hanging on EOF.
	webchatCmd.Flags().String("input", "-", "从文件读取消息（默认 - 从 stdin 管道读取；终端下请提供位置参数或 --input 文件）")
	// --keep takes an explicit value, like the other flags (and dscli
	// chat): both "--keep <值>" and "--keep=<值>" set it - a conversation
	// ID, "last" (the most recent conversation), a conversation URL, or
	// "list" (list saved conversations). Plain string flag on purpose: the
	// old NoOptDefVal="last" fallback left the value unset when it was
	// written with a space, silently sending the conversation ID as the
	// message body.
	webchatCmd.Flags().String("keep", "", "继续会话：--keep=<会话ID|会话URL|last>（last = 最近一次）| --keep=list（列出已保存会话）；默认开新对话")
	// --attach accepts any user-readable path (absolute included): the CLI
	// is human-driven and the operator can already read those files. The
	// ask_expert TOOL is LLM-driven and sandboxes paths to the project
	// directory, the user's home (~/ or $HOME absolute), or the system
	// temp dir (/tmp) instead (verifySafePath), since the model is
	// untrusted.
	webchatCmd.Flags().StringSlice("attach", nil, "附件文件路径（图片/文本/PDF），可多次指定")
	// --role defaults to "" = plain chat (no role prompt injection). A
	// non-empty value selects the role prompt template
	// (dev/expert/review/test/architect) before the user message.
	webchatCmd.Flags().String("role", "", "Role: dev (developer), expert (domain expert), review (code review), test (QA engineer), architect (software architect)；空 = 纯聊天（不注入角色提示词）；dev 角色注册 <shell> 块通道")
	// No --shell flag: the <shell> block channel is WebChat's only tool
	// channel (docs/task-shell-block.md), so this command always runs it -
	// there is no switch to turn it on or off.
}

// webchatOptionsFromFlags builds the HandleWebChat options from parsed CLI
// flags. Extracted as a pure function (no side effects) so tests can lock the
// contract: flag names, defaults, and the pass-through mapping into
// WebChatOptions (Role included).
//
// Role semantics: the flag defaults to "" (plain chat - no role prompt
// injected). A non-empty value is passed through unchanged.
//
// ShellTool is always true: <shell> is WebChat's only working tool channel
// (docs/task-shell-block.md), so the command has no switch for it - the
// shell channel is not optional.
func webchatOptionsFromFlags(cmd *cobra.Command) (lp.WebChatOptions, error) {
	keep, err := cmd.Flags().GetString("keep")
	if err != nil {
		return lp.WebChatOptions{}, err
	}
	role, err := cmd.Flags().GetString("role")
	if err != nil {
		return lp.WebChatOptions{}, err
	}
	attach, err := cmd.Flags().GetStringSlice("attach")
	if err != nil {
		return lp.WebChatOptions{}, err
	}
	return lp.WebChatOptions{
		Attachments: attach,
		Keep:        keep,
		Role:        role,
		ShellTool:   true,
	}, nil
}

func webchatRunE(cmd *cobra.Command, args []string) error {
	span, ctx := clog.StartSpanFromContext(cmd.Context(), "webchatRunE")
	defer span.Finish()
	keep, _ := cmd.Flags().GetString("keep")
	if keep == "list" {
		return webchatListConversations()
	}

	// Keep the system awake while the web session runs: a locked screen
	// suspends chat.deepseek.com and can kill long DSML tool loops.
	wakeLock := keeprunning.KeepRunning()
	defer wakeLock()

	message, err := gatherWebchatInput(cmd, args)
	if err != nil {
		return err
	}

	opts, err := webchatOptionsFromFlags(cmd)
	if err != nil {
		return err
	}
	var result lp.WebChatResult
	startTime := time.Now()

	// The webchat CLI always runs the shell channel, and a reply carrying a
	// `<shell>` block runs locally whatever the role: tool invocation is
	// decoupled from role injection (docs/task-shell-block.md). Say so
	// upfront (stderr, so piped stdout stays clean) - silent local execution
	// from a remote model is the surprise. Role "" (the default) is plain
	// chat: no role prompt is injected, and the tool doc is not registered
	// automatically - a session that needs it carries the doc in the message
	// itself (positional argument or --input file).
	fmt.Fprintf(os.Stderr, "⚠️ `<shell>` 块通道已启用：远程模型回复中的 bash 脚本将在本地执行（cwd = 项目根；破坏性命令被拦截）。\n")

	outfmt.Printf("📤 发送到 DeepSeek Web ...\n")
	// HandleWebChat shares the ask_expert entry point: transient server
	// overload and truncation are retried with backoff. A non-empty Role
	// prepends the role prompt; replies carrying a `<shell>` block are
	// executed locally and fed back into the same conversation until the
	// expert finishes.
	result, err = lp.HandleWebChat(ctx, message, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "webchat 失败: %v\n", err)
		return nil
	}

	elapsed := time.Since(startTime)
	outfmt.Printf("📥 收到回复 (%.1fs)\n\n", elapsed.Seconds())
	// 工具循环场景下，每一轮（含最终答复）已由 HandleWebChat 通过
	// outfmt.PrintContent 打印（含 reasoning 与 content）；这里只打印收尾
	// 的纯 content——非循环场景（一次性的散文回复）才需要。
	if !result.Printed {
		fmt.Println(result.Content)
	}

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
		fmt.Printf("  %-36s  %s\n", c.ID, c.UpdatedAt)
		fmt.Printf("      %s\n", c.URL)
	}
	return nil
}

// gatherWebchatInput collects the message from args or --input flag.
// Priority: positional args (space-joined) > --input flag (file path or "-"
// for stdin) - the same input grammar as dscli chat's gatherInput.
// The flag defaults to "-", so a piped stdin (echo ... | dscli webchat)
// works without any argument; a terminal stdin is rejected with a helpful
// error instead of hanging on EOF.
func gatherWebchatInput(cmd *cobra.Command, args []string) (string, error) {
	if len(args) > 0 {
		return strings.Join(args, " "), nil
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
