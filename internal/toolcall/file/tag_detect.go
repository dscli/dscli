package file

import "strings"

// detectCASTags 检查内容是否包含 read_file 输出的 CAS tag 前缀。
// 返回检测到疑似 tag 的行数。
//
// 检测三种模式：
//  1. 完整冒号格式："123:[Ab12] content..."（行号 + [CAS tag]）— 几乎无假阳性
//  2. 带括号格式："[Ab12] content..."（仅 [CAS tag]）— 要求 tag 看起来像校验和
//  3. 裸 tag 格式："Ab12 content..."（无括号，4 字符 + 空格）— 要求 tag 看起来像校验和
func detectCASTags(content string) int {
	lines := strings.Split(content, "\n")
	count := 0
	for _, line := range lines {
		if len(line) < 6 {
			continue
		}

		// 模式1：数字 + 冒号 + [ + 4 位 tag 字符 + ] + 空格（"123:[Ab12] content"）
		if colonIdx := strings.IndexByte(line, ':'); colonIdx > 0 && colonIdx+7 < len(line) {
			allDigits := true
			for i := 0; i < colonIdx; i++ {
				if line[i] < '0' || line[i] > '9' {
					allDigits = false
					break
				}
			}
			if allDigits && line[colonIdx+1] == '[' &&
				line[colonIdx+6] == ']' &&
				line[colonIdx+7] == ' ' &&
				tagChar(line[colonIdx+2]) &&
				tagChar(line[colonIdx+3]) &&
				tagChar(line[colonIdx+4]) &&
				tagChar(line[colonIdx+5]) {
				count++
				continue
			}
		}

		// 模式2：行首 [ + 4 位 tag 字符 + ] + 空格（"[Ab12] content"）
		if len(line) >= 7 && line[0] == '[' && line[5] == ']' && line[6] == ' ' &&
			tagChar(line[1]) && tagChar(line[2]) &&
			tagChar(line[3]) && tagChar(line[4]) {
			// 检查前缀是否像 CAS tag（而非英文单词）
			if isTagLike(line[1:5]) {
				count++
				continue
			}
		}

		// 模式3：行首 4 位 tag 字符 + 空格 + 内容（"Ab12 content"）
		// 要求 4 字符全部来自 tagCharset，且看起来像校验和（而非英文单词）
		if len(line) >= 5 && line[4] == ' ' &&
			tagChar(line[0]) && tagChar(line[1]) &&
			tagChar(line[2]) && tagChar(line[3]) {
			if isTagLike(line[0:4]) {
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

// stripCASTags 从 content 中移除匹配已知 tag 的 CAS tag 前缀。
//
// 对于 content 的每一行，检查是否以已知的 expectedTag（带括号或不带括号）开头。
// 如果匹配，则剥离该前缀。返回剥离后的 content 和是否发生过剥离。
//
// 这是安全操作：只在已知的、已验证过的 tag 匹配时才剥离，
// 不会误伤内容文本。
//
// 支持的格式：
//   - "[tag] content" → "content"
//   - "tag content" → "content"
//   - "N:[tag] content" → "content"（N 为数字行号）
//   - "N:tag content" → "content"
func stripCASTags(content string, expectedTags []string) (string, bool) {
	if len(expectedTags) == 0 || content == "" {
		return content, false
	}

	lines := strings.Split(content, "\n")
	changed := false

	// 逐行处理，每行只与对应位置的 expectedTag 比较
	for i, line := range lines {
		if i >= len(expectedTags) {
			break // 超过预期 tag 数量的行不再处理
		}
		tag := expectedTags[i]
		if tag == "" || line == "" {
			continue
		}

		// 模式 A: "[tag] content"
		if strings.HasPrefix(line, "["+tag+"] ") {
			lines[i] = strings.TrimPrefix(line, "["+tag+"] ")
			changed = true
			continue
		}

		// 模式 B: "tag content"
		if strings.HasPrefix(line, tag+" ") {
			lines[i] = strings.TrimPrefix(line, tag+" ")
			changed = true
			continue
		}

		// 模式 C: "N:[tag] content" — N 是数字行号
		if colonIdx := strings.IndexByte(line, ':'); colonIdx > 0 && colonIdx < 6 {
			allDigits := true
			for j := 0; j < colonIdx; j++ {
				if line[j] < '0' || line[j] > '9' {
					allDigits = false
					break
				}
			}
			if allDigits {
				rest := line[colonIdx+1:]
				if strings.HasPrefix(rest, "["+tag+"] ") {
					lines[i] = strings.TrimPrefix(rest, "["+tag+"] ")
					changed = true
					continue
				}
				if strings.HasPrefix(rest, tag+" ") {
					lines[i] = strings.TrimPrefix(rest, tag+" ")
					changed = true
					continue
				}
			}
		}
	}

	if !changed {
		return content, false
	}

	return strings.Join(lines, "\n"), true
}
