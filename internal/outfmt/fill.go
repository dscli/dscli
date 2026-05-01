package outfmt

import "strings"

// DefaultFillWidth is the default maximum line width for FillParagraph.
const DefaultFillWidth = 80

// FillParagraph wraps text at word boundaries to fit within maxWidth characters.
// It preserves double-newline paragraph breaks and normalizes single newlines
// to spaces within a paragraph. This is designed for English text; Chinese
// support will be added later.
//
// If maxWidth <= 0, DefaultFillWidth is used.
func FillParagraph(text string, maxWidth int) string {
	if maxWidth <= 0 {
		maxWidth = DefaultFillWidth
	}

	// Split into paragraphs (double newline separates paragraphs)
	paragraphs := strings.Split(text, "\n\n")
	var result []string

	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		result = append(result, wrapParagraph(para, maxWidth))
	}

	return strings.Join(result, "\n\n")
}

// wrapParagraph wraps a single paragraph at word boundaries to fit maxWidth.
// It normalizes whitespace (replaces newlines with spaces, collapses multiple spaces)
// then greedily fills lines with words.
func wrapParagraph(text string, maxWidth int) string {
	// Replace single newlines with spaces (normalize within paragraph)
	text = strings.ReplaceAll(text, "\n", " ")
	// Collapse whitespace
	text = strings.Join(strings.Fields(text), " ")

	if text == "" {
		return ""
	}

	words := strings.Fields(text)
	var lines []string
	currentLine := words[0]

	for _, word := range words[1:] {
		// +1 for the space between words
		if len(currentLine)+1+len(word) <= maxWidth {
			currentLine += " " + word
		} else {
			lines = append(lines, currentLine)
			currentLine = word
		}
	}
	lines = append(lines, currentLine)

	return strings.Join(lines, "\n")
}
