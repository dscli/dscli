package outfmt

import "strings"

// parseTableRow parses a table row like "| a | b | c |" into ["a", "b", "c"].
// Leading/trailing whitespace in each cell is trimmed.
// Returns nil if the line doesn't start and end with |.
func parseTableRow(line string) []string {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "|") || !strings.HasSuffix(trimmed, "|") {
		return nil
	}
	inner := trimmed[1 : len(trimmed)-1]
	parts := strings.Split(inner, "|")
	cells := make([]string, len(parts))
	for i, p := range parts {
		cells[i] = strings.TrimSpace(p)
	}
	return cells
}

// isTableRow checks if a line is a table row (starts and ends with |).
func isTableRow(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|")
}

// isOrgTableSeparator checks if a line is an org table separator.
// Org separators look like: |----+----+----|
// Must contain both '-' and '+' characters.
func isOrgTableSeparator(line string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "|") || !strings.HasSuffix(trimmed, "|") {
		return false
	}
	inner := trimmed[1 : len(trimmed)-1]
	hasMinus := false
	hasPlus := false
	for _, ch := range inner {
		switch ch {
		case '-':
			hasMinus = true
		case '+':
			hasPlus = true
		case '|', ' ', '\t':
			// allowed
		default:
			return false
		}
	}
	return hasMinus && hasPlus
}

// isMarkdownTableSeparator checks if a line is a markdown table separator.
// Markdown separators look like: |---|---| or |:---|:---:|---:|
// Must contain '-' characters; ':' is optional (alignment syntax).
func isMarkdownTableSeparator(line string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "|") || !strings.HasSuffix(trimmed, "|") {
		return false
	}
	inner := trimmed[1 : len(trimmed)-1]
	hasMinus := false
	for _, ch := range inner {
		switch ch {
		case '-':
			hasMinus = true
		case ':', '|', ' ', '\t':
			// allowed
		default:
			return false
		}
	}
	return hasMinus
}
