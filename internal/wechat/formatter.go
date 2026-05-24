package wechat

import (
	"fmt"
	"strings"
)

// OutputFormat 输出格式类型
type OutputFormat string

const (
	FormatSimple   OutputFormat = "simple"
	FormatTable    OutputFormat = "table"
	FormatMarkdown OutputFormat = "markdown"
	FormatOrg      OutputFormat = "org"
)

// MessageFormatter 消息格式化器
type MessageFormatter struct {
	format OutputFormat
}

// NewMessageFormatter 创建消息格式化器
func NewMessageFormatter(format OutputFormat) *MessageFormatter {
	return &MessageFormatter{format: format}
}

// FormatMessages 格式化消息列表
func (f *MessageFormatter) FormatMessages(messages []Message) string {
	switch f.format {
	case FormatSimple:
		return f.formatSimple(messages)
	case FormatMarkdown:
		return f.formatMarkdown(messages)
	case FormatOrg:
		return f.formatOrg(messages)
	default:
		return f.formatTable(messages)
	}
}

// FormatMessageDetail 格式化单条消息详情
func (f *MessageFormatter) FormatMessageDetail(msg *Message) string {
	var builder strings.Builder

	switch f.format {
	case FormatSimple:
		fmt.Fprintf(&builder, "ID: %s\n", msg.ID)
		fmt.Fprintf(&builder, "微信消息ID: %s\n", msg.WxMsgID)
		fmt.Fprintf(&builder, "方向: %s\n", msg.Direction)
		fmt.Fprintf(&builder, "发送者: %s\n", msg.From)
		fmt.Fprintf(&builder, "接收者: %s\n", msg.To)
		fmt.Fprintf(&builder, "类型: %s\n", msg.Type)
		fmt.Fprintf(&builder, "状态: %s\n", msg.Status)
		fmt.Fprintf(&builder, "时间: %s\n", msg.CreatedAt.Format("2006-01-02 15:04:05"))
		if !msg.RepliedAt.IsZero() {
			fmt.Fprintf(&builder, "回复时间: %s\n", msg.RepliedAt.Format("2006-01-02 15:04:05"))
		}

		fmt.Fprintf(&builder, "\n内容:\n%s\n", msg.Content)

	case FormatMarkdown:
		builder.WriteString("## 消息详情\n\n")
		fmt.Fprintf(&builder, "**ID**: %s  \n", msg.ID)
		fmt.Fprintf(&builder, "**微信消息ID**: %s  \n", msg.WxMsgID)
		fmt.Fprintf(&builder, "**方向**: %s  \n", msg.Direction)
		fmt.Fprintf(&builder, "**发送者**: %s  \n", msg.From)
		fmt.Fprintf(&builder, "**接收者**: %s  \n", msg.To)
		fmt.Fprintf(&builder, "**类型**: %s  \n", msg.Type)
		fmt.Fprintf(&builder, "**状态**: %s  \n", msg.Status)
		fmt.Fprintf(&builder, "**时间**: %s  \n", msg.CreatedAt.Format("2006-01-02 15:04:05"))
		if !msg.RepliedAt.IsZero() {
			fmt.Fprintf(&builder, "**回复时间**: %s  \n", msg.RepliedAt.Format("2006-01-02 15:04:05"))
		}
		builder.WriteString("\n**内容**:\n\n")
		fmt.Fprintf(&builder, "```\n%s\n```\n", msg.Content)

	case FormatOrg:
		builder.WriteString("* 消息详情\n")
		fmt.Fprintf(&builder, "  - ID: %s\n", msg.ID)
		fmt.Fprintf(&builder, "  - 微信消息ID: %s\n", msg.WxMsgID)
		fmt.Fprintf(&builder, "  - 方向: %s\n", msg.Direction)
		fmt.Fprintf(&builder, "  - 发送者: %s\n", msg.From)
		fmt.Fprintf(&builder, "  - 接收者: %s\n", msg.To)
		fmt.Fprintf(&builder, "  - 类型: %s\n", msg.Type)
		fmt.Fprintf(&builder, "  - 状态: %s\n", msg.Status)
		fmt.Fprintf(&builder, "  - 时间: %s\n", msg.CreatedAt.Format("2006-01-02 15:04:05"))
		if !msg.RepliedAt.IsZero() {
			fmt.Fprintf(&builder, "  - 回复时间: %s\n", msg.RepliedAt.Format("2006-01-02 15:04:05"))
		}
		fmt.Fprintf(&builder, "\n* 内容\n%s\n", msg.Content)

	default: // FormatTable
		builder.WriteString("📱 消息详情\n")
		builder.WriteString("=============================\n")
		fmt.Fprintf(&builder, "ID:           %s\n", msg.ID)
		fmt.Fprintf(&builder, "微信消息ID:   %s\n", msg.WxMsgID)
		fmt.Fprintf(&builder, "方向:         %s\n", msg.Direction)
		fmt.Fprintf(&builder, "发送者:       %s\n", msg.From)
		fmt.Fprintf(&builder, "接收者:       %s\n", msg.To)
		fmt.Fprintf(&builder, "类型:         %s\n", msg.Type)
		fmt.Fprintf(&builder, "状态:         %s\n", msg.Status)
		fmt.Fprintf(&builder, "时间:         %s\n", msg.CreatedAt.Format("2006-01-02 15:04:05"))
		if !msg.RepliedAt.IsZero() {
			fmt.Fprintf(&builder, "回复时间:     %s\n", msg.RepliedAt.Format("2006-01-02 15:04:05"))
		}
		builder.WriteString("=============================\n")
		builder.WriteString("内容:\n")
		fmt.Fprintf(&builder, "%s\n", msg.Content)
	}

	return builder.String()
}

// formatSimple 简洁格式（制表符分隔）
func (f *MessageFormatter) formatSimple(messages []Message) string {
	var builder strings.Builder

	for _, msg := range messages {
		fmt.Fprintf(&builder, "%s\t%s\t%s\t%s\t%s\n",
			msg.ID,
			msg.CreatedAt.Format("15:04"),
			truncateString(msg.From, 15),
			truncateString(msg.To, 15),
			truncateString(msg.Content, 50))
	}

	return builder.String()
}

// formatTable 表格格式（人类友好）
func (f *MessageFormatter) formatTable(messages []Message) string {
	var builder strings.Builder

	builder.WriteString("📱 消息列表\n")
	builder.WriteString("=====================================================================\n")
	builder.WriteString("ID    时间    发送者           接收者           内容               状态\n")
	builder.WriteString("=====================================================================\n")

	for _, msg := range messages {
		fmt.Fprintf(&builder, "%-6s %-7s %-15s %-15s %-20s %-10s\n",
			msg.ID,
			msg.CreatedAt.Format("15:04"),
			truncateString(msg.From, 15),
			truncateString(msg.To, 15),
			truncateString(msg.Content, 20),
			msg.Status)
	}

	return builder.String()
}

// formatMarkdown Markdown表格格式
func (f *MessageFormatter) formatMarkdown(messages []Message) string {
	var builder strings.Builder

	builder.WriteString("| ID | 时间 | 发送者 | 接收者 | 内容 | 状态 |\n")
	builder.WriteString("|----|------|--------|--------|------|------|\n")

	for _, msg := range messages {
		fmt.Fprintf(&builder, "| %s | %s | %s | %s | %s | %s |\n",
			msg.ID,
			msg.CreatedAt.Format("15:04"),
			escapeMarkdown(truncateString(msg.From, 15)),
			escapeMarkdown(truncateString(msg.To, 15)),
			escapeMarkdown(truncateString(msg.Content, 50)),
			msg.Status)
	}

	return builder.String()
}

// formatOrg Org mode表格格式
func (f *MessageFormatter) formatOrg(messages []Message) string {
	var builder strings.Builder

	builder.WriteString("| ID | 时间 | 发送者 | 接收者 | 内容 | 状态 |\n")
	builder.WriteString("|----+------+--------+--------+------+------|\n")

	for _, msg := range messages {
		fmt.Fprintf(&builder, "| %s | %s | %s | %s | %s | %s |\n",
			msg.ID,
			msg.CreatedAt.Format("15:04"),
			truncateString(msg.From, 15),
			truncateString(msg.To, 15),
			truncateString(msg.Content, 50),
			msg.Status)
	}

	return builder.String()
}

// truncateString 截断字符串，辅助函数
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// escapeMarkdown 转义Markdown特殊字符
func escapeMarkdown(s string) string {
	// 转义Markdown表格中的特殊字符
	replacer := strings.NewReplacer(
		"|", "\\|",
		"`", "\\`",
		"*", "\\*",
		"_", "\\_",
		"{", "\\{",
		"}", "\\}",
		"[", "\\[",
		"]", "\\]",
		"(", "\\(",
		")", "\\)",
		"#", "\\#",
		"+", "\\+",
		"-", "\\-",
		".", "\\.",
		"!", "\\!",
	)
	return replacer.Replace(s)
}
