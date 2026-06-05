package main

import (
	"context"
	"fmt"
	"strconv"

	"github.com/dscli/dscli/internal/mail"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/spf13/cobra"
)

func init() {
	mailCmd := AddRootCommand(&cobra.Command{
		Use:   "mail",
		Short: "邮件系统 - 发送、阅读、搜索邮件和通讯录",
		Long:  `mail 命令用于 AI 维护者之间的显式通信。`,
	})

	// == send ==============================================================
	sendCmd := &cobra.Command{
		Use:   "send <recipient>",
		Short: "发送邮件给指定维护者",
		Long:  `向指定维护者发送邮件。接收者可以是名字（如 "Newton"）或邮箱（如 "newton@dscli.io"），不区分大小写。`,
		Args:  cobra.ExactArgs(1),
		RunE:  mailSendRunE,
	}
	sendCmd.Flags().StringP("subject", "s", "", "邮件主题")
	sendCmd.Flags().StringP("body", "b", "", "邮件正文")
	mailCmd.AddCommand(sendCmd)

	// == read ==============================================================
	readCmd := &cobra.Command{
		Use:   "read <mail-id>",
		Short: "阅读单封邮件",
		Long:  `阅读指定 ID 的邮件全文，同时标记为已读。`,
		Args:  cobra.ExactArgs(1),
		RunE:  mailReadRunE,
	}
	mailCmd.AddCommand(readCmd)

	// == list ==============================================================
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "列出邮件列表",
		Long:  `列出收件箱中的邮件（仅显示主题，不展开正文）。`,
		Args:  cobra.NoArgs,
		RunE:  mailListRunE,
	}
	listCmd.Flags().BoolP("unread", "u", false, "只显示未读邮件")
	listCmd.Flags().IntP("limit", "n", 20, "最大显示数")
	mailCmd.AddCommand(listCmd)

	// == search ============================================================
	searchCmd := &cobra.Command{
		Use:   "search <query>",
		Short: "搜索邮件",
		Long:  "使用 FTS5 全文搜索邮件主题和正文。",
		Args:  cobra.ExactArgs(1),
		RunE:  mailSearchRunE,
	}
	searchCmd.Flags().IntP("limit", "n", 10, "最大结果数")
	mailCmd.AddCommand(searchCmd)

	// == contacts ===========================================================
	contactsCmd := &cobra.Command{
		Use:   "contacts",
		Short: "列出有项目分配的联系人",
		Long:  "列出所有已分配项目的 AI 联系人及其工作项目列表。当前联系人标记为 →。",
		Args:  cobra.NoArgs,
		RunE:  contactsRunE,
	}
	mailCmd.AddCommand(contactsCmd)

	// == reply ==============================================================
	replyCmd := &cobra.Command{
		Use:   "reply <mail-id>",
		Short: "回复邮件",
		Long:  "回复指定 ID 的邮件。收件人自动设置为原邮件发件人，主题默认加 Re: 前缀。",
		Args:  cobra.ExactArgs(1),
		RunE:  mailReplyRunE,
	}
	replyCmd.Flags().StringP("subject", "s", "", "回复主题（默认 Re: <原标题>）")
	replyCmd.Flags().StringP("body", "b", "", "回复正文")
	mailCmd.AddCommand(replyCmd)

	// == delete =============================================================
	deleteCmd := &cobra.Command{
		Use:   "delete <mail-id>",
		Short: "删除邮件",
		Long:  "删除指定 ID 的邮件。只有收件人可以删除自己的邮件。",
		Args:  cobra.ExactArgs(1),
		RunE:  mailDeleteRunE,
	}
	mailCmd.AddCommand(deleteCmd)
}

func mailSendRunE(cmd *cobra.Command, args []string) error {
	recipient := args[0]
	subject, _ := cmd.Flags().GetString("subject")
	body, _ := cmd.Flags().GetString("body")

	result, _, err := mail.HandleSendMail(context.Background(), recipient, subject, body)
	if err != nil {
		return err
	}
	outfmt.Println(result)
	return nil
}

func mailReadRunE(cmd *cobra.Command, args []string) error {
	mid, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("无效的邮件 ID: %w", err)
	}

	result, _, err := mail.HandleReadMail(context.Background(), mid)
	if err != nil {
		return err
	}
	outfmt.Print(result)
	return nil
}

func mailListRunE(cmd *cobra.Command, _ []string) error {
	unreadOnly, _ := cmd.Flags().GetBool("unread")
	limit, _ := cmd.Flags().GetInt("limit")

	result, _, err := mail.HandleListMail(context.Background(), unreadOnly, limit)
	if err != nil {
		return err
	}
	outfmt.Print(result)
	return nil
}

func mailSearchRunE(cmd *cobra.Command, args []string) error {
	query := args[0]
	limit, _ := cmd.Flags().GetInt("limit")

	result, _, err := mail.HandleMailSearch(context.Background(), query, limit)
	if err != nil {
		return err
	}
	outfmt.Print(result)
	return nil
}

func contactsRunE(cmd *cobra.Command, _ []string) error {
	result, _, err := mail.HandleContacts(context.Background())
	if err != nil {
		return err
	}
	outfmt.Print(result)
	return nil
}

func mailReplyRunE(cmd *cobra.Command, args []string) error {
	mid, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("无效的邮件 ID: %w", err)
	}
	subject, _ := cmd.Flags().GetString("subject")
	body, _ := cmd.Flags().GetString("body")

	result, _, err := mail.HandleReplyMail(context.Background(), mid, subject, body)
	if err != nil {
		return err
	}
	outfmt.Println(result)
	return nil
}

func mailDeleteRunE(cmd *cobra.Command, args []string) error {
	mid, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("无效的邮件 ID: %w", err)
	}

	result, _, err := mail.HandleDeleteMail(context.Background(), mid)
	if err != nil {
		return err
	}
	outfmt.Println(result)
	return nil
}
