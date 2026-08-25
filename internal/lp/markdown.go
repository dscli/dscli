package lp

import (
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// markdownFromDOM converts the rendered DeepSeek chat DOM back to markdown.
//
// DeepSeek's .ds-markdown elements render markdown as HTML and the DOM loses
// structure: code fences become a toolbar (language label + Copy/Download
// buttons) plus a tokenized <pre>, inline code becomes a bare <code> element
// without backticks, and lists keep their <li> markers in CSS rather than in
// text. A plain innerText extraction therefore returns text that is polluted
// with UI chrome (language name / 复制 / 下载) and stripped of every code
// delimiter — precisely what callers like ask_expert feed back into prompts.
//
// This package converts the innerHTML fragment back into markdown so the
// extracted answer matches what the user would copy from the code block's
// copy button.
//
// The walker is deliberately conservative: unknown elements degrade to their
// text content, and only well-understood DeepSeek structures are upgraded.

// joinMarkdown converts raw .ds-markdown innerHTML fragments (as returned by
// jsGetAssistantText) to markdown and concatenates them with blank lines.
// Empty parts are dropped; an empty input yields "".
func joinMarkdown(parts []string) string {
	outs := make([]string, 0, len(parts))
	for _, p := range parts {
		if m := markdownFragment(p); m != "" {
			outs = append(outs, m)
		}
	}
	return strings.TrimSpace(strings.Join(outs, "\n\n"))
}

// markdownFragment converts one .ds-markdown element's innerHTML to markdown.
func markdownFragment(frag string) string {
	ctx := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := html.ParseFragment(strings.NewReader(frag), ctx)
	if err != nil {
		// ParseFragment is lenient; this is a defensive fallback.
		return strings.TrimSpace(frag)
	}
	var b strings.Builder
	writeBlocks(&b, nodes)
	return strings.TrimSpace(b.String())
}

// blockTags are element names that start a new block when found in a block
// container. Everything else is inline and absorbed into the current paragraph.
var blockTags = map[string]bool{
	"p": true, "div": true, "section": true, "article": true,
	"pre": true, "ul": true, "ol": true, "table": true,
	"blockquote": true, "hr": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
}

// writeBlocks serializes the children of a block container: inline content is
// accumulated into paragraphs, block elements are emitted as their own blocks.
// Parts are joined with a blank line and no trailing separator is emitted, so
// nested containers (e.g. a div containing a single <pre>) don't double the
// spacing.
func writeBlocks(b *strings.Builder, nodes []*html.Node) {
	var parts []string
	var inline strings.Builder
	flush := func() {
		if t := strings.TrimSpace(inline.String()); t != "" {
			parts = append(parts, t)
		}
		inline.Reset()
	}
	for _, n := range nodes {
		if n.Type == html.TextNode {
			inline.WriteString(n.Data)
			continue
		}
		if n.Type != html.ElementNode {
			continue
		}
		if isUIChrome(n) {
			continue // code-block toolbar: language label + Copy/Download
		}
		if blockTags[n.Data] {
			flush()
			var block strings.Builder
			writeBlock(&block, n)
			if s := strings.TrimSpace(block.String()); s != "" {
				parts = append(parts, s)
			}
		} else {
			writeInline(&inline, n)
		}
	}
	flush()
	b.WriteString(strings.Join(parts, "\n\n"))
}

// writeBlock emits a single block element.
func writeBlock(b *strings.Builder, n *html.Node) {
	switch n.Data {
	case "p":
		var inline strings.Builder
		writeInline(&inline, n)
		b.WriteString(strings.TrimSpace(inline.String()))
	case "div", "section", "article":
		var inner strings.Builder
		writeBlocks(&inner, blockChildren(n))
		b.WriteString(inner.String())
	case "pre":
		writePre(b, n)
	case "ul", "ol":
		writeList(b, n, 0)
	case "table":
		writeTable(b, n)
	case "blockquote":
		var inner strings.Builder
		writeBlocks(&inner, blockChildren(n))
		b.WriteString(quoteLines(inner.String()))
	case "hr":
		b.WriteString("---")
	default:
		if len(n.Data) == 2 && n.Data[0] == 'h' && n.Data[1] >= '1' && n.Data[1] <= '6' {
			var inline strings.Builder
			writeInline(&inline, n)
			b.WriteString(strings.Repeat("#", int(n.Data[1]-'0')) + " " + strings.TrimSpace(inline.String()))
			return
		}
		var inline strings.Builder
		writeInline(&inline, n)
		b.WriteString(strings.TrimSpace(inline.String()))
	}
}

// writeInline serializes inline content: text, spans, emphasis, inline code,
// links and line breaks. Block elements encountered in an inline position
// (e.g. a <p> inside an <li>) are treated as inline containers so their text
// joins the same line; only <svg> (decorative icons) is dropped.
func writeInline(b *strings.Builder, n *html.Node) {
	if n == nil {
		return
	}
	if n.Type == html.TextNode {
		b.WriteString(n.Data)
		return
	}
	if n.Type != html.ElementNode || isUIChrome(n) {
		return
	}
	switch n.Data {
	case "code":
		writeInlineCode(b, n)
	case "strong", "b":
		b.WriteString("**")
		inlineChildren(b, n)
		b.WriteString("**")
	case "em", "i":
		b.WriteString("*")
		inlineChildren(b, n)
		b.WriteString("*")
	case "s", "del", "strike":
		b.WriteString("~~")
		inlineChildren(b, n)
		b.WriteString("~~")
	case "a":
		href := attr(n, "href")
		var t strings.Builder
		inlineChildren(&t, n)
		text := strings.TrimSpace(t.String())
		if href != "" && text != "" {
			b.WriteString("[" + text + "](" + href + ")")
		} else {
			b.WriteString(text)
		}
	case "br":
		b.WriteString("\n")
	case "img":
		if src := attr(n, "src"); src != "" {
			b.WriteString("![" + attr(n, "alt") + "](" + src + ")")
		}
	case "svg":
		// decorative fill/square icons around code blocks: drop
	default:
		inlineChildren(b, n)
	}
}

// inlineChildren serializes every child of n as inline content.
func inlineChildren(b *strings.Builder, n *html.Node) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		writeInline(b, c)
	}
}

// writeInlineCode wraps <code> content in backticks. If the code itself
// contains backticks, the delimiter escalates to double backticks
// (CommonMark rule) so the code survives round-trips.
func writeInlineCode(b *strings.Builder, n *html.Node) {
	var t strings.Builder
	inlineChildren(&t, n)
	code := strings.TrimSpace(t.String())
	code = strings.Trim(code, "\n")
	if code == "" {
		return
	}
	if strings.Contains(code, "`") {
		b.WriteString("``" + code + "``")
	} else {
		b.WriteString("`" + code + "`")
	}
}

// writePre renders a code block with its language banner. DeepSeek renders
// ```go as a banner (language label + Copy/Download buttons) followed by a
// tokenized <pre>, so the language is recovered from the banner and the code
// from the pre's full text (token spans concatenate back to the exact code).
func writePre(b *strings.Builder, pre *html.Node) {
	code := strings.TrimRight(nodeText(pre), "\n")
	b.WriteString("```" + codeLang(pre) + "\n")
	b.WriteString(code)
	b.WriteString("\n```")
}

// codeLang finds the language label of a code block: walk up to the
// .md-code-block wrapper, then read the first non-button text in its banner.
func codeLang(pre *html.Node) string {
	wrapper := findUp(pre, func(n *html.Node) bool {
		return n.Type == html.ElementNode && n.Data == "div" && hasClass(n, "md-code-block")
	})
	if wrapper == nil {
		return ""
	}
	banner := findDesc(wrapper, func(n *html.Node) bool {
		return n.Type == html.ElementNode && n.Data == "div" && hasClass(n, "md-code-block-banner")
	})
	if banner == nil {
		return ""
	}
	return firstNonButtonText(banner)
}

// firstNonButtonText returns the first text inside n that is not a
// Copy/Download button label. The language span has a hashed class (unstable),
// while button labels live in .code-info-button-text spans with stable class —
// both heuristics are used.
func firstNonButtonText(n *html.Node) string {
	if n.Type == html.ElementNode && n.Data == "span" {
		cls := classAttr(n)
		if strings.Contains(cls, "code-info-button-text") {
			return ""
		}
		if t := strings.TrimSpace(nodeText(n)); t != "" && !isCodeButtonLabel(t) {
			return t
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if s := firstNonButtonText(c); s != "" {
			return s
		}
	}
	return ""
}

func isCodeButtonLabel(s string) bool {
	switch strings.ToLower(s) {
	case "复制", "下载", "copy", "download":
		return true
	}
	return false
}

// writeList renders <ul>/<ol> with one marker per <li>, nesting deeper levels
// with two-space indentation. Item paragraphs join the same line; a nested
// list follows on its own indented lines. Lines are joined with "\n" and no
// trailing newline is emitted (block separators are the caller's concern).
func writeList(b *strings.Builder, list *html.Node, depth int) {
	ordered := list.Data == "ol"
	start := 1
	if ordered {
		if v, err := strconv.Atoi(attr(list, "start")); err == nil {
			start = v
		}
	}
	idx := start
	indent := strings.Repeat("  ", depth)
	var lines []string
	for li := list.FirstChild; li != nil; li = li.NextSibling {
		if li.Type != html.ElementNode || li.Data != "li" {
			continue
		}
		marker := "- "
		if ordered {
			marker = fmt.Sprintf("%d. ", idx)
			idx++
		}
		var item strings.Builder
		item.WriteString(indent + marker)
		writeListItem(&item, li, indent)
		lines = append(lines, item.String())
	}
	b.WriteString(strings.Join(lines, "\n"))
}

// writeListItem writes one <li>: its inline content, then any nested lists.
func writeListItem(b *strings.Builder, li *html.Node, indent string) {
	var t strings.Builder
	var nested []*html.Node
	for c := li.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && (c.Data == "ul" || c.Data == "ol") {
			nested = append(nested, c)
			continue
		}
		writeInline(&t, c)
	}
	text := strings.TrimSpace(t.String())
	// Continuation lines (from <br>) align under the item text.
	text = strings.ReplaceAll(text, "\n", "\n"+indent+"  ")
	if text != "" {
		b.WriteString(text)
	}
	for _, n := range nested {
		if text != "" {
			b.WriteString("\n")
		}
		writeList(b, n, len(indent)/2+1)
	}
}

// writeTable renders <table> as a pipe table: the first row becomes the
// header when it uses <th>, with a separator row after it.
func writeTable(b *strings.Builder, table *html.Node) {
	var rows []*html.Node
	for c := table.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode {
			continue
		}
		switch c.Data {
		case "thead", "tbody", "tfoot":
			for r := c.FirstChild; r != nil; r = r.NextSibling {
				if r.Type == html.ElementNode && r.Data == "tr" {
					rows = append(rows, r)
				}
			}
		case "tr":
			rows = append(rows, c)
		}
	}
	if len(rows) == 0 {
		return
	}
	wroteRow := false
	wroteHeader := false
	header := false
	for _, row := range rows {
		cells := tableCells(row)
		if len(cells) == 0 {
			// Degenerate row without th/td (malformed HTML). Skip it instead
			// of emitting a bare "|" line, which downstream table parsers
			// treat as a table row.
			continue
		}
		if !wroteRow {
			// The first emitted row decides whether the table has a header,
			// so a degenerate first row cannot suppress the separator.
			for _, c := range cells {
				if c.Data == "th" {
					header = true
					break
				}
			}
		}
		if wroteRow {
			b.WriteString("\n")
		}
		wroteRow = true
		line := "|"
		for _, cell := range cells {
			var t strings.Builder
			writeInline(&t, cell)
			line += " " + strings.ReplaceAll(strings.TrimSpace(t.String()), "|", `\|`) + " |"
		}
		b.WriteString(line)
		if header && !wroteHeader {
			// Separator goes right after the header row (first emitted row).
			b.WriteString("\n")
			sep := "|"
			for range cells {
				sep += " --- |"
			}
			b.WriteString(sep)
			wroteHeader = true
		}
	}
}

// tableCells returns the direct th/td children of a row.
func tableCells(row *html.Node) []*html.Node {
	var cells []*html.Node
	for c := row.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && (c.Data == "th" || c.Data == "td") {
			cells = append(cells, c)
		}
	}
	return cells
}

// quoteLines prefixes each non-empty line with "> " (blockquote style).
func quoteLines(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		if strings.TrimSpace(l) == "" {
			lines[i] = ">"
		} else {
			lines[i] = "> " + l
		}
	}
	return strings.Join(lines, "\n")
}

// isUIChrome reports whether n is part of DeepSeek's code-block toolbar
// (banner with language label and Copy/Download buttons). It is UI chrome,
// not model output.
func isUIChrome(n *html.Node) bool {
	return n.Type == html.ElementNode && strings.Contains(classAttr(n), "md-code-block-banner")
}

// blockChildren returns the child nodes of n as a slice.
func blockChildren(n *html.Node) []*html.Node {
	var out []*html.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		out = append(out, c)
	}
	return out
}

// nodeText returns the concatenated text of all descendant text nodes.
func nodeText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(x *html.Node) {
		if x.Type == html.TextNode {
			b.WriteString(x.Data)
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

// findUp walks the ancestor chain and returns the first node matching match.
func findUp(n *html.Node, match func(*html.Node) bool) *html.Node {
	for a := n.Parent; a != nil; a = a.Parent {
		if match(a) {
			return a
		}
	}
	return nil
}

// findDesc returns the first descendant (pre-order) matching match.
func findDesc(n *html.Node, match func(*html.Node) bool) *html.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if match(c) {
			return c
		}
		if r := findDesc(c, match); r != nil {
			return r
		}
	}
	return nil
}

// attr returns the value of the named attribute, or "".
func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// classAttr returns the class attribute of n.
func classAttr(n *html.Node) string { return attr(n, "class") }

// hasClass reports whether n has cls in its whitespace-separated class list.
func hasClass(n *html.Node, cls string) bool {
	for _, c := range strings.Fields(classAttr(n)) {
		if c == cls {
			return true
		}
	}
	return false
}
