package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gitcode.com/nanjunjie/dscli/internal/log"
)

var (
	fimModel     string
	fimSuffix    string
	fimMaxTokens int
	fimTemp      float64
)

var fimCmd = &cobra.Command{
	Use:   "fim [prompt...]",
	Short: "FIM 代码补全",
	Long: `发送提示给 DeepSeek FIM 模型进行代码补全。
如果提供了参数，则将所有参数拼接作为 prompt；
如果没有参数，则从标准输入读取 prompt。`,
	Run: func(cmd *cobra.Command, args []string) {
		log.Info("开始FIM代码补全请求")
		log.Info("开始FIM代码补全请求")
		var prompt string
		if len(args) > 0 {
			prompt = strings.Join(args, " ")
		} else {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				fmt.Fprintf(os.Stderr, "读取标准输入失败: %v\n", err)
				os.Exit(1)
			}
			prompt = strings.TrimSpace(string(data))
		}
		if prompt == "" {
			fmt.Fprintln(os.Stderr, "错误: prompt 不能为空")
			os.Exit(1)
		}

		resp, err := client.FIM(fimModel, prompt, fimSuffix, fimMaxTokens, fimTemp)
		log.Info("FIM请求成功，生成 %d 个补全结果", len(resp.Choices))
		if err != nil {
			fmt.Fprintf(os.Stderr, "FIM 请求失败: %v\n", err)
			os.Exit(1)
		}
		if len(resp.Choices) == 0 {
			fmt.Fprintln(os.Stderr, "错误: 未收到回复")
			os.Exit(1)
		}
		fmt.Println(resp.Choices[0].Text)
	},
}

func init() {
	fimCmd.Flags().StringVar(&fimModel, "model", "deepseek-coder", "使用的模型名称")
	fimCmd.Flags().StringVar(&fimSuffix, "suffix", "", "补全后缀 (可选)")
	fimCmd.Flags().IntVar(&fimMaxTokens, "max-tokens", 1024, "最大生成 token 数")
	fimCmd.Flags().Float64Var(&fimTemp, "temperature", 0.7, "采样温度")
	rootCmd.AddCommand(fimCmd)
}
