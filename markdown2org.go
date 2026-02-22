package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var markdown2orgCmd = &cobra.Command{
	Use:   "markdown2org",
	Short: "Convert Markdown to Org mode format (streaming)",
	Long: `Convert Markdown from stdin to Org mode format with streaming support.
Perfect for piping with dscli chat command.

Conversion rules:
- Headers: # -> *, ## -> **, ### -> *** etc.
- Bold: **text** -> *text*
- Italic: *text* -> /text/
- Strikethrough: ~~text~~ -> +text+
- Inline code: 'code' -> =code=
- Code blocks: '''lang -> #+begin_src lang
- Links: [text](url) -> [[url][text]]
- Lists: - item -> - item (unchanged)

Examples:
  echo "# Heading\n**bold** text" | dscli markdown2org
  dscli chat < input.txt | dscli markdown2org
  cat document.md | dscli markdown2org`,
	RunE: Markdown2OrgRunE,
}

// Markdown2OrgRunE executes streaming Markdown to Org conversion
func Markdown2OrgRunE(cmd *cobra.Command, args []string) error {
	converter := NewMarkdownToOrgConverter()
	return converter.ConvertStream(os.Stdin, os.Stdout)
}

// MarkdownToOrgConverter converts Markdown to Org mode
type MarkdownToOrgConverter struct {
	inCodeBlock     bool
	inOrgCodeBlock  bool
	currentCodeLang string
}

// NewMarkdownToOrgConverter creates a new converter
func NewMarkdownToOrgConverter() *MarkdownToOrgConverter {
	return &MarkdownToOrgConverter{
		inCodeBlock:     false,
		currentCodeLang: "",
	}
}

// ConvertLine converts a single line of Markdown to Org mode
func (c *MarkdownToOrgConverter) ConvertLine(line string) string {
	trimmedLine := strings.TrimSpace(line)

	// Handle code blocks
	if strings.HasPrefix(trimmedLine, "```") {
		if !c.inCodeBlock {
			// Code block start
			c.inCodeBlock = true
			lang := strings.TrimPrefix(trimmedLine, "```")
			lang = strings.TrimSpace(lang)
			return fmt.Sprintf("#+begin_src %s\n", lang)
		} else {
			// Code block end
			c.inCodeBlock = false
			return "#+end_src\n"
		}
	}

	// handle org code blocks
	if strings.HasPrefix(trimmedLine, "#+begin_src") {
		if !c.inOrgCodeBlock {
			c.inOrgCodeBlock = true
		}
		return line // Return original line with newline
	}

	if strings.HasPrefix(trimmedLine, "#+end_src") {
		if c.inOrgCodeBlock {
			c.inOrgCodeBlock = false
		}
		return line // Return original line with newline
	}

	// If in code block, return as-is
	if c.inCodeBlock || c.inOrgCodeBlock {
		return line
	}

	// Convert headers (# -> *, ## -> **, etc.)
	if len(line) > 0 && line[0] == '#' {
		level := 0
		for level < len(line) && line[level] == '#' {
			level++
		}
		if level > 0 && level <= 6 {
			text := strings.TrimSpace(line[level:])
			stars := strings.Repeat("*", level)
			return fmt.Sprintf("%s %s\n", stars, text)
		}
	}

	result := line

	// Use simple string processing
	result = c.convertMarkdownSimple(result)

	// Ensure we keep the newline
	if strings.HasSuffix(line, "\n") && !strings.HasSuffix(result, "\n") {
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
					result.WriteString("*")
					result.WriteString(boldText)
					result.WriteString("*")
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
					result.WriteString("/")
					result.WriteString(italicText)
					result.WriteString("/")
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
					result.WriteString("+")
					result.WriteString(strikeText)
					result.WriteString("+")
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
				result.WriteString("=")
				result.WriteString(codeText)
				result.WriteString("=")
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

// ConvertStream converts input to output with streaming
func (c *MarkdownToOrgConverter) ConvertStream(input io.Reader, output io.Writer) error {
	scanner := bufio.NewScanner(input)
	writer := bufio.NewWriter(output)
	defer writer.Flush()

	for scanner.Scan() {
		line := scanner.Text()
		converted := c.ConvertLine(line + "\n") // Add newline for processing
		if _, err := writer.WriteString(converted); err != nil {
			return fmt.Errorf("failed to write output: %w", err)
		}
		// Flush immediately to maintain streaming
		writer.Flush()
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}

	return nil
}

func init() {
	rootCmd.AddCommand(markdown2orgCmd)
}
