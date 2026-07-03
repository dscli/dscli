package main

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/nanjj/clog"

	"github.com/dscli/dscli/internal/config"
	"github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/editor"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/prompt"
	"github.com/spf13/cobra"
)

func init() {
	historyCmd := AddRootCommand(&cobra.Command{
		Use:               "history",
		Short:             "历史消息管理",
		PersistentPreRunE: historyPreRunE,
	})
	listCmd := AddCommand(historyCmd, &cobra.Command{
		Use:   "list",
		Short: "List history messages",
		RunE:  historyListRunE,
	})
	listCmd.Flags().Bool("json", false, "Output in JSON format (with full content)")
	_ = AddCommand(historyCmd, &cobra.Command{
		Use:   "load",
		Short: "Load and validate history messages",
		RunE:  historyLoadRunE,
	})
	_ = AddCommand(historyCmd, &cobra.Command{
		Use:   "update",
		Short: "Mark message as history (update its ok status)",
		Args:  cobra.ExactArgs(1),
		RunE:  historyUpdateRunE,
	})
	recentCmd := AddCommand(historyCmd, &cobra.Command{
		Use:   "recent",
		Short: "List recent messages in current session (table format)",
		RunE:  historyRecentRunE,
	})
	recentCmd.Flags().Int("limit", 20, "Return last N messages (default 20, max 100)")
	recentCmd.Flags().Int64("start", 0, "Start from specified message ID going backward (0=latest)")
	_ = AddCommand(historyCmd, &cobra.Command{
		Use:   "show",
		Short: "Show full message details",
		Args:  cobra.ExactArgs(1),
		RunE:  historyShowRunE,
	})

	editCmd := AddCommand(historyCmd, &cobra.Command{
		Use:   "edit",
		Short: "Edit content or tool_calls of a message",
		Args:  cobra.ExactArgs(1),
		RunE:  historyEditRunE,
	})

	recallCmd := AddCommand(historyCmd, &cobra.Command{
		Use:   "recall [keywords...]",
		Short: "Search message content",
		Long: `Search history messages, matching user messages and assistant summaries (non-tool-call assistant messages).

Examples:
  dscli history recall "Go error handling"
  dscli history recall goroutine channel`,
		Args: cobra.MinimumNArgs(1),
		RunE: recallSearchRunE,
	})

	notesCmd := AddCommand(historyCmd, &cobra.Command{
		Use:   "notes",
		Short: "List conversation notes for current project",
		Long: `List recently saved conversation notes for the current project.

Notes are cross-session memory clues that can be saved via the note tool.

Examples:
  dscli history notes
  dscli history notes --days 7`,
		RunE: historyNotesRunE,
	})

	recallCmd.Flags().Int("days", 30, "Search messages from last N days")
	recallCmd.Flags().Int("limit", 5, "Max number of results")

	notesCmd.Flags().Int("days", 30, "Load notes from last N days")

	historyCmd.PersistentFlags().Int("histsize", 32, "history size")
	historyCmd.PersistentFlags().String("role", "dev", "role: dev, expert, review, test (QA engineer)")
	historyCmd.PersistentFlags().String("filter", "all", "filter true, false, all")
	editCmd.Flags().String("column", "content", "column name to edit, default content, others like tool_calls can be edited too.")

	_ = AddCommand(historyCmd, &cobra.Command{
		Use:   "move <project_id>",
		Short: "将当前项目的历史消息移到另一个项目",
		Long: `将当前项目的所有历史消息（messages）移动到指定项目。

项目 ID 可以通过 dscli project list 查看。

示例:
  dscli history move 7    # 将当前项目的消息移到项目 7`,
		Args: cobra.ExactArgs(1),
		RunE: historyMoveRunE,
	})
}

func historyPreRunE(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()

	ctx = context.WithValue(ctx, context.CurrentModelNameKey, context.ModelDeepseekChat)
	ctx = context.WithValue(ctx, context.CurrentModelIDKey, DeepseekChat)

	role, err := cmd.Flags().GetString("role")
	if err != nil {
		return err
	}

	if role == "" {
		role = "dev"
	}

	ctx = context.WithValue(ctx, context.CurrentRoleKey, role)

	contextWindow := config.GetInt("context-window", 1000000)
	ctx = context.WithValue(ctx, context.LeftTokensKey, contextWindow)

	histsize, err := cmd.Flags().GetInt("histsize")
	if err != nil {
		return err
	}
	ctx = context.WithValue(ctx, context.HistSizeKey, histsize)
	cmd.SetContext(ctx)
	return nil
}

func historyShowRunE(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()
	span, ctx := clog.StartSpanFromContext(ctx, "historyShowRunE")
	defer span.Finish()
	id, err := strconv.Atoi(args[0])
	if err != nil {
		return err
	}
	message, err := prompt.ShowMessage(ctx, int64(id))
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取消息失败: %v\n", err)
		return nil
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
	return nil
}

func historyEditRunE(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()
	span, ctx := clog.StartSpanFromContext(ctx, "historyEditRunE")
	defer span.Finish()
	id, err := strconv.Atoi(args[0])
	if err != nil {
		return err
	}
	column, err := cmd.Flags().GetString("column")
	if err != nil {
		return err
	}
	if !slices.Contains([]string{"content", "tool_calls"}, column) {
		return fmt.Errorf("not support %s", column)
	}

	message, err := prompt.ShowMessage(ctx, int64(id))
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取消息失败: %v\n", err)
		return nil
	}
	switch column {
	case "content":
		content := message.Content
		content, err = editor.OpenEditor(ctx, content)
		if err != nil {
			fmt.Fprintf(os.Stderr, "编辑内容失败: %v\n", err)
			return nil
		}
		err = prompt.UpdateContent(ctx, int64(id), content)
		if err != nil {
			fmt.Fprintf(os.Stderr, "更新内容失败: %v\n", err)
			return nil
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
			fmt.Fprintf(os.Stderr, "编辑参数失败: %v\n", err)
			return nil
		}
		tc.Function.Arguments = arguments
		tcs = []prompt.ToolCall{tc}
		err = prompt.UpdateToolCalls(ctx, int64(id), tcs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "更新工具调用失败: %v\n", err)
			return nil
		}
	}
	return nil
}

func historyUpdateRunE(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()
	span, ctx := clog.StartSpanFromContext(ctx, "historyUpdateRunE")
	defer span.Finish()
	id, err := strconv.Atoi(args[0])
	if err != nil {
		return err
	}
	if err := prompt.UpdateHistory(ctx, int64(id)); err != nil {
		fmt.Fprintf(os.Stderr, "更新历史状态失败: %v\n", err)
		return nil
	}
	return nil
}

// historyMoveRunE handles "dscli history move <project_id>".
func historyMoveRunE(cmd *cobra.Command, args []string) error {
	span, ctx := clog.StartSpanFromContext(cmd.Context(), "historyMoveRunE")
	defer span.Finish()
	projectID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil || projectID <= 0 {
		return fmt.Errorf("无效的 project_id: %s（需要正整数）", args[0])
	}

	if err := prompt.MoveMessages(ctx, projectID); err != nil {
		fmt.Fprintf(os.Stderr, "移动历史消息失败: %v\n", err)
		return nil
	}

	return nil
}

func historyRecentRunE(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()
	span, ctx := clog.StartSpanFromContext(ctx, "historyRecentRunE")
	defer span.Finish()

	limit, err := cmd.Flags().GetInt("limit")
	if err != nil {
		return err
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	startID, err := cmd.Flags().GetInt64("start")
	if err != nil {
		return err
	}

	msgs, err := prompt.RecentMessages(ctx, limit, startID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取最近消息失败: %v\n", err)
		return nil
	}

	if len(msgs) == 0 {
		if startID > 0 {
			outfmt.Printf("从 #%d 往前没有更多消息了。\n", startID)
		} else {
			outfmt.Println("当前会话没有历史消息。")
		}
		return nil
	}

	wrt := outfmt.NewTabwrt()
	defer wrt.Flush()
	for _, m := range msgs {
		role := "用户"
		if m.Role == "assistant" {
			role = "助手"
		}
		wrt.Println(
			fmt.Sprint(m.ID),
			prompt.FormatTime(m.CreatedAt),
			role,
			prompt.Truncate(m.Content, 80),
		)
	}
	return nil
}

func historyListRunE(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()
	span, ctx := clog.StartSpanFromContext(ctx, "historyListRunE")
	defer span.Finish()
	history, err := prompt.ListHistory(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "列出历史消息失败: %v\n", err)
		return nil
	}

	useJSON, _ := cmd.Flags().GetBool("json")
	if useJSON {
		type histEntry struct {
			ID               int64             `json:"id"`
			Role             string            `json:"role"`
			Content          string            `json:"content"`
			ReasoningContent string            `json:"reasoning_content,omitempty"`
			ToolCallID       string            `json:"tool_call_id,omitempty"`
			ToolCalls        []prompt.ToolCall `json:"tool_calls,omitempty"`
			CreatedAt        string            `json:"created_at"`
		}
		var result []histEntry
		for _, hist := range history {
			entry := histEntry{
				ID:               hist.ID,
				Role:             hist.Role,
				Content:          hist.Content,
				ReasoningContent: hist.ReasoningContent,
				ToolCallID:       hist.ToolCallID,
				CreatedAt:        hist.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			}
			if len(hist.ToolCalls) > 0 {
				entry.ToolCalls = hist.ToolCalls
			}
			result = append(result, entry)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(result)
		return nil
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
	return nil
}
func historyLoadRunE(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()
	span, ctx := clog.StartSpanFromContext(ctx, "historyLoadRunE")
	defer span.Finish()
	history, err := prompt.LoadHistory(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载历史消息失败: %v\n", err)
		return nil
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
	return nil
}

func recallSearchRunE(cmd *cobra.Command, args []string) (err error) {
	span, ctx := clog.StartSpanFromContext(cmd.Context(), "recallSearchRunE")
	defer span.Finish()
	days, err := cmd.Flags().GetInt("days")
	if err != nil {
		return err
	}

	limit, err := cmd.Flags().GetInt("limit")
	if err != nil {
		return err
	}

	var keywords []string
	for _, arg := range args {
		for kw := range strings.FieldsSeq(arg) {
			kw = strings.TrimSpace(kw)
			if kw != "" {
				keywords = append(keywords, kw)
			}
		}
	}

	results, err := prompt.SearchMessages(ctx, keywords, days, limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "搜索历史消息失败: %v\n", err)
		return nil
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

func historyNotesRunE(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()
	span, ctx := clog.StartSpanFromContext(ctx, "historyNotesRunE")
	defer span.Finish()
	days, err := cmd.Flags().GetInt("days")
	if err != nil {
		return err
	}
	notes, err := prompt.LoadNotes(ctx, days)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载笔记失败: %v\n", err)
		return nil
	}
	if len(notes) == 0 {
		outfmt.Println("暂无笔记。")
		return nil
	}
	wrt := outfmt.NewTabwrt()
	defer wrt.Flush()
	for _, n := range notes {
		wrt.Println(prompt.FormatTime(n.CreatedAt), n.Content)
	}
	return nil
}
