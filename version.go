package main

import (
	"runtime"

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
		Run:   VersionRun,
	})
}

func VersionRun(cmd *cobra.Command, args []string) {
	PrintHeader("dscli 版本信息")

	PrintSection("基本信息")
	PrintKeyValue("版本", Version)
	if Build != "" {
		PrintKeyValue("构建信息", Build)
	}

	PrintSection("运行时信息")
	PrintKeyValue("Go 版本", runtime.Version())
	PrintKeyValue("操作系统", runtime.GOOS)
	PrintKeyValue("处理器架构", runtime.GOARCH)
	PrintKeyValue("编译器", runtime.Compiler)

	PrintSection("配置信息")
	PrintKeyValue("配置目录", ConfigDir)
	PrintKeyValue("项目根目录", ProjectRoot)
	PrintKeyValue("输出模式", mode)
	PrintKeyValue("日志级别", logLevel)
	PrintKeyValue("颜色输出", boolToString(!colorEnabled))
	PrintKeyValue("时间戳显示", boolToString(!showTimestamp))

	PrintSection("模型配置")
	PrintKeyValue("聊天模型", ModelDeepseekChat)
	PrintKeyValue("推理模型", ModelDeepseekReasoner)
}
