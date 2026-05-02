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
	// 委托 root 的 PersistentPreRunE 进行通用初始化（outfmt、db路径等）
	// recall 不需要 API key，绕过其检查：临时将 key 设置后再让 root 检查
	// 但 root PreRunE 会因缺少 key 而报错？不，由于 recall 有自己的 PersistentPreRunE，
	// root 的 PersistentPreRunE 不会自动执行。这里手动调用以获取 outfmt、db 等初始化。
	//
	// 注意：必须防止 root PreRunE 中的 API key 检查导致错误。
	// 采用更安全的方式：直接继承必要的初始化逻辑，不调用 root PreRunE。
	// （cobra 不支持 PreRunE 链，子 PersistentPreRunE 会覆盖父的）

	// 设置输出模式（复用 root 的配置逻辑）
	mode, err := cmd.Flags().GetString("mode")
	if err != nil {
		return
	}
	// mode 未显式设置时使用默认值
	if mode == "" {
		mode = "markdown"
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

	// 注意：不在 PreRun 中打开数据库连接，由 SearchMessages 按需打开
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

	// 从 args 中提取关键词
	// 注意：引号包裹的短语会被 cobra 作为一个 arg 传递，此处用 Fields 切分
	// 意味着 "Go 错误处理" 会被拆成 ["Go", "错误处理"] 两个独立关键词
	// 如需精确短语搜索，未来可添加 --exact 标志
	var keywords []string
	for _, arg := range args {
		for _, kw := range strings.Fields(arg) {
			kw = strings.TrimSpace(kw)
			if kw != "" {
				keywords = append(keywords, kw)
			}
		}
	}

	results, err := recall.SearchMessages(keywords, days, limit)
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