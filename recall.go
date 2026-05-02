package main

import (
	"fmt"
	"strings"

	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/sqlite"
	"gitcode.com/dscli/dscli/internal/toolcall/recall"
	"github.com/spf13/cobra"
)

func init() {
	recallCmd := AddRootCommand(&cobra.Command{
		Use:               "recall",
		Short:             "回忆历史对话，搜索消息内容",
		PersistentPreRunE: recallPreRunE,
	})

	_ = AddCommand(recallCmd, &cobra.Command{
		Use:   "search [keywords...]",
		Short: "搜索消息内容（空格分隔多关键词，OR逻辑，匹配任一即返回）",
		Long: `搜索历史消息，只匹配 user 消息和助手总结（无工具调用的 assistant 消息）。

示例：
  dscli recall search "Go 错误处理"
  dscli recall search goroutine channel`,
		Args: cobra.MinimumNArgs(1),
		RunE: recallSearchRunE,
	})

	recallCmd.PersistentFlags().Int("days", 30, "搜索最近N天的消息")
	recallCmd.PersistentFlags().Int("limit", 5, "返回结果数量上限")
}

func recallPreRunE(cmd *cobra.Command, args []string) (err error) {
	// 设置输出模式（复用root的配置逻辑）
	mode, err := cmd.Flags().GetString("mode")
	if err != nil {
		return
	}
	switch mode {
	case "markdown":
	case "org":
		outfmt.SetOutputMode(mode)
	default:
		return fmt.Errorf("不支持的模式: %s", mode)
	}

	noColor, _ := cmd.Flags().GetBool("no-color")
	outfmt.SetColorEnabled(!noColor)

	noTimestamp, _ := cmd.Flags().GetBool("no-timestamp")
	outfmt.SetShowTimestamp(!noTimestamp)

	verbose, _ := cmd.Flags().GetBool("verbose")
	outfmt.SetVerbose(verbose)
	outfmt.SetOutputWriter(cmd.OutOrStdout())

	// 初始化数据库路径
	dbPath, _ := cmd.Flags().GetString("db")
	if dbPath != "" {
		sqlite.SetDBPath(dbPath)
	}

	// 初始化数据库（recall不需要API key）
	if _, err := sqlite.OpenDB(); err != nil {
		return fmt.Errorf("数据库初始化失败: %w", err)
	}

	return nil
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

	// 从 args 中提取关键词（cobra 已切分好，但多词可能在同一个 arg 中）
	var keywords []string
	for _, arg := range args {
		for _, kw := range strings.Fields(arg) {
			kw = strings.TrimSpace(kw)
			if kw != "" {
				keywords = append(keywords, kw)
			}
		}
	}

	results, err := recall.SearchMessages(keywords, days, limit, false, 0)
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
		timeStr := recall.FormatTime(r.Message.CreatedAt)
		preview := recall.Truncate(r.Message.Content, 120)

		wrt.Println(
			timeStr,
			roleLabel,
			r.ProjectPath,
			preview,
		)
	}

	return nil
}
