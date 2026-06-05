package main

import (
	"fmt"
	"os"

	"github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/dsc"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/spf13/cobra"
)

func init() {
	fimCmd := AddRootCommand(&cobra.Command{
		Use:   "fim [prompt...]",
		Short: "FIM 代码补全",
		Long: `发送提示给 DeepSeek FIM 模型进行代码补全。
内容通过位置参数、标准输入提供，或通过 --input 指定文件。

示例：
   dscli fim 实现一个快速排序函数
   echo "func fib(n int) int {" | dscli fim --suffix "}"
   dscli fim --input prompt.txt
   dscli fim <<EOF
   func handleError(err error) {
   EOF
   dscli fim "实现冒泡排序" --stop '###' --stop 'END'`,

		RunE: FimRunE,
	})
	flags := fimCmd.Flags()
	flags.String("model", context.ModelDeepseekChat, "使用的模型名称")
	flags.String("suffix", "", "补全后缀 (可选)")
	flags.Int("max-tokens", 0, "最大生成 token 数（0 使用配置默认值）")
	flags.Float64("temperature", 0.7, "采样温度")
	flags.StringArray("stop", nil, "停止词，可重复使用 (如: --stop '###' --stop 'END')")
	flags.String("input", "", "从文件读取 prompt（留空则从标准输入读取）")
}

func FimRunE(cmd *cobra.Command, args []string) (err error) {
	prompt, err := ReadInput(cmd, args)
	if err != nil {
		return err
	}
	if prompt == "" {
		err = fmt.Errorf("错误: prompt 不能为空")
		return err
	}
	fimModel, err := cmd.Flags().GetString("model")
	if err != nil {
		return err
	}

	fimSuffix, err := cmd.Flags().GetString("suffix")
	if err != nil {
		return err
	}

	fimMaxTokens, err := cmd.Flags().GetInt("max-tokens")
	if err != nil {
		return err
	}
	fimTemp, err := cmd.Flags().GetFloat64("temperature")
	if err != nil {
		return err
	}

	fimStop, err := cmd.Flags().GetStringArray("stop")
	if err != nil {
		return err
	}
	var stop any
	switch len(fimStop) {
	case 0:
	case 1:
		stop = fimStop[0]
	default:
		stop = fimStop
	}

	ctx := cmd.Context()
	ctx = context.WithValue(ctx, context.CurrentModelIDKey, fimModel)
	resp, err := DeepseekClient.FIM(ctx, dsc.FIMRequest{
		Model:       fimModel,
		Prompt:      prompt,
		Suffix:      fimSuffix,
		MaxTokens:   fimMaxTokens,
		Temperature: fimTemp,
		Stop:        stop,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "FIM 请求失败: %v\n", err)
		os.Exit(1)
	}
	if len(resp.Choices) == 0 {
		fmt.Fprintln(os.Stderr, "错误: 未收到回复")
		os.Exit(1)
	}

	outfmt.Println(resp.Choices[0].Text)
	return nil
}
