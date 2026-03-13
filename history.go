package main

import (
	"context"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

func init() {
	historyCmd := AddRootCommand(&cobra.Command{
		Use: "history",
	})
	listCmd := AddCommand(historyCmd, &cobra.Command{
		Use:     "list",
		PreRunE: historyListPreRunE,
		RunE:    historyListRunE,
	})
	_ = AddCommand(historyCmd, &cobra.Command{
		Use:  "update",
		Args: cobra.ExactArgs(1),
		RunE: historyUpdateRunE,
	})
	listCmd.Flags().Int("histsize", 32, "history size")
	listCmd.Flags().String("filter", "all", "filter true, false, all")
	listCmd.Flags().String("model", ModelDeepseekChat, "model")
}

func historyUpdateRunE(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()
	id, err := strconv.Atoi(args[0])
	if err != nil {
		return
	}
	return UpdateHistory(ctx, int64(id))
}

func historyListPreRunE(cmd *cobra.Command, args []string) (err error) {
	err = chatCommonPreRunE(cmd, args)
	if err != nil {
		return
	}
	ctx := cmd.Context()
	histsize, err := cmd.Flags().GetInt("histsize")
	if err != nil {
		return
	}
	ctx = context.WithValue(ctx, HistSize, histsize)
	cmd.SetContext(ctx)
	return
}

func historyListRunE(cmd *cobra.Command, args []string) (err error) {
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
	return
}
