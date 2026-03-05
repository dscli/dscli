package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var (
	mode          string
	logLevel      string
	colorEnabled  bool
	showTimestamp bool
	verbose       bool

	rootCmd = &cobra.Command{
		Use:   "dscli",
		Short: "DeepSeek CLI - 与 DeepSeek API 交互",
		Long: `dscli 是一个命令行工具，用于调用 DeepSeek 的 API。
支持 models、balance、chat 和 fim 四个子命令。

输出选项：
  --mode          输出模式：markdown（Markdown格式）、org（Org模式格式）
  --log-level     日志级别：debug、info、warn、error、fatal
  --no-color      禁用颜色输出
  --no-timestamp  禁用时间戳显示
  --verbose       显示详细输出`,
		PersistentPreRunE: RootPreRunE,
		Version:           Version,
	}
)

func init() {
	rootCmd.PersistentFlags().StringVar(&mode, "mode", "markdown", "输出模式：markdown（Markdown格式）、org（Org模式格式）")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "日志级别：debug、info、warn、error、fatal")
	rootCmd.PersistentFlags().BoolVar(&colorEnabled, "no-color", false, "禁用颜色输出")
	rootCmd.PersistentFlags().BoolVar(&showTimestamp, "no-timestamp", false, "禁用时间戳显示")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "显示详细输出")
}

func AddCommand(parent *cobra.Command, child *cobra.Command) *cobra.Command {
	parent.AddCommand(child)
	return child
}

func AddRootCommand(child *cobra.Command) *cobra.Command {
	return AddCommand(rootCmd, child)
}

func RootPreRunE(cmd *cobra.Command, args []string) (err error) {
	// 配置输出系统
	configureOutput()
	SetOutputWriter(cmd.OutOrStdout())
	switch mode {
	case "markdown":
	case "org":
		SetOutputMode(mode)
	default:
		err = fmt.Errorf("do not support %s", mode)
		return
	}

	key := Getenv("DEEPSEEK_API_KEY", "")
	if key == "" {
		err = fmt.Errorf("no api key specified")
		return
	}

	url := os.Getenv("DEEPSEEK_BASE_URL")
	if url == "" {
		url = "https://api.deepseek.com" // 默认值
	}

	DeepseekClient = NewClient(key, url)
	return nil
}

// configureOutput 配置输出系统
func configureOutput() {
	// 设置日志级别
	switch strings.ToLower(logLevel) {
	case "debug":
		SetLogLevel(LogLevelDebug)
	case "info":
		SetLogLevel(LogLevelInfo)
	case "warn":
		SetLogLevel(LogLevelWarn)
	case "error":
		SetLogLevel(LogLevelError)
	case "fatal":
		SetLogLevel(LogLevelFatal)
	default:
		SetLogLevel(LogLevelInfo)
	}

	// 设置颜色输出
	SetColorEnabled(!colorEnabled) // 注意：--no-color 为 true 时禁用颜色

	// 设置时间戳显示
	SetShowTimestamp(!showTimestamp) // 注意：--no-timestamp 为 true 时禁用时间戳

	// 设置详细输出
	SetVerbose(verbose)
}

func RootExecute() error { return rootCmd.Execute() }
