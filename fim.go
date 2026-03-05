package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var (
	fimModel     string
	fimSuffix    string
	fimMaxTokens int
	fimTemp      float64
)

func init() {
	fimCmd := AddRootCommand(&cobra.Command{
		Use:   "fim [prompt...]",
		Short: "FIM 代码补全",
		Long: `发送提示给 DeepSeek FIM 模型进行代码补全。
如果提供了参数，则将所有参数拼接作为 prompt；
如果没有参数，则从标准输入读取 prompt。`,
		RunE: FimRunE,
	})
	flags := fimCmd.Flags()
	flags.StringVar(&fimModel, "model", "deepseek-coder", "使用的模型名称")
	flags.StringVar(&fimSuffix, "suffix", "", "补全后缀 (可选)")
	flags.IntVar(&fimMaxTokens, "max-tokens", 1024, "最大生成 token 数")
	flags.Float64Var(&fimTemp, "temperature", 0.7, "采样温度")
}

func FimRunE(cmd *cobra.Command, args []string) (err error) {
	var prompt string
	if len(args) > 0 {
		prompt = strings.Join(args, " ")
	} else {
		var data []byte
		data, err = io.ReadAll(os.Stdin)
		if err != nil {
			return
		}
		prompt = strings.TrimSpace(string(data))
	}
	if prompt == "" {
		err = fmt.Errorf("错误: prompt 不能为空")
		return
	}

	resp, err := DeepseekClient.FIM(fimModel, prompt, fimSuffix, fimMaxTokens, fimTemp)
	log.Printf("FIM请求成功，生成 %d 个补全结果", len(resp.Choices))
	if err != nil {
		fmt.Fprintf(os.Stderr, "FIM 请求失败: %v\n", err)
		os.Exit(1)
	}
	if len(resp.Choices) == 0 {
		fmt.Fprintln(os.Stderr, "错误: 未收到回复")
		os.Exit(1)
	}
	Println(resp.Choices[0].Text)
	return
}
