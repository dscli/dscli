package main

import (
	"runtime"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/toolcall"
	"github.com/spf13/cobra"
)

// boolToString 将布尔值转换为字符串
func boolToString(b bool) string {
	if b {
		return "启用"
	}
	return "禁用"
}

func init() {
	_ = AddRootCommand(&cobra.Command{
		Use:   "version",
		Short: "显示版本信息",
		Long:  `显示 dscli 的版本信息、构建信息和运行时信息。`,
		RunE:  VersionRunE,
	})
}

func VersionRunE(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	return versionRunE(ctx)
}

func versionRunE(ctx context.Context) (err error) {
	projectRoot := context.ContextValue(ctx, context.ProjectRootKey, "")
	outfmt.PrintHeader("dscli 版本信息")

	outfmt.PrintSection("基本信息")
	outfmt.PrintKeyValue("版本", Version)
	if Build != "" {
		outfmt.PrintKeyValue("构建信息", Build)
	}

	outfmt.PrintSection("运行时信息")
	outfmt.PrintKeyValue("Go 版本", runtime.Version())
	outfmt.PrintKeyValue("操作系统", runtime.GOOS)
	outfmt.PrintKeyValue("处理器架构", runtime.GOARCH)
	outfmt.PrintKeyValue("编译器", runtime.Compiler)

	outfmt.PrintSection("配置信息")
	outfmt.PrintKeyValue("配置目录", ConfigDir)
	outfmt.PrintKeyValue("项目根目录", projectRoot)
	outfmt.PrintKeyValue("输出模式", mode)
	outfmt.PrintKeyValue("详细输出", boolToString(verbose))
	outfmt.PrintKeyValue("颜色输出", boolToString(!colorEnabled))
	outfmt.PrintKeyValue("时间戳显示", boolToString(!showTimestamp))
	outfmt.PrintSection("模型配置")
	outfmt.PrintKeyValue("聊天模型", toolcall.ModelDeepseekChat)
	outfmt.PrintKeyValue("推理模型", toolcall.ModelDeepseekReasoner)
	return
}
