package wechat

import (
	"fmt"
	"strings"
	"text/tabwriter"
)

// OutputFormat 输出格式
type OutputFormat string

const (
	FormatSimple   OutputFormat = "simple"   // 制表符分隔
	FormatTable    OutputFormat = "table"    // 表格（默认）
	FormatMarkdown OutputFormat = "markdown" // Markdown表格
	FormatOrg      OutputFormat = "org"      // Org mode表格
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
	if len(messages) == 0 {
		return "没有消息"
	}

	switch f.format {
	case FormatSimple:
		return f.formatSimple(messages)
	case FormatTable:
		return f.formatTable(messages)
	case FormatMarkdown:
		return f.formatMarkdown(messages)
	case FormatOrg:
		return f.formatOrg(messages)
	default:
		return f.formatTable(messages)
	}
}

// formatSimple 简洁格式（制表符分隔）
func (f *MessageFormatter) formatSimple(messages []Message) string {
	var builder strings.Builder

	for _, msg := range messages {
		builder.WriteString(fmt.Sprintf("%s\t%s\t%s\t%s\t%s\n",
			msg.ID,
			msg.CreatedAt.Format("15:04"),
			truncate(msg.From, 15),
			truncate(msg.To, 15),
			truncate(msg.Content, 50)))
	}

	return builder.String()
}

// formatTable 表格格式
func (f *MessageFormatter) formatTable(messages []Message) string {
	var builder strings.Builder
	w := tabwriter.NewWriter(&builder, 0, 0, 2, ' ', tabwriter.Debug)

	// 表头
	fmt.Fprintln(w, "ID\t时间\t发送者\t接收者\t内容\t状态")
	fmt.Fprintln(w, "──\t──\t──\t──\t──\t──")

	// 数据行
	for _, msg := range messages {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			msg.ID,
			msg.CreatedAt.Format("15:04"),
			truncate(msg.From, 15),
			truncate(msg.To, 15),
			truncate(msg.Content, 50),
			msg.Status)
	}

	w.Flush()
	return builder.String()
}

// formatMarkdown Markdown表格格式
func (f *MessageFormatter) formatMarkdown(messages []Message) string {
	var builder strings.Builder

	builder.WriteString("| ID | 时间 | 发送者 | 接收者 | 内容 | 状态 |\n")
	builder.WriteString("|----|------|--------|--------|------|------|\n")

	for _, msg := range messages {
		builder.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s |\n",
			msg.ID,
			msg.CreatedAt.Format("15:04"),
			escapeMarkdown(truncate(msg.From, 15)),
			escapeMarkdown(truncate(msg.To, 15)),
			escapeMarkdown(truncate(msg.Content, 50)),
			msg.Status))
	}

	return builder.String()
}

// formatOrg Org mode表格格式
func (f *MessageFormatter) formatOrg(messages []Message) string {
	var builder strings.Builder

	builder.WriteString("| ID | 时间 | 发送者 | 接收者 | 内容 | 状态 |\n")
	builder.WriteString("|----+------+--------+--------+------+------|\n")

	for _, msg := range messages {
		builder.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s |\n",
			msg.ID,
			msg.CreatedAt.Format("15:04"),
			truncate(msg.From, 15),
			truncate(msg.To, 15),
			truncate(msg.Content, 50),
			msg.Status))
	}

	return builder.String()
}

// FormatMessageDetail 格式化单条消息详情
func (f *MessageFormatter) FormatMessageDetail(msg *Message) string {
	var builder strings.Builder

	switch f.format {
	case FormatSimple:
		builder.WriteString(fmt.Sprintf("ID: %s\n", msg.ID))
		builder.WriteString(fmt.Sprintf("微信消息ID: %s\n", msg.WxMsgID))
		builder.WriteString(fmt.Sprintf("方向: %s\n", msg.Direction))
		builder.WriteString(fmt.Sprintf("发送者: %s\n", msg.From))
		builder.WriteString(fmt.Sprintf("接收者: %s\n", msg.To))
		builder.WriteString(fmt.Sprintf("类型: %s\n", msg.Type))
		builder.WriteString(fmt.Sprintf("状态: %s\n", msg.Status))
		builder.WriteString(fmt.Sprintf("时间: %s\n", msg.CreatedAt.Format("2006-01-02 15:04:05")))
		if !msg.RepliedAt.IsZero() {
			builder.WriteString(fmt.Sprintf("回复时间: %s\n", msg.RepliedAt.Format("2006-01-02 15:04:05")))
		}
		builder.WriteString(fmt.Sprintf("\n内容:\n%s\n", msg.Content))

	case FormatMarkdown:
		builder.WriteString("## 消息详情\n\n")
		builder.WriteString(fmt.Sprintf("**ID**: %s  \n", msg.ID))
		builder.WriteString(fmt.Sprintf("**微信消息ID**: %s  \n", msg.WxMsgID))
		builder.WriteString(fmt.Sprintf("**方向**: %s  \n", msg.Direction))
		builder.WriteString(fmt.Sprintf("**发送者**: %s  \n", msg.From))
		builder.WriteString(fmt.Sprintf("**接收者**: %s  \n", msg.To))
		builder.WriteString(fmt.Sprintf("**类型**: %s  \n", msg.Type))
		builder.WriteString(fmt.Sprintf("**状态**: %s  \n", msg.Status))
		builder.WriteString(fmt.Sprintf("**时间**: %s  \n", msg.CreatedAt.Format("2006-01-02 15:04:05")))
		if !msg.RepliedAt.IsZero() {
			builder.WriteString(fmt.Sprintf("**回复时间**: %s  \n", msg.RepliedAt.Format("2006-01-02 15:04:05")))
		}
		builder.WriteString("\n**内容**:\n\n")
		builder.WriteString(fmt.Sprintf("%s\n", msg.Content))

	case FormatOrg:
		builder.WriteString("* 消息详情\n")
		builder.WriteString(fmt.Sprintf("  - ID: %s\n", msg.ID))
		builder.WriteString(fmt.Sprintf("  - 微信消息ID: %s\n", msg.WxMsgID))
		builder.WriteString(fmt.Sprintf("  - 方向: %s\n", msg.Direction))
		builder.WriteString(fmt.Sprintf("  - 发送者: %s\n", msg.From))
		builder.WriteString(fmt.Sprintf("  - 接收者: %s\n", msg.To))
		builder.WriteString(fmt.Sprintf("  - 类型: %s\n", msg.Type))
		builder.WriteString(fmt.Sprintf("  - 状态: %s\n", msg.Status))
		builder.WriteString(fmt.Sprintf("  - 时间: %s\n", msg.CreatedAt.Format("2006-01-02 15:04:05")))
		if !msg.RepliedAt.IsZero() {
			builder.WriteString(fmt.Sprintf("  - 回复时间: %s\n", msg.RepliedAt.Format("2006-01-02 15:04:05")))
		}
		builder.WriteString(fmt.Sprintf("\n* 内容\n%s\n", msg.Content))

	default: // FormatTable
		builder.WriteString("📱 消息详情\n")
		builder.WriteString("─────────────────────────────\n")
		builder.WriteString(fmt.Sprintf("ID:           %s\n", msg.ID))
		builder.WriteString(fmt.Sprintf("微信消息ID:   %s\n", msg.WxMsgID))
		builder.WriteString(fmt.Sprintf("方向:         %s\n", msg.Direction))
		builder.WriteString(fmt.Sprintf("发送者:       %s\n", msg.From))
		builder.WriteString(fmt.Sprintf("接收者:       %s\n", msg.To))
		builder.WriteString(fmt.Sprintf("类型:         %s\n", msg.Type))
		builder.WriteString(fmt.Sprintf("状态:         %s\n", msg.Status))
		builder.WriteString(fmt.Sprintf("时间:         %s\n", msg.CreatedAt.Format("2006-01-02 15:04:05")))
		if !msg.RepliedAt.IsZero() {
			builder.WriteString(fmt.Sprintf("回复时间:     %s\n", msg.RepliedAt.Format("2006-01-02 15:04:05")))
		}
		builder.WriteString("─────────────────────────────\n")
		builder.WriteString("内容:\n")
		builder.WriteString(fmt.Sprintf("%s\n", msg.Content))
	}

	return builder.String()
}

// truncate 截断字符串
func truncate(s string, length int) string {
	if len(s) <= length {
		return s
	}
	return s[:length-3] + "..."
}

// escapeMarkdown 转义Markdown特殊字符
func escapeMarkdown(s string) string {
	// 转义Markdown特殊字符
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
