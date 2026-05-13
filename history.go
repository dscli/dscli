package main

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"gitcode.com/dscli/dscli/internal/config"
	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/editor"
	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/prompt"
	"github.com/spf13/cobra"
)

func init() {
	historyCmd := AddRootCommand(&cobra.Command{
		Use:               "history",
		Short:             "历史消息管理",
		PersistentPreRunE: historyPreRunE,
	})
	_ = AddCommand(historyCmd, &cobra.Command{
		Use:   "list",
		Short: "列出历史消息",
		RunE:  historyListRunE,
	})
	_ = AddCommand(historyCmd, &cobra.Command{
		Use:   "load",
		Short: "加载并校验历史消息中助手回复与工具调用的匹配关系",
		RunE:  historyLoadRunE,
	})
	_ = AddCommand(historyCmd, &cobra.Command{
		Use:   "update",
		Short: "将指定消息标记为历史（更新其 ok 状态）",
		Args:  cobra.ExactArgs(1),
		RunE:  historyUpdateRunE,
	})
	_ = AddCommand(historyCmd, &cobra.Command{
		Use:   "show",
		Short: "显示指定消息的完整信息",
		Args:  cobra.ExactArgs(1),
		RunE:  historyShowRunE,
	})

	editCmd := AddCommand(historyCmd, &cobra.Command{
		Use:   "edit",
		Short: "编辑指定消息的 content 或 tool_calls 字段",
		Args:  cobra.ExactArgs(1),
		RunE:  historyEditRunE,
	})

	recallCmd := AddCommand(historyCmd, &cobra.Command{
		Use:   "recall [keywords...]",
		Short: "搜索消息内容（空格分隔多关键词，OR逻辑，匹配任一即返回）",
		Long: `搜索历史消息，只匹配 user 消息和助手总结（无工具调用的 assistant 消息）。

示例：
  dscli recall search "Go 错误处理"
  dscli recall search goroutine channel`,
		Args: cobra.MinimumNArgs(1),
		RunE: recallSearchRunE,
	})

	recallCmd.Flags().Int("days", 30, "搜索最近N天的消息")
	recallCmd.Flags().Int("limit", 5, "返回结果数量上限")

	historyCmd.PersistentFlags().Int("histsize", 32, "history size")
	historyCmd.PersistentFlags().String("role", "dev", "role: dev, expert, review")
	historyCmd.PersistentFlags().String("filter", "all", "filter true, false, all")
	historyCmd.PersistentFlags().String("model", context.ModelDeepseekChat, "model")
	editCmd.Flags().String("column", "content", "column name to edit, default content, others like tool_calls can be edited too.")
}

func historyShowRunE(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()
	id, err := strconv.Atoi(args[0])
	if err != nil {
		return err
	}
	message, err := prompt.ShowMessage(ctx, int64(id))
	if err != nil {
		return err
	}
	wrt := outfmt.NewTabwrt()
	defer wrt.Flush()
	wrt.Println("ID", fmt.Sprint(message.ID))
	wrt.Println("ModelID", fmt.Sprint(message.ModelID))
	wrt.Println("SessionID", fmt.Sprint(message.SessionID))
	wrt.Println("Role", message.Role)
	wrt.Println("ToolCallID", message.ToolCallID)
	wrt.Println("ToolCalls", prompt.ToSQLNullString(message.ToolCalls).String)
	wrt.Println("ReasoningContent", message.ReasoningContent)
	wrt.Println("Content", message.Content)
	return err
}

func historyEditRunE(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()
	id, err := strconv.Atoi(args[0])
	if err != nil {
		return err
	}
	column, err := cmd.Flags().GetString("column")
	if err != nil {
		return err
	}
	if !slices.Contains([]string{"content", "tool_calls"}, column) {
		err = fmt.Errorf("not support %s", column)
		return err
	}

	message, err := prompt.ShowMessage(ctx, int64(id))
	if err != nil {
		return err
	}
	switch column {
	case "content":
		content := message.Content
		content, err = editor.OpenEditor(ctx, content)
		if err != nil {
			return err
		}
		err = prompt.UpdateContent(ctx, int64(id), content)
		if err != nil {
			return err
		}
	case "tool_calls":
		tcs := message.ToolCalls
		if len(tcs) == 0 {
			tcs = append(tcs, prompt.ToolCall{})
		}
		tc := tcs[0]
		arguments := tc.Function.Arguments
		arguments, err = editor.OpenEditor(ctx, arguments)
		if err != nil {
			return err
		}
		tc.Function.Arguments = arguments
		tcs = []prompt.ToolCall{tc}
		err = prompt.UpdateToolCalls(ctx, int64(id), tcs)
		if err != nil {
			return err
		}
	}
	return err
}

func historyUpdateRunE(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()
	id, err := strconv.Atoi(args[0])
	if err != nil {
		return err
	}
	return prompt.UpdateHistory(ctx, int64(id))
}

func historyPreRunE(cmd *cobra.Command, args []string) (err error) {
	model, err := cmd.Flags().GetString("model")
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	var modelID int64
	switch model {
	case context.ModelDeepseekChat:
		modelID = DeepseekChat
	case context.ModelDeepseekReasoner:
		modelID = DeepseekReasoner
	default:
		err = fmt.Errorf("do not support %s", model)
		if verbose, _ := cmd.Flags().GetBool("verbose"); verbose {
			fmt.Printf("[DEBUG] ChatPreRunE: unsupported model error: %v\n", err)
		}
		return err
	}

	ctx = context.WithValue(ctx, context.CurrentModelNameKey, model)
	ctx = context.WithValue(ctx, context.CurrentModelIDKey, modelID)
	// 读取 --role 标志并存入 context
	role, err := cmd.Flags().GetString("role")
	if err != nil {
		return err
	}

	if role == "" {
		role = "dev"
	}

	ctx = context.WithValue(ctx, context.CurrentRoleKey, role)

	// 从配置读取上下文窗口大小（默认 1,000,000，对应 DeepSeek V4 百万 token 上下文）。
	// 此值用作历史消息 token 预算的上限，实际截断主要由 --histsize 控制。
	// 配置文件 key: context-window，环境变量: CONTEXT_WINDOW。
	contextWindow := config.GetInt("context-window", 1000000)
	ctx = context.WithValue(ctx, context.LeftTokensKey, contextWindow)

	histsize, err := cmd.Flags().GetInt("histsize")
	if err != nil {
		return err
	}
	ctx = context.WithValue(ctx, context.HistSizeKey, histsize)
	cmd.SetContext(ctx)
	return err
}

func historyListRunE(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()
	history, err := prompt.ListHistory(ctx)
	if err != nil {
		return err
	}

	filter, err := cmd.Flags().GetString("filter")
	if err != nil {
		return err
	}

	wrt := outfmt.NewTabwrt()
	defer wrt.Flush()
	for _, hist := range history {
		switch filter {
		case "all":
			wrt.Println(fmt.Sprint(hist.ID), hist.Role, hist.ToolCallID, prompt.ToolCallsID(hist.ToolCalls), fmt.Sprint(hist.OK))
		case "true":
			if hist.OK {
				wrt.Println(fmt.Sprint(hist.ID), hist.Role, hist.ToolCallID, prompt.ToolCallsID(hist.ToolCalls), fmt.Sprint(hist.OK))
			}
		default:
			if !hist.OK {
				wrt.Println(fmt.Sprint(hist.ID), hist.Role, hist.ToolCallID, prompt.ToolCallsID(hist.ToolCalls), fmt.Sprint(hist.OK))
			}
		}
	}
	return err
}

func historyLoadRunE(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()
	history, err := prompt.LoadHistory(ctx)
	if err != nil {
		return err
	}
	filter, err := cmd.Flags().GetString("filter")
	if err != nil {
		return err
	}
	wrt := outfmt.NewTabwrt()
	defer wrt.Flush()
	for i, hist := range history[0 : len(history)-1] {
		role := hist.Role
		pass := true
		if role == "assistant" {
			toolCallID := prompt.ToolCallsID(hist.ToolCalls)
			if toolCallID != "" {
				nextToolCallID := history[i+1].ToolCallID
				if toolCallID != nextToolCallID {
					pass = false
				}
			}
		}
		switch filter {
		case "all":
			wrt.Println(fmt.Sprint(hist.ID), hist.Role, hist.ToolCallID, prompt.ToolCallsID(hist.ToolCalls), fmt.Sprint(pass))
		case "true":
			if pass {
				wrt.Println(fmt.Sprint(hist.ID), hist.Role, hist.ToolCallID, prompt.ToolCallsID(hist.ToolCalls), fmt.Sprint(pass))
			}
		default:
			if !pass {
				wrt.Println(fmt.Sprint(hist.ID), hist.Role, hist.ToolCallID, prompt.ToolCallsID(hist.ToolCalls), fmt.Sprint(pass))
			}

		}
	}

	pass := true
	hist := history[len(history)-1]
	switch filter {
	case "all":
		wrt.Println(fmt.Sprint(hist.ID), hist.Role, hist.ToolCallID, prompt.ToolCallsID(hist.ToolCalls), fmt.Sprint(pass))
	case "true":
		if pass {
			wrt.Println(fmt.Sprint(hist.ID), hist.Role, hist.ToolCallID, prompt.ToolCallsID(hist.ToolCalls), fmt.Sprint(pass))
		}
	default:
		if !pass {
			wrt.Println(fmt.Sprint(hist.ID), hist.Role, hist.ToolCallID, prompt.ToolCallsID(hist.ToolCalls), fmt.Sprint(pass))
		}
	}
	return err
}

func recallSearchRunE(cmd *cobra.Command, args []string) (err error) {
	days, err := cmd.Flags().GetInt("days")
	if err != nil {
		return err
	}

	limit, err := cmd.Flags().GetInt("limit")
	if err != nil {
		return err
	}

	// 从 args 中提取关键词
	// 注意：引号包裹的短语会被 cobra 作为一个 arg 传递，此处用 Fields 切分
	// 意味着 "Go 错误处理" 会被拆成 ["Go", "错误处理"] 两个独立关键词
	// 如需精确短语搜索，未来可添加 --exact 标志
	var keywords []string
	for _, arg := range args {
		for kw := range strings.FieldsSeq(arg) {
			kw = strings.TrimSpace(kw)
			if kw != "" {
				keywords = append(keywords, kw)
			}
		}
	}

	ctx := cmd.Context()
	results, err := prompt.SearchMessages(ctx, keywords, days, limit)
	if err != nil {
		return err
	}

	if len(results) == 0 {
		outfmt.Println("没有找到匹配的消息。")
		return nil
	}

	wrt := outfmt.NewTabwrt()
	defer wrt.Flush()

	for _, r := range results {
		roleLabel := "🙋 用户"
		if r.Message.Role == "assistant" {
			roleLabel = "🤖 助手"
		}
		timeStr := prompt.FormatTime(r.Message.CreatedAt)
		preview := prompt.Truncate(r.Message.Content, 120)

		wrt.Println(
			timeStr,
			roleLabel,
			r.ProjectPath,
			preview,
		)
	}

	return nil
}
