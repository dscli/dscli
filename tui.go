package main

import (
	"fmt"
	"os"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

func init() {
	tuiCmd := AddRootCommand(&cobra.Command{
		Use:   "tui",
		Short: "启动终端用户界面（Terminal UI）",
		Long: `tui 命令启动一个交互式终端界面，支持以下功能：
  • 💰 Balance  - 查看账户余额
  • 🤖 Models   - 查看可用模型列表
  • 📜 History  - 浏览对话历史
  • 🔧 Skills   - 查看已安装技能
  • 📝 Prompt   - 查看系统提示词
  • 💬 Chat     - 与 DeepSeek 对话

在 TUI 中操作：
  j/k 或 ↑/↓  导航
  Enter      选择
  i          聚焦输入框（Chat 模式）
  q/esc      返回
  Ctrl+C     退出`,
		PreRunE: tuiPreRunE,
		RunE:    tuiRunE,
	})
	tuiCmd.Flags().String("model", context.ModelDeepseekChat, "使用的模型名称")
	tuiCmd.Flags().Int("histsize", 8, "history size loaded")
}

func tuiPreRunE(cmd *cobra.Command, args []string) (err error) {
	model, err := cmd.Flags().GetString("model")
	if err != nil {
		return
	}
	ctx := cmd.Context()

	var modelID int64
	switch model {
	case context.ModelDeepseekChat:
		ctx = context.WithValue(ctx, context.CurrentModelNameKey, context.ModelDeepseekChat)
		modelID = DeepseekChat
	case context.ModelDeepseekReasoner:
		ctx = context.WithValue(ctx, context.CurrentModelNameKey, context.ModelDeepseekReasoner)
		modelID = DeepseekReasoner
	default:
		err = fmt.Errorf("do not support %s", model)
		if verbose, _ := cmd.Flags().GetBool("verbose"); verbose {
			fmt.Printf("[DEBUG] ChatPreRunE: unsupported model error: %v\n", err)
		}
		return
	}
	ctx = context.WithValue(ctx, context.CurrentModelIDKey, modelID)
	cmd.SetContext(ctx)
	return
}

func tuiRunE(cmd *cobra.Command, args []string) error {
	if DeepseekClient == nil {
		return fmt.Errorf("DeepSeek client not initialized — check your API key")
	}

	outfmt.SetOutputMode("tui")

	ctx := cmd.Context()
	histSize, err := cmd.Flags().GetInt("histsize")
	if err != nil {
		return err
	}
	ctx = context.WithValue(ctx, context.HistSizeKey, histSize)

	model := tui.New(ctx, DeepseekClient, Version, Build, context.ProjectRoot)
	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
		return err
	}

	return nil
}