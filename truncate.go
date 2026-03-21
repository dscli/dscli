package main

import (
	"strings"
	"unicode/utf8"

	"gitcode.com/dscli/dscli/internal/outfmt"
)

// 默认省略号（可根据需要修改为常量或配置）
const ellipsis = "..."

// ellipsisRuneLen 是省略号的字符长度（即 rune 数量），用于计算
var ellipsisRuneLen = utf8.RuneCountInString(ellipsis)

// TruncateString 截断字符串，并在末尾添加省略号
// 参数:
//
//	s: 要截断的字符串
//	maxLen: 截断后的最大字符数（包括省略号）
//
// 返回值:
//
//	截断后的字符串。如果字符串长度不超过maxLen，则返回原字符串。
//	如果maxLen小于3，返回空字符串。
//	否则返回前(maxLen-3)个字符加上"..."。
//
// 注意:
//   - 使用[]rune处理Unicode字符，避免截断时出现乱码
//   - 省略号"..."占用3个字符位置
//   - 当maxLen < 3时，无法添加省略号，返回空字符串
//
// 示例:
//
//	TruncateString("Hello World", 8)  // 返回 "Hello..."
//	TruncateString("你好世界", 4)      // 返回 "你好世界"
//	TruncateString("Test", 2)         // 返回 "" (maxLen < 3)
func TruncateString(s string, maxLen int) string {
	if maxLen < 3 {
		// 如果maxLen小于3，无法添加省略号，返回空字符串
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-3]) + ellipsis
}

// TruncateHead 截取字符串头部，超出部分用省略号代替。
// 若原字符串长度 <= maxLen，直接返回原串。
// 若 maxLen <= 省略号长度，返回省略号的前 maxLen 个字符。
// 否则返回 "原串前若干字符" + "..."
func TruncateHead(s string, maxLen int) string {
	if maxLen < 0 {
		return ""
	}
	// 将字符串转为 rune 切片，便于按字符截取
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= ellipsisRuneLen {
		// 返回省略号的前 maxLen 个字符
		return string([]rune(ellipsis)[:maxLen])
	}
	keepLen := maxLen - ellipsisRuneLen
	return string(runes[:keepLen]) + ellipsis
}

// TruncateTail 截取字符串尾部，超出部分用省略号代替。
// 若原字符串长度 <= maxLen，直接返回原串。
// 若 maxLen <= 省略号长度，返回省略号的前 maxLen 个字符。
// 否则返回 "..." + "原串后若干字符"
func TruncateTail(s string, maxLen int) string {
	if maxLen < 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= ellipsisRuneLen {
		return string([]rune(ellipsis)[:maxLen])
	}
	keepLen := maxLen - ellipsisRuneLen
	return ellipsis + string(runes[len(runes)-keepLen:])
}

// TruncateHeadTail 截取字符串头部和尾部，中间用省略号连接。
// 若原字符串长度 <= maxLen，直接返回原串。
// 若 maxLen <= 省略号长度，返回省略号的前 maxLen 个字符。
// 否则将可用字符数（maxLen - 省略号长度）平均分配给头尾（尾部可能多一个字符），返回 "头" + "..." + "尾"
func TruncateHeadTail(s string, maxLen int) string {
	if maxLen < 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= ellipsisRuneLen {
		return string([]rune(ellipsis)[:maxLen])
	}
	avail := maxLen - ellipsisRuneLen
	headLen := avail / 2
	tailLen := avail - headLen // 当 avail 为奇数时，尾部多一个字符
	head := runes[:headLen]
	tail := runes[len(runes)-tailLen:]
	return string(head) + ellipsis + string(tail)
}

// ToolResultTruncator 工具结果截断器
type ToolResultTruncator struct {
	// 最大字符数（rune数量）
	MaxRunes int
	// 最大字节数
	MaxBytes int
	// 是否保留开头和结尾
	KeepBothEnds bool
	// 截断标记
	TruncationMarker string
}

// DefaultToolResultTruncator 默认工具结果截断器
var DefaultToolResultTruncator = &ToolResultTruncator{
	MaxRunes:         8000,  // 大约对应4000 tokens
	MaxBytes:         16000, // 安全边界
	KeepBothEnds:     true,
	TruncationMarker: "\n\n[...内容已截断...]\n\n",
}

// TruncateToolResult 截断工具执行结果
func TruncateToolResult(result string) string {
	return DefaultToolResultTruncator.Truncate(result)
}

// Truncate 截断字符串
func (t *ToolResultTruncator) Truncate(result string) string {
	// 如果结果为空或已经足够小，直接返回
	if result == "" {
		return result
	}

	// 检查是否需要截断
	runes := []rune(result)
	bytesLen := len(result)

	// 如果既不超过rune限制也不超过byte限制，直接返回
	if len(runes) <= t.MaxRunes && bytesLen <= t.MaxBytes {
		return result
	}

	// 记录截断信息
	outfmt.Debug("工具结果需要截断: runes=%d (max=%d), bytes=%d (max=%d)",
		len(runes), t.MaxRunes, bytesLen, t.MaxBytes)

	// 根据配置进行截断
	if t.KeepBothEnds {
		return t.truncateKeepBothEnds(runes)
	}
	return t.truncateFromEnd(runes)
}

// truncateKeepBothEnds 保留开头和结尾的截断方式
func (t *ToolResultTruncator) truncateKeepBothEnds(runes []rune) string {
	// 计算每部分应该保留的长度
	// 我们保留开头60%，结尾40%，中间用标记分隔
	startRunes := int(float64(t.MaxRunes) * 0.6)
	endRunes := t.MaxRunes - startRunes - len([]rune(t.TruncationMarker))

	// 确保有足够的空间
	if startRunes <= 0 || endRunes <= 0 {
		// 如果空间不足，回退到简单截断
		return t.truncateFromEnd(runes)
	}

	// 获取开头部分
	startPart := string(runes[:startRunes])

	// 获取结尾部分
	endPart := string(runes[len(runes)-endRunes:])

	// 组合结果
	return startPart + t.TruncationMarker + endPart
}

// truncateFromEnd 从末尾截断
func (t *ToolResultTruncator) truncateFromEnd(runes []rune) string {
	// 简单地从开头截断到最大长度
	if len(runes) > t.MaxRunes {
		return string(runes[:t.MaxRunes]) + t.TruncationMarker
	}
	return string(runes)
}

// TruncateSummary 智能截断并添加摘要
func TruncateSummary(result string, maxRunes int) string {
	if result == "" {
		return result
	}

	runes := []rune(result)
	if len(runes) <= maxRunes {
		return result
	}

	// 分析内容类型
	contentType := detectContentType(result)

	switch contentType {
	case "json":
		return TruncateJSON(result, maxRunes)
	case "markdown":
		return TruncateMarkdown(result, maxRunes)
	case "log":
		return TruncateHeadTail(result, maxRunes)
	default:
		return DefaultToolResultTruncator.truncateKeepBothEnds(runes)
	}
}

// detectContentType 检测内容类型
func detectContentType(content string) string {
	content = strings.TrimSpace(content)

	// 检查是否是JSON
	if strings.HasPrefix(content, "{") || strings.HasPrefix(content, "[") {
		// 简单检查：尝试解析第一行
		firstLine := strings.Split(content, "\n")[0]
		if strings.Contains(firstLine, "\"") && (strings.Contains(firstLine, ":") || strings.Contains(firstLine, ",")) {
			return "json"
		}
	}

	// 检查是否是Markdown
	if strings.Contains(content, "# ") || strings.Contains(content, "```") ||
		strings.Contains(content, "**") || strings.Contains(content, "* ") {
		return "markdown"
	}

	// 检查是否是日志
	if strings.Contains(content, "ERROR") || strings.Contains(content, "WARN") ||
		strings.Contains(content, "INFO") || strings.Contains(content, "DEBUG") ||
		strings.Contains(content, "❌") || strings.Contains(content, "⚠️") ||
		strings.Contains(content, "✅") || strings.Contains(content, "📝") {
		return "log"
	}

	return "text"
}

// TruncateJSON 截断JSON内容
func TruncateJSON(content string, maxRunes int) string {
	runes := []rune(content)
	if len(runes) <= maxRunes {
		return content
	}

	// 对于JSON，我们尝试保持结构完整
	// 找到第一个完整的对象或数组
	truncated := string(runes[:maxRunes])

	// 尝试找到最后一个完整的大括号或中括号
	lastBrace := strings.LastIndex(truncated, "}")
	lastBracket := strings.LastIndex(truncated, "]")

	cutPoint := maxRunes
	if lastBrace > lastBracket && lastBrace > 0 {
		cutPoint = lastBrace + 1
	} else if lastBracket > 0 {
		cutPoint = lastBracket + 1
	}

	// 如果截断点合理，使用它
	if cutPoint < len(runes) && cutPoint > maxRunes/2 {
		return string(runes[:cutPoint]) + "\n\n[...JSON内容已截断...]\n"
	}

	return truncated + "\n\n[...JSON内容已截断...]\n"
}

// TruncateMarkdown 截断Markdown内容
func TruncateMarkdown(content string, maxRunes int) string {
	runes := []rune(content)
	if len(runes) <= maxRunes {
		return content
	}

	// 对于Markdown，我们尝试在段落边界截断
	lines := strings.Split(content, "\n")
	var resultLines []string
	currentLength := 0

	for _, line := range lines {
		lineRunes := []rune(line)
		if currentLength+len(lineRunes)+1 > maxRunes {
			// 如果添加这行会超过限制，停止
			if len(resultLines) > 0 {
				resultLines = append(resultLines, "\n[...Markdown内容已截断...]")
			}
			break
		}
		resultLines = append(resultLines, line)
		currentLength += len(lineRunes) + 1 // +1 for newline
	}

	return strings.Join(resultLines, "\n")
}
