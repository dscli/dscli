package main

import (
	"fmt"
	"io"
	"strings"
)

// MarkdownToOrgConverter converts Markdown to Org mode
type MarkdownToOrgConverter struct {
	inCodeBlock     bool
	inOrgBlock      bool
	currentCodeLang string
}

// NewMarkdownToOrgConverter creates a new converter
func NewMarkdownToOrgConverter() *MarkdownToOrgConverter {
	return &MarkdownToOrgConverter{
		inCodeBlock:     false,
		inOrgBlock:      false,
		currentCodeLang: "",
	}
}

// ConvertLine converts a single line of Markdown to Org mode
func (c *MarkdownToOrgConverter) ConvertLine(line string) string {
	// 保存原始行是否有换行符
	hasNewline := strings.HasSuffix(line, "\n")
	trimmedLine := strings.TrimSpace(line)

	// Handle code blocks - 必须放在最前面
	if after, ok := strings.CutPrefix(trimmedLine, "```"); ok {
		if !c.inCodeBlock {
			// Code block start
			c.inCodeBlock = true
			lang := after
			lang = strings.TrimSpace(lang)
			if lang == "" {
				lang = "text"
			}
			return fmt.Sprintf("#+begin_src %s\n", lang)
		} else {
			// Code block end
			c.inCodeBlock = false
			return "#+end_src\n"
		}
	}

	// handle org blocks - 需要精确匹配
	if strings.HasPrefix(trimmedLine, "#+begin_src") || strings.HasPrefix(trimmedLine, "#+begin_example") {
		if !c.inOrgBlock {
			c.inOrgBlock = true
		}
		// 保持原始行的换行符
		if hasNewline && !strings.HasSuffix(line, "\n") {
			return line + "\n"
		}
		return line
	}

	if strings.HasPrefix(trimmedLine, "#+end_src") || strings.HasPrefix(trimmedLine, "#+end_example") {
		if c.inOrgBlock {
			c.inOrgBlock = false
		}
		// 保持原始行的换行符
		if hasNewline && !strings.HasSuffix(line, "\n") {
			return line + "\n"
		}
		return line
	}

	// If in code block, return as-is
	if c.inCodeBlock || c.inOrgBlock {
		// 保持原始行的换行符
		if hasNewline && !strings.HasSuffix(line, "\n") {
			return strings.ReplaceAll(line, "_", "_\u200b") + "\n"
		}
		return strings.ReplaceAll(line, "_", "_\u200b")
	}

	// Convert headers (# -> *, ## -> **, etc.)
	if len(line) > 0 && line[0] == '#' {
		level := 0
		for level < len(line) && line[level] == '#' {
			level++
		}
		if level > 0 && level <= 6 {
			text := strings.TrimSpace(line[level:])
			// 对标题文本应用格式转换
			text = c.convertMarkdownSimple(text)
			stars := strings.Repeat("*", level)
			return fmt.Sprintf("%s %s\n", stars, text)
		}
	}

	result := line

	// Use simple string processing
	result = c.convertMarkdownSimple(result)

	// 确保有换行符
	if !strings.HasSuffix(result, "\n") {
		result += "\n"
	}

	return result
}

// convertMarkdownSimple converts Markdown using simple string processing
func (c *MarkdownToOrgConverter) convertMarkdownSimple(text string) string {
	var result strings.Builder
	i := 0
	n := len(text)

	for i < n {
		// Check for bold **
		if i+1 < n && text[i] == '*' && text[i+1] == '*' {
			// Find closing **
			j := i + 2
			for j < n {
				if j+1 < n && text[j] == '*' && text[j+1] == '*' {
					// Found closing **
					boldText := text[i+2 : j]
					// 递归处理粗体中的斜体
					boldText = c.convertItalicInBold(boldText)
					result.WriteString("\u200b*")
					result.WriteString(boldText)
					result.WriteString("*\u200b")
					i = j + 2
					break
				}
				j++
			}
			if j >= n {
				// No closing ** found
				result.WriteString(text[i:])
				break
			}
			continue
		}

		// Check for italic * (but not part of **)
		if text[i] == '*' && (i == 0 || text[i-1] != '*') && (i+1 >= n || text[i+1] != '*') {
			// Find closing *
			j := i + 1
			for j < n {
				if text[j] == '*' && (j+1 >= n || text[j+1] != '*') {
					// Found closing *
					italicText := text[i+1 : j]
					result.WriteString("\u200b/")
					result.WriteString(italicText)
					result.WriteString("/\u200b")
					i = j + 1
					break
				}
				j++
			}
			if j >= n {
				// No closing * found
				result.WriteByte(text[i])
				i++
			}
			continue
		}

		// Check for strikethrough ~~
		if i+1 < n && text[i] == '~' && text[i+1] == '~' {
			j := i + 2
			for j < n {
				if j+1 < n && text[j] == '~' && text[j+1] == '~' {
					strikeText := text[i+2 : j]
					result.WriteString("\u200b+")
					result.WriteString(strikeText)
					result.WriteString("+\u200b")
					i = j + 2
					break
				}
				j++
			}
			if j >= n {
				result.WriteString(text[i:])
				break
			}
			continue
		}

		// Check for inline code `
		if text[i] == '`' {
			j := i + 1
			for j < n && text[j] != '`' {
				j++
			}
			if j < n {
				codeText := text[i+1 : j]
				result.WriteString("\u200b=")
				result.WriteString(codeText)
				result.WriteString("=\u200b")
				i = j + 1
			} else {
				result.WriteByte(text[i])
				i++
			}
			continue
		}

		// Check for links [text](url)
		if text[i] == '[' {
			// Find closing ]
			bracketEnd := -1
			for j := i + 1; j < n; j++ {
				if text[j] == ']' {
					bracketEnd = j
					break
				}
			}
			if bracketEnd != -1 && bracketEnd+1 < n && text[bracketEnd+1] == '(' {
				// Find closing )
				parenEnd := -1
				for j := bracketEnd + 2; j < n; j++ {
					if text[j] == ')' {
						parenEnd = j
						break
					}
				}
				if parenEnd != -1 {
					linkText := text[i+1 : bracketEnd]
					url := text[bracketEnd+2 : parenEnd]
					result.WriteString("[[")
					result.WriteString(url)
					result.WriteString("][")
					result.WriteString(linkText)
					result.WriteString("]]")
					i = parenEnd + 1
					continue
				}
			}
		}

		// Default: copy character
		result.WriteByte(text[i])
		i++
	}

	return result.String()
}

// convertItalicInBold converts italic text inside bold text
func (c *MarkdownToOrgConverter) convertItalicInBold(text string) string {
	var result strings.Builder
	i := 0
	n := len(text)

	for i < n {
		// Check for italic * inside bold
		if text[i] == '*' && (i == 0 || text[i-1] != '*') && (i+1 >= n || text[i+1] != '*') {
			// Find closing *
			j := i + 1
			for j < n {
				if text[j] == '*' && (j+1 >= n || text[j+1] != '*') {
					// Found closing *
					italicText := text[i+1 : j]
					result.WriteString("\u200b/")
					result.WriteString(italicText)
					result.WriteString("/\u200b")
					i = j + 1
					break
				}
				j++
			}
			if j >= n {
				// No closing * found
				result.WriteByte(text[i])
				i++
			}
			continue
		}

		// Default: copy character
		result.WriteByte(text[i])
		i++
	}

	return result.String()
}

// ConvertLines converts input to output line by line (simpler, more reliable)
func (c *MarkdownToOrgConverter) ConvertLines(input string, output io.Writer) error {
	lines := strings.Split(input, "\n")
	for _, line := range lines {
		converted := c.ConvertLine(line + "\n")
		if _, err := output.Write([]byte(converted)); err != nil {
			return fmt.Errorf("failed to write output: %w", err)
		}
	}
	return nil
}
