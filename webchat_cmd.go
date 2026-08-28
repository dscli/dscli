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
		Short: "通过 Chrome 浏览器与 DeepSeek Web 聊天（免费；支持角色与 DSML 工具调用）",
		Long: `通过 Chrome 浏览器与 https://chat.deepseek.com 交互。

首次使用会自动打开浏览器窗口要求登录（登录状态持久保存）。

发送消息：
  dscli webchat "什么是闭包？"
  echo "review 这段代码" | dscli webchat
  echo "识别图中文字" | dscli webchat --model flash --attach screenshot.png

继续会话（--keep）：
  dscli webchat --keep "第一个问题"            # 继续最近一次会话
  dscli webchat --keep=<会话ID> "继续讨论..."   # 继续指定会话
  dscli webchat --keep=<会话URL> "继续讨论..."  # 继续浏览器中打开的会话
  dscli webchat --keep=list                     # 列出所有已保存会话
每次回复都会把会话 ID 打印到 stderr（格式 keep:<id>），可直接作为 --keep 参数使用。

模型（--model）：
  pro    专家模型（V4 Pro，默认），深度思考
  flash  快速模型（V4 Flash），深度思考 + 智能搜索 + 图片上传
  vision 识图模型（V4 Vision），深度思考 + 图片上传

--keep 且未指定 --model 时保留原会话模型。

角色（--role，与 dscli chat 一致；默认空 = 纯聊天）：
  dscli webchat --role review "review 最近的提交"   # code review 角色
  dscli webchat --role expert "分析这个架构问题"     # 领域专家角色
  dscli webchat --role dev "实现一个功能"            # 开发助手
  dscli webchat "随便聊聊"                           # 默认纯聊天：无角色注入（回复中的 DSML 工具调用仍会执行）
非空角色会前置角色提示词；无论角色与否，DeepSeek Web 回复中的 DSML 工具调用
（read_file / shell / write_file 等）都由 dscli 本地执行并把结果回填到
同一会话（同 code_review 工具）。这是远程模型在本地执行命令的会话：角色会话
开始前会打印警告，且只执行角色配置允许的工具 + 危险命令拦截（rm -rf、
sudo、curl/wget 外传等被拒绝）；仍建议在可信工作目录使用。--role=（空值）即纯聊天：不注入角色
提示词（回复中的 DSML 工具调用仍会执行）。判定规则：回复以 </tool_calls> 结束
即解析并执行其中的 tool_calls（见 toolcall.IsDSMLToolCallEnd）。

附件（--attach，可多次指定，仅 flash/vision 模型支持）：
  dscli webchat --model vision --attach screenshot.png "这张截图说明了什么？"

上传限制：最多 50 个文件、共 100MB，仅识别图片中的文字。`,
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
	webchatCmd.Flags().String("model", "", "聊天模型: pro (专家/V4 Pro), flash (快速/V4 Flash), vision (识图/V4 Vision)；默认 pro，--keep 时保留原模型")
	// --attach accepts any user-readable path (absolute included): the CLI
	// is human-driven and the operator can already read those files. The
	// ask_expert TOOL is LLM-driven and sandboxes paths to the project
	// directory, the user's home (~/ or $HOME absolute), or the system
	// temp dir (/tmp) instead (verifySafePath), since the model is
	// untrusted.
	webchatCmd.Flags().StringSlice("attach", nil, "附件图片路径，可多次指定（仅 flash/vision 模型支持）")
	// --role defaults to "" = plain chat (no role prompt injection; DSML
	// tool calls in replies are still judged and executed when the reply
	// ends with </tool_calls> - see toolcall.IsDSMLToolCallEnd). A
	// non-empty value selects the role prompt template
	// (dev/expert/review/test) before the user message.
	webchatCmd.Flags().String("role", "", "Role: dev (developer), expert (domain expert), review (code review), test (QA engineer)；空 = 纯聊天（不注入角色提示词；回复中的 DSML 工具调用仍会执行）")
}

// webchatOptionsFromFlags builds the HandleWebChat options from parsed CLI
// flags. Extracted as a pure function (no side effects) so tests can lock the
// contract: flag names, defaults, and the pass-through mapping into
// WebChatOptions (Role included).
//
// Role semantics: the flag defaults to "" (plain chat - no role prompt
// injected; DSML tool calls in replies are still judged for execution). A
// non-empty value is passed through unchanged.
func webchatOptionsFromFlags(cmd *cobra.Command) (lp.WebChatOptions, error) {
	keep, err := cmd.Flags().GetString("keep")
	if err != nil {
		return lp.WebChatOptions{}, err
	}
	role, err := cmd.Flags().GetString("role")
	if err != nil {
		return lp.WebChatOptions{}, err
	}
	modelStr, err := cmd.Flags().GetString("model")
	if err != nil {
		return lp.WebChatOptions{}, err
	}
	attach, err := cmd.Flags().GetStringSlice("attach")
	if err != nil {
		return lp.WebChatOptions{}, err
	}
	return lp.WebChatOptions{
		Mode:        lp.Mode(modelStr),
		Attachments: attach,
		Keep:        keep,
		Role:        role,
	}, nil
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

	opts, err := webchatOptionsFromFlags(cmd)
	if err != nil {
		return err
	}
	var result lp.WebChatResult
	startTime := time.Now()

	// A role prompt makes this an agentic consultation: the remote model may
	// reply with DSML tool calls that HandleWebChat executes locally with the
	// user's OS permissions. Say so upfront (stderr, so piped stdout stays
	// clean) - silent local execution from a remote model is the surprise.
	// Role "" (the default) is plain chat: no role prompt, but DSML tool
	// calls in replies are still judged and executed when the reply IS one.
	// The exact tool set comes from the role config (role_configs /
	// roles.DefaultFor) - the same source that gates GetAllTools, so
	// `dscli role update --tools` is the single place that decides it.
	if opts.Role != "" {
		fmt.Fprintf(os.Stderr, "⚠️ 角色 %q 已启用：远程模型回复中的 DSML 工具调用（按角色配置的本地工具）将在本地执行。\n", opts.Role)
	}

	outfmt.Printf("📤 发送到 DeepSeek Web ...\n")
	// HandleWebChat shares the ask_expert entry point: transient server
	// overload and truncation are retried with backoff. A non-empty Role
	// prepends the role prompt; regardless of Role, replies that are judged
	// to be DSML tool calls (<invoke> markup) are executed locally and fed
	// back into the same conversation until the expert finishes.
	result, err = lp.HandleWebChat(ctx, message, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "webchat 失败: %v\n", err)
		return nil
	}

	elapsed := time.Since(startTime)
	outfmt.Printf("📥 收到回复 (%.1fs)\n\n", elapsed.Seconds())
	// 工具循环场景下，每一轮（含最终答复）已由 HandleWebChat 通过
	// outfmt.PrintContent 打印（含 reasoning 与 token）；这里只打印收尾
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
