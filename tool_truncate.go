package main

import (
	"strings"
	"unicode/utf8"
)

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
	Debug("工具结果需要截断: runes=%d (max=%d), bytes=%d (max=%d)",
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

// SmartTruncateWithSummary 智能截断并添加摘要
func SmartTruncateWithSummary(result string, maxRunes int) string {
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
		return truncateJSON(result, maxRunes)
	case "markdown":
		return truncateMarkdown(result, maxRunes)
	case "log":
		return truncateLog(result, maxRunes)
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

// truncateJSON 截断JSON内容
func truncateJSON(content string, maxRunes int) string {
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

// truncateMarkdown 截断Markdown内容
func truncateMarkdown(content string, maxRunes int) string {
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

// truncateLog 截断日志内容
func truncateLog(content string, maxRunes int) string {
	runes := []rune(content)
	if len(runes) <= maxRunes {
		return content
	}

	// 对于日志，我们保留开头和结尾
	// 开头保留40%，结尾保留40%，中间20%用标记替换
	startRunes := int(float64(maxRunes) * 0.4)
	endRunes := int(float64(maxRunes) * 0.4)
	middleRunes := maxRunes - startRunes - endRunes - len([]rune("\n\n[...日志已截断...]\n\n"))

	if startRunes <= 0 || endRunes <= 0 || middleRunes < 10 {
		// 如果空间不足，只保留开头
		return string(runes[:maxRunes]) + "\n[...日志已截断...]"
	}

	startPart := string(runes[:startRunes])
	endPart := string(runes[len(runes)-endRunes:])

	return startPart + "\n\n[...日志已截断...]\n\n" + endPart
}

// CountRunes 统计rune数量（UTF-8安全）
func CountRunes(s string) int {
	return utf8.RuneCountInString(s)
}

// CountBytes 统计字节数
func CountBytes(s string) int {
	return len(s)
}
