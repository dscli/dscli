package main

import (
	"fmt"
	"slices"
	"strconv"

	"gitcode.com/dscli/dscli/internal/context"
	"github.com/spf13/cobra"
)

func init() {
	historyCmd := AddRootCommand(&cobra.Command{
		Use:               "history",
		PersistentPreRunE: historyPreRunE,
	})
	_ = AddCommand(historyCmd, &cobra.Command{
		Use:  "list",
		RunE: historyListRunE,
	})
	_ = AddCommand(historyCmd, &cobra.Command{
		Use:  "load",
		RunE: historyLoadRunE,
	})
	_ = AddCommand(historyCmd, &cobra.Command{
		Use:  "update",
		Args: cobra.ExactArgs(1),
		RunE: historyUpdateRunE,
	})
	_ = AddCommand(historyCmd, &cobra.Command{
		Use:  "show",
		Args: cobra.ExactArgs(1),
		RunE: historyShowRunE,
	})

	editCmd := AddCommand(historyCmd, &cobra.Command{
		Use:  "edit",
		Args: cobra.ExactArgs(1),
		RunE: historyEditRunE,
	})

	historyCmd.PersistentFlags().Int("histsize", 32, "history size")
	historyCmd.PersistentFlags().String("filter", "all", "filter true, false, all")
	historyCmd.PersistentFlags().String("model", ModelDeepseekChat, "model")
	editCmd.Flags().String("column", "content", "column name to edit, default content, others like tool_calls can be edited too.")
}

func historyShowRunE(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()
	id, err := strconv.Atoi(args[0])
	if err != nil {
		return
	}
	message, err := ShowMessage(ctx, int64(id))
	if err != nil {
		return
	}
	wrt := NewTabwrt()
	defer wrt.Flush()
	wrt.Println("ID", fmt.Sprint(message.ID))
	wrt.Println("ModelID", fmt.Sprint(message.ModelID))
	wrt.Println("SessionID", fmt.Sprint(message.SessionID))
	wrt.Println("Role", message.Role)
	wrt.Println("ToolCallID", message.ToolCallID)
	wrt.Println("ToolCalls", ToSQLNullString(message.ToolCalls).String)
	wrt.Println("ReasoningContent", message.ReasoningContent)
	wrt.Println("Content", message.Content)
	return
}

func historyEditRunE(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()
	id, err := strconv.Atoi(args[0])
	if err != nil {
		return
	}
	column, err := cmd.Flags().GetString("column")
	if err != nil {
		return
	}
	if !slices.Contains([]string{"content", "tool_calls"}, column) {
		err = fmt.Errorf("not support %s", column)
		return
	}

	message, err := ShowMessage(ctx, int64(id))
	if err != nil {
		return
	}
	switch column {
	case "content":
		content := message.Content
		content, err = OpenEditor(ctx, content)
		if err != nil {
			return
		}
		err = UpdateContent(ctx, int64(id), content)
		if err != nil {
			return
		}
	case "tool_calls":
		tcs := message.ToolCalls
		if len(tcs) == 0 {
			tcs = append(tcs, ToolCall{})
		}
		tc := tcs[0]
		arguments := tc.Function.Arguments
		arguments, err = OpenEditor(ctx, arguments)
		if err != nil {
			return
		}
		tc.Function.Arguments = arguments
		tcs = []ToolCall{tc}
		err = UpdateToolCalls(ctx, int64(id), tcs)
		if err != nil {
			return
		}
	}
	return
}

func historyUpdateRunE(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()
	id, err := strconv.Atoi(args[0])
	if err != nil {
		return
	}
	return UpdateHistory(ctx, int64(id))
}

func historyPreRunE(cmd *cobra.Command, args []string) (err error) {
	err = chatCommonPreRunE(cmd, args)
	if err != nil {
		return
	}
	ctx := cmd.Context()
	histsize, err := cmd.Flags().GetInt("histsize")
	if err != nil {
		return
	}
	ctx = context.WithValue(ctx, context.HistSizeKey, histsize)
	cmd.SetContext(ctx)
	return
}

func historyListRunE(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()
	history, err := ListHistory(ctx)
	if err != nil {
		return
	}
	filter, err := cmd.Flags().GetString("filter")
	wrt := NewTabwrt()
	defer wrt.Flush()
	for _, hist := range history {
		switch filter {
		case "all":
			wrt.Println(fmt.Sprint(hist.ID), hist.Role, hist.ToolCallID, ToolCallsID(hist.ToolCalls), fmt.Sprint(hist.OK))
		case "true":
			if hist.OK {
				wrt.Println(fmt.Sprint(hist.ID), hist.Role, hist.ToolCallID, ToolCallsID(hist.ToolCalls), fmt.Sprint(hist.OK))
			}
		default:
			if !hist.OK {
				wrt.Println(fmt.Sprint(hist.ID), hist.Role, hist.ToolCallID, ToolCallsID(hist.ToolCalls), fmt.Sprint(hist.OK))
			}
		}
	}
	return
}

func historyLoadRunE(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()
	history, err := LoadHistory(ctx)
	if err != nil {
		return
	}
	filter, err := cmd.Flags().GetString("filter")
	wrt := NewTabwrt()
	defer wrt.Flush()
	for i, hist := range history[0 : len(history)-1] {
		role := hist.Role
		pass := true
		if role == "assistant" {
			toolCallID := ToolCallsID(hist.ToolCalls)
			if toolCallID != "" {
				nextToolCallID := history[i+1].ToolCallID
				if toolCallID != nextToolCallID {
					pass = false
				}
			}
		}
		switch filter {
		case "all":
			wrt.Println(fmt.Sprint(hist.ID), hist.Role, hist.ToolCallID, ToolCallsID(hist.ToolCalls), fmt.Sprint(pass))
		case "true":
			if pass {
				wrt.Println(fmt.Sprint(hist.ID), hist.Role, hist.ToolCallID, ToolCallsID(hist.ToolCalls), fmt.Sprint(pass))
			}
		default:
			if !pass {
				wrt.Println(fmt.Sprint(hist.ID), hist.Role, hist.ToolCallID, ToolCallsID(hist.ToolCalls), fmt.Sprint(pass))
			}

		}
	}

	pass := true
	hist := history[len(history)-1]
	switch filter {
	case "all":
		wrt.Println(fmt.Sprint(hist.ID), hist.Role, hist.ToolCallID, ToolCallsID(hist.ToolCalls), fmt.Sprint(pass))
	case "true":
		if pass {
			wrt.Println(fmt.Sprint(hist.ID), hist.Role, hist.ToolCallID, ToolCallsID(hist.ToolCalls), fmt.Sprint(pass))
		}
	default:
		if !pass {
			wrt.Println(fmt.Sprint(hist.ID), hist.Role, hist.ToolCallID, ToolCallsID(hist.ToolCalls), fmt.Sprint(pass))
		}
	}
	return
}
