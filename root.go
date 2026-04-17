package main

import (
	"fmt"

	"gitcode.com/dscli/dscli/internal/config"
	"gitcode.com/dscli/dscli/internal/dsc"
	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/sqlite"
	"github.com/spf13/cobra"
)

var (
	DeepseekClient dsc.Client

	rootCmd = &cobra.Command{
		Use:   "dscli",
		Short: "DeepSeek CLI - 与 DeepSeek API 交互",
		Long: `dscli 是一个命令行工具，用于调用 DeepSeek 的 API。
支持 models、balance、chat 和 fim 四个子命令。

输出选项：
  --mode          输出模式：markdown（Markdown格式）、org（Org模式格式）
  --verbose       打开调试选项，显示详细输出
  --no-color      禁用颜色输出
  --no-timestamp  禁用时间戳显示
  --db            数据库文件路径（默认：~/.dscli/sqlite.db）`,
		PersistentPreRunE: RootPersistentPreRunE,
		Version:           Version,
	}
)

func init() {
	rootCmd.PersistentFlags().String("mode", "markdown", "输出模式：markdown（Markdown格式）、org（Org模式格式）")
	rootCmd.PersistentFlags().Bool("no-color", false, "禁用颜色输出")
	rootCmd.PersistentFlags().Bool("no-timestamp", false, "禁用时间戳显示")
	rootCmd.PersistentFlags().Bool("verbose", false, "打开调试选项（显示详细输出）")
	rootCmd.PersistentFlags().String("db", "", "数据库文件路径（默认：~/.dscli/sqlite.db）")
}

func AddCommand(parent *cobra.Command, child *cobra.Command) *cobra.Command {
	parent.AddCommand(child)
	return child
}

func AddRootCommand(child *cobra.Command) *cobra.Command {
	return AddCommand(rootCmd, child)
}

func RootPersistentPreRunE(cmd *cobra.Command, args []string) (err error) {
	verbose, err := cmd.Flags().GetBool("verbose")
	if err != nil {
		return
	}

	mode, err := cmd.Flags().GetString("mode")
	if err != nil {
		return
	}

	colorEnabled, err := cmd.Flags().GetBool("no-color")
	if err != nil {
		return
	}

	showTimestamp, err := cmd.Flags().GetBool("no-timestamp")
	if err != nil {
		return
	}

	// 设置颜色输出
	outfmt.SetColorEnabled(!colorEnabled) // 注意：--no-color 为 true 时禁用颜色

	// 设置时间戳显示
	outfmt.SetShowTimestamp(!showTimestamp) // 注意：--no-timestamp 为 true 时禁用时间戳

	// 设置详细输出
	outfmt.SetVerbose(verbose)

	// 配置输出系统
	outfmt.SetOutputWriter(cmd.OutOrStdout())
	switch mode {
	case "markdown":
	case "org":
		outfmt.SetOutputMode(mode)
	default:
		err = fmt.Errorf("do not support %s", mode)
		return
	}
	path, err := cmd.Flags().GetString("db")
	if err != nil {
		return
	}
	// 设置数据库路径（如果指定了--db选项）
	if path != "" {
		sqlite.SetDBPath(path)
	}

	// 初始化数据库（确保所有init()函数已执行）
	if _, err := sqlite.OpenDB(); err != nil {
		return fmt.Errorf("数据库初始化失败: %w", err)
	}

	key := config.Get("deepseek-api-key", "")
	if key == "" {
		err = fmt.Errorf("no api key specified")
		return
	}

	url := config.Get("deepseek-base-url", "https://api.deepseek.com")
	DeepseekClient = dsc.NewClient(key, url)
	return nil
}

func RootExecute() error { return rootCmd.Execute() }
