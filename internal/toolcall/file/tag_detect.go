package file

import "strings"

// detectCASTags 检查内容是否包含 read_file 输出的 CAS tag 前缀。
// 返回检测到疑似 tag 的行数。
//
// 检测两种模式：
//  1. 完整冒号格式："123:Ab12 content..."（行号 + CAS tag）— 几乎无假阳性
//  2. 裸 tag 格式："Ab12 content..."（仅 CAS tag）— 要求 tag 看起来像校验和
//     而非英文单词（包含数字、下划线、或非首位的非小写字母）
func detectCASTags(content string) int {
	lines := strings.Split(content, "\n")
	count := 0
	for _, line := range lines {
		if len(line) < 5 {
			continue
		}

		// 模式1：数字 + 冒号 + 4 位 tag 字符 + 空格（"123:Ab12 content"）
		if colonIdx := strings.IndexByte(line, ':'); colonIdx > 0 && colonIdx+5 < len(line) {
			allDigits := true
			for i := 0; i < colonIdx; i++ {
				if line[i] < '0' || line[i] > '9' {
					allDigits = false
					break
				}
			}
			if allDigits && line[colonIdx+5] == ' ' &&
				tagChar(line[colonIdx+1]) &&
				tagChar(line[colonIdx+2]) &&
				tagChar(line[colonIdx+3]) &&
				tagChar(line[colonIdx+4]) {
				count++
				continue
			}
		}

		// 模式2：行首 4 位 tag 字符 + 空格（"Ab12 content"）
		if line[4] == ' ' && tagChar(line[0]) && tagChar(line[1]) &&
			tagChar(line[2]) && tagChar(line[3]) {
			// 检查前缀是否像 CAS tag（而非英文单词）
			if isTagLike(line[:4]) {
				count++
			}
		}
	}
	return count
}

// tagChar 检查字节是否为合法的 CAS tag 字符。
func tagChar(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
		(c >= '0' && c <= '9') || c == '_'
}

// isTagLike 判断 4 字符前缀是否看起来像 CAS tag（而非英文单词）。
// CAS tag 的特征是含有数字、下划线、或非首位的非小写字母。
func isTagLike(s string) bool {
	if len(s) < 4 {
		return false
	}
	// 含数字 → 像 tag（如 "4Y5Q", "eh7b"）
	for i := 0; i < 4; i++ {
		if s[i] >= '0' && s[i] <= '9' {
			return true
		}
	}
	// 含下划线 → 像 tag（如 "_1aB"）
	for i := 0; i < 4; i++ {
		if s[i] == '_' {
			return true
		}
	}
	// 非首位有非小写字母 → 像 tag（如 "Q8fA" 的 '8', "DATA" 的 'A'）
	for i := 1; i < 4; i++ {
		if s[i] < 'a' || s[i] > 'z' {
			return true
		}
	}
	return false
}

// casTagThreshold 是触发 CAS tag 污染拒绝所需的最小匹配行数。
const casTagThreshold = 3
