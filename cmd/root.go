package cmd

import (
	"fmt"
	"os"

	"gitcode.com/nanjunjie/dscli/internal/api"
	"github.com/spf13/cobra"
)

var (
	apiKey   string
	baseURL  string
	debug    bool
	client   *api.Client
)

var rootCmd = &cobra.Command{
	Use:   "dscli",
	Short: "DeepSeek CLI - 与 DeepSeek API 交互",
	Long: `dscli 是一个命令行工具，用于调用 DeepSeek 的 API。
支持 models、balance、chat 和 fim 四个子命令。`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// 初始化 API 客户端
		key := apiKey
		if key == "" {
			key = os.Getenv("DEEPSEEK_API_KEY")
		}
		if key == "" {
			fmt.Fprintln(os.Stderr, "错误: 未设置 API Key，请通过 DEEPSEEK_API_KEY 环境变量或 --api-key 参数提供")
			os.Exit(1)
		}

		url := baseURL
		if url == "" {
			url = os.Getenv("DEEPSEEK_BASE_URL")
		}
		if url == "" {
			url = "https://api.deepseek.com" // 默认值
		}

		client = api.NewClient(key, url, debug)
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	// 全局标志
	rootCmd.PersistentFlags().StringVar(&apiKey, "api-key", "", "DeepSeek API Key (也可以通过 DEEPSEEK_API_KEY 环境变量设置)")
	rootCmd.PersistentFlags().StringVar(&baseURL, "base-url", "", "DeepSeek API 基础 URL (也可以通过 DEEPSEEK_BASE_URL 环境变量设置)")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "启用调试模式，打印请求和响应信息")
}