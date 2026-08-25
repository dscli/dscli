package lp

import (
	"strings"
	"testing"
)

// Sample HTML fragments mirror the real DeepSeek DOM captured by the probe
// (2026-08): code blocks render as .md-code-block with a banner (language
// label + 复制/下载 buttons) and a tokenized <pre>; inline code is a bare
// <code>; list items wrap their paragraph in <p class="ds-markdown-paragraph">.

// Backtick is spliced into the raw literal below: Go raw strings cannot
// contain the delimiter character.
const backtick = "`"

const dsCodeBlockHTML = `<div class="md-code-block md-code-block-dark">
<div class="md-code-block-banner-wrap">
<div class="md-code-block-banner md-code-block-banner-lite">
<div class="_121d384">
<div class="d2a24f03"><span class="d813de27">go</span></div>
<div class="d2a24f03 _246a029">
<div role="button" class="ds-button ds-button--borderlessNeutral"><span class="code-info-button-text">复制</span></div>
<div role="button" class="ds-button ds-button--borderlessNeutral"><span class="code-info-button-text">下载</span></div>
</div>
</div>
</div>
</div>
<pre><span><span class="token keyword">package</span> main</span>
<span><span class="token keyword">import</span> <span class="token punctuation">(</span><span class="token string">"fmt"</span><span class="token punctuation">;</span><span class="token string">"regexp"</span><span class="token punctuation">)</span></span>
<span><span class="token keyword">func</span> <span class="token function">main</span><span class="token punctuation">()</span> <span class="token punctuation">{</span></span>
<span>    re <span class="token operator">:=</span> regexp<span class="token punctuation">.</span><span class="token function">MustCompile</span><span class="token punctuation">(</span><span class="token string">` + backtick + `\b[A-Z][a-z]+\b` + backtick + `</span><span class="token punctuation">)</span></span>
<span><span class="token punctuation">}</span></span></pre>
</div>`

func TestMarkdownFragmentCodeBlock(t *testing.T) {
	got := markdownFragment(dsCodeBlockHTML)
	want := "```go\n" +
		"package main\n" +
		`import ("fmt";"regexp")` + "\n" +
		"func main() {\n" +
		"    re := regexp.MustCompile(`\\b[A-Z][a-z]+\\b`)\n" +
		"}\n" +
		"```"
	if got != want {
		t.Errorf("code block mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestMarkdownFragmentPlainCodeBlock(t *testing.T) {
	// No banner (plain ``` fence): language must be empty, toolbar skipped.
	html := `<div class="md-code-block"><pre><span>echo hi</span></pre></div>`
	got := markdownFragment(html)
	want := "```\necho hi\n```"
	if got != want {
		t.Errorf("plain code block mismatch:\ngot %q\nwant %q", got, want)
	}
}

func TestMarkdownFragmentParagraphAndInlineCode(t *testing.T) {
	html := `<p class="ds-markdown-paragraph"><span class="">使用 </span><code>regexp.MustCompile</code><span class=""> 编译正则，若失败会直接 </span><code>panic</code><span class="">。</span></p>`
	got := markdownFragment(html)
	want := "使用 `regexp.MustCompile` 编译正则，若失败会直接 `panic`。"
	if got != want {
		t.Errorf("paragraph mismatch:\ngot %q\nwant %q", got, want)
	}
}

func TestMarkdownFragmentLists(t *testing.T) {
	html := `<ul><li><p class="ds-markdown-paragraph"><span class="">第一项</span></p></li>` +
		`<li><p class="ds-markdown-paragraph"><span class="">第二项，含 </span><code>code</code></p><ul><li><p class="ds-markdown-paragraph"><span class="">嵌套</span></p></li></ul></li>` +
		`<li><p class="ds-markdown-paragraph"><span class="">第三项</span></p></li></ul>`
	got := markdownFragment(html)
	want := "- 第一项\n- 第二项，含 `code`\n  - 嵌套\n- 第三项"
	if got != want {
		t.Errorf("list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestMarkdownFragmentOrderedListStart(t *testing.T) {
	html := `<ol start="3"><li><p class="ds-markdown-paragraph"><span class="">三项</span></p></li><li><p class="ds-markdown-paragraph"><span class="">四项</span></p></li></ol>`
	got := markdownFragment(html)
	want := "3. 三项\n4. 四项"
	if got != want {
		t.Errorf("ordered list mismatch:\ngot %q\nwant %q", got, want)
	}
}

func TestMarkdownFragmentHeadingQuote(t *testing.T) {
	html := `<h2>标题</h2><p class="ds-markdown-paragraph">正文</p>` +
		`<blockquote><p class="ds-markdown-paragraph">引用一段</p><p class="ds-markdown-paragraph">第二段</p></blockquote>`
	got := markdownFragment(html)
	want := "## 标题\n\n正文\n\n> 引用一段\n>\n> 第二段"
	if got != want {
		t.Errorf("heading/quote mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestMarkdownFragmentTable(t *testing.T) {
	html := `<table><thead><tr><th>名称</th><th>值</th></tr></thead><tbody><tr><td><code>x</code></td><td>1</td></tr></tbody></table>`
	got := markdownFragment(html)
	want := "| 名称 | 值 |\n| --- | --- |\n| `x` | 1 |"
	if got != want {
		t.Errorf("table mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestMarkdownFragmentEmphasisLinkBr(t *testing.T) {
	html := `<p class="ds-markdown-paragraph"><strong>粗体</strong> 与 <em>斜体</em>，去<a href="https://example.com">链接</a>，<br>换行</p>`
	got := markdownFragment(html)
	want := "**粗体** 与 *斜体*，去[链接](https://example.com)，\n换行"
	if got != want {
		t.Errorf("emphasis mismatch:\ngot %q\nwant %q", got, want)
	}
}

func TestMarkdownFragmentMixedBlocks(t *testing.T) {
	html := `<p class="ds-markdown-paragraph">先看代码：</p>` + dsCodeBlockHTML +
		`<p class="ds-markdown-paragraph">上面就这些。</p>`
	got := markdownFragment(html)
	wantPrefix := "先看代码：\n\n```go\n"
	wantSuffix := "\n```\n\n上面就这些。"
	if len(got) < len(wantPrefix)+len(wantSuffix) ||
		got[:len(wantPrefix)] != wantPrefix ||
		got[len(got)-len(wantSuffix):] != wantSuffix {
		t.Errorf("mixed blocks mismatch:\ngot:\n%s", got)
	}
	// No toolbar labels anywhere.
	for _, label := range []string{"复制", "下载", "go\n复制"} {
		if strings.Contains(got, label) {
			t.Errorf("UI chrome label %q leaked into output", label)
		}
	}
}

func TestJoinMarkdownSkipsEmpty(t *testing.T) {
	got := joinMarkdown([]string{"", `<p>a</p>`, "  "})
	want := "a"
	if got != want {
		t.Errorf("joinMarkdown mismatch: got %q want %q", got, want)
	}
}

func TestMarkdownFragmentEmptyAndGarbage(t *testing.T) {
	if got := markdownFragment(""); got != "" {
		t.Errorf("empty input: got %q", got)
	}
	if got := markdownFragment(`<p>你好</p>`); got != "你好" {
		t.Errorf("simple: got %q", got)
	}
}
