package main

import (
	"gitcode.com/dscli/dscli/internal/chimein"
	"gitcode.com/dscli/dscli/internal/outfmt"
	"github.com/spf13/cobra"
)

func init() {
	climeinCmd := AddRootCommand(&cobra.Command{
		Use:   "climein",
		Short: "向当前会话插入用户消息（插话）",
		Long: `在 LLM 与 ToolCall 交互过程中插入用户消息。
内容通过标准输入提供，或通过 --input 指定文件。

示例：
  echo "注意要用 goroutine 而不是 thread" | dscli climein
  dscli climein --input message.txt
  dscli climein <<EOF
  你前面的方案有问题，
  应该优先考虑使用接口而非具体类型。
  EOF`,
		RunE: ClimeinRunE,
	})
	climeinCmd.Flags().String("input", "", "从文件读取插话内容（留空则从标准输入读取）")
}

func ClimeinRunE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	content, err := ReadInput(cmd, args)
	if err != nil {
		return err
	}

	if content == "" {
		outfmt.Println("⚠️ 插话内容为空，未执行任何操作。")
		return nil
	}

	if err := chimein.Append(ctx, content); err != nil {
		return err
	}

	outfmt.PrintUserContent(ctx, content)
	outfmt.Println("✅ 插话内容已追加，将在下一轮 LLM 交互前注入。")
	return nil
}
