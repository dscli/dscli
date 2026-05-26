package main

import (
	"context"
	"fmt"
	"strconv"

	"gitcode.com/dscli/dscli/internal/issue"
	"gitcode.com/dscli/dscli/internal/outfmt"
	"github.com/spf13/cobra"
)

func init() {
	issueCmd := AddRootCommand(&cobra.Command{
		Use:   "issue",
		Short: "Issue 管理 - 创建、列出、查看、更新、关闭、重开、分配",
		Long:  `issue 命令用于管理 GitCode 仓库的 Issues。`,
	})

	// == create ============================================================
	createCmd := &cobra.Command{
		Use:   "create",
		Short: "创建新 issue",
		Long:  `在关联的 GitCode 仓库中创建新 issue。`,
		Args:  cobra.NoArgs,
		RunE:  issueCreateRunE,
	}
	createCmd.Flags().StringP("title", "t", "", "Issue 标题（必填）")
	createCmd.Flags().StringP("body", "b", "", "Issue 正文")
	createCmd.MarkFlagRequired("title")
	issueCmd.AddCommand(createCmd)

	// == list ==============================================================
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "列出 issues",
		Long:  `列出仓库中的 issues，可按状态筛选。`,
		Args:  cobra.NoArgs,
		RunE:  issueListRunE,
	}
	listCmd.Flags().StringP("state", "s", "open", "筛选状态: open, closed, all")
	issueCmd.AddCommand(listCmd)

	// == show ==============================================================
	showCmd := &cobra.Command{
		Use:   "show <number>",
		Short: "显示 issue 详情",
		Long:  `显示指定编号 issue 的完整信息。`,
		Args:  cobra.ExactArgs(1),
		RunE:  issueShowRunE,
	}
	issueCmd.AddCommand(showCmd)

	// == update ============================================================
	updateCmd := &cobra.Command{
		Use:   "update",
		Short: "更新 issue",
		Long:  `更新 issue 的标题、正文或状态。至少提供一个更新字段。`,
		Args:  cobra.NoArgs,
		RunE:  issueUpdateRunE,
	}
	updateCmd.Flags().IntP("number", "n", 0, "Issue 编号（必填）")
	updateCmd.Flags().StringP("title", "t", "", "新标题")
	updateCmd.Flags().StringP("body", "b", "", "新正文")
	updateCmd.Flags().StringP("state", "s", "", "新状态: open, closed")
	updateCmd.MarkFlagRequired("number")
	issueCmd.AddCommand(updateCmd)

	// == close =============================================================
	closeCmd := &cobra.Command{
		Use:   "close <number>",
		Short: "关闭 issue",
		Long:  `关闭指定编号的 issue。`,
		Args:  cobra.ExactArgs(1),
		RunE:  issueCloseRunE,
	}
	issueCmd.AddCommand(closeCmd)

	// == reopen ============================================================
	reopenCmd := &cobra.Command{
		Use:   "reopen <number>",
		Short: "重新打开 issue",
		Long:  `重新打开指定编号的已关闭 issue。`,
		Args:  cobra.ExactArgs(1),
		RunE:  issueReopenRunE,
	}
	issueCmd.AddCommand(reopenCmd)

	// == assign ============================================================
	assignCmd := &cobra.Command{
		Use:   "assign",
		Short: "分配 issue 负责人",
		Long:  `将 issue 分配给指定用户。`,
		Args:  cobra.NoArgs,
		RunE:  issueAssignRunE,
	}
	assignCmd.Flags().IntP("number", "n", 0, "Issue 编号（必填）")
	assignCmd.Flags().StringP("username", "u", "", "用户名（必填）")
	assignCmd.MarkFlagRequired("number")
	assignCmd.MarkFlagRequired("username")
	issueCmd.AddCommand(assignCmd)
}

// === RunE helpers ==========================================================

func issueCreateRunE(cmd *cobra.Command, _ []string) error {
	title, _ := cmd.Flags().GetString("title")
	body, _ := cmd.Flags().GetString("body")

	iss, err := issue.CreateIssue(context.Background(), issue.CreateIssueOptions{
		Title: title,
		Body:  body,
	})
	if err != nil {
		return err
	}

	outfmt.Println("✅ Issue 创建成功!")
	outfmt.Println("")
	issue.PrintIssue(*iss, true)
	return nil
}

func issueListRunE(cmd *cobra.Command, _ []string) error {
	state, _ := cmd.Flags().GetString("state")
	if state != "open" && state != "closed" && state != "all" {
		return fmt.Errorf("状态必须是 'open'、'closed' 或 'all'，收到: %s", state)
	}

	issues, err := issue.ListIssues(context.Background(), state)
	if err != nil {
		return err
	}

	if len(issues) == 0 {
		outfmt.Printf("📋 没有找到状态为 '%s' 的 issues\n", state)
		return nil
	}

	outfmt.Printf("📋 Issues (状态: %s, 总数: %d):\n\n", state, len(issues))
	for _, iss := range issues {
		issue.PrintIssue(iss, false)
	}
	return nil
}

func issueShowRunE(_ *cobra.Command, args []string) error {
	number, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("无效的 issue 编号: %w", err)
	}

	iss, err := issue.ShowIssue(context.Background(), number)
	if err != nil {
		return err
	}

	issue.PrintIssue(*iss, true)
	return nil
}

func issueUpdateRunE(cmd *cobra.Command, _ []string) error {
	number, _ := cmd.Flags().GetInt("number")
	title, _ := cmd.Flags().GetString("title")
	body, _ := cmd.Flags().GetString("body")
	state, _ := cmd.Flags().GetString("state")

	if title == "" && body == "" && state == "" {
		return fmt.Errorf("必须提供至少一个更新字段（--title, --body 或 --state）")
	}

	// CLI 接受 open/closed，映射为 API 需要的 close/reopen
	apiState := state
	if state == "open" {
		apiState = "reopen"
	} else if state == "closed" {
		apiState = "close"
	}

	iss, err := issue.UpdateIssue(context.Background(), issue.UpdateIssueOptions{
		Number: number,
		Title:  title,
		Body:   body,
		State:  apiState,
	})
	if err != nil {
		return err
	}

	outfmt.Println("✅ Issue 更新成功!")
	outfmt.Println("")
	issue.PrintIssue(*iss, true)
	return nil
}

func issueCloseRunE(_ *cobra.Command, args []string) error {
	number, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("无效的 issue 编号: %w", err)
	}

	iss, err := issue.CloseIssue(context.Background(), number)
	if err != nil {
		return err
	}

	outfmt.Printf("✅ Issue #%s 已关闭! (状态: %s)\n", iss.Number, iss.State)
	return nil
}

func issueReopenRunE(_ *cobra.Command, args []string) error {
	number, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("无效的 issue 编号: %w", err)
	}

	iss, err := issue.ReopenIssue(context.Background(), number)
	if err != nil {
		return err
	}

	outfmt.Printf("✅ Issue #%s 已重新打开! (状态: %s)\n", iss.Number, iss.State)
	return nil
}

func issueAssignRunE(cmd *cobra.Command, _ []string) error {
	number, _ := cmd.Flags().GetInt("number")
	username, _ := cmd.Flags().GetString("username")

	iss, err := issue.AssignIssue(context.Background(), number, username)
	if err != nil {
		return err
	}

	assigneeInfo := username
	if iss.Assignee != nil && iss.Assignee.Name != "" {
		assigneeInfo = fmt.Sprintf("%s (%s)", iss.Assignee.Name, iss.Assignee.Login)
	}
	outfmt.Printf("✅ Issue #%s 已分配给用户: %s\n", iss.Number, assigneeInfo)
	return nil
}
