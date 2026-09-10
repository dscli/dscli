package lp

import (
	"context"
	"encoding/json"
	"html"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/dscli/dscli/internal/config"
)

// f64 returns a pointer to v; the share JSON uses nullable floats.
func f64(v float64) *float64 { return &v }

// shareURL is a share page URL with a valid id; tests fetch it through the
// fake lightpanda binary.
const shareURL = "https://chat.deepseek.com/share/xgdk5g2bzpey39eko5"

// richSharePayload is a minimal conversation in the fragments schema, as
// served with the DeepSeek client headers.
const richSharePayload = `{"code":0,"msg":"","data":{"biz_code":0,"biz_msg":"","biz_data":{"title":"Shared Conversation","model_type":"default","messages":[` +
	`{"message_id":1,"role":"USER","status":"FINISHED","fragments":[{"id":1,"type":"REQUEST","content":"你好"}]},` +
	`{"message_id":2,"role":"ASSISTANT","status":"FINISHED","fragments":[{"id":2,"type":"THINK","content":"想想","elapsed_secs":1.5},{"id":3,"type":"RESPONSE","content":"回答"}]}` +
	`]}}}`

// shareEnvelope wraps a payload into the html dump envelope the fake
// binary prints: the payload is parked in a <pre> element, html-escaped,
// inside lightpanda's --json output - exactly what the real share fetch
// produces.
func shareEnvelope(t *testing.T, payload string) string {
	t.Helper()
	inner := `<html><body><pre id="ds-share-json">` + html.EscapeString(payload) + `</pre></body></html>`
	out, err := json.Marshal(map[string]any{"url": "u", "http_status": 200, "content": inner})
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestDeepSeekShareID(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
		ok   bool
	}{
		{"plain", "https://chat.deepseek.com/share/xgdk5g2bzpey39eko5", "xgdk5g2bzpey39eko5", true},
		{"trailing slash", "https://chat.deepseek.com/share/abc/", "abc", true},
		{"query and fragment", "https://chat.deepseek.com/share/abc?from=app#top", "abc", true},
		{"http scheme", "http://chat.deepseek.com/share/abc", "abc", true},
		{"hyphen and underscore", "https://chat.deepseek.com/share/a-b_C", "a-b_C", true},
		{"other host", "https://example.com/share/abc", "", false},
		{"subdomain", "https://www.chat.deepseek.com/share/abc", "", false},
		{"wrong path", "https://chat.deepseek.com/a/abc", "", false},
		{"subpath", "https://chat.deepseek.com/share/abc/more", "", false},
		{"empty id", "https://chat.deepseek.com/share/", "", false},
		{"space in id", "https://chat.deepseek.com/share/ab%20cd", "", false},
		{"not a url", "://nope", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := deepSeekShareID(tt.url)
			if got != tt.want || ok != tt.ok {
				t.Errorf("deepSeekShareID(%q) = (%q, %v), want (%q, %v)", tt.url, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestShareInjectScript(t *testing.T) {
	script := shareInjectScript("xgdk5g2bzpey39eko5")
	wants := []string{
		`"xgdk5g2bzpey39eko5"`, // the id as a JS string literal
		`'ds-share-json'`,      // the parking element id
		"encodeURIComponent",
		"x-client-bundle-id",
		"x-client-platform",
		"x-client-version",
		strconv.Itoa(shareScriptTimeoutMS),
	}
	for _, want := range wants {
		if !strings.Contains(script, want) {
			t.Errorf("shareInjectScript() missing %q", want)
		}
	}
	for _, leftover := range []string{"%SHARE_ID%", "%ELEMENT_ID%", "%TIMEOUT_MS%"} {
		if strings.Contains(script, leftover) {
			t.Errorf("shareInjectScript() left placeholder %q unreplaced", leftover)
		}
	}
}

func TestExtractParkedJSON(t *testing.T) {
	tests := []struct {
		name string
		dump string
		want string
		ok   bool
	}{
		{
			name: "simple",
			dump: `<html><body><pre id="ds-share-json">{"a":1}</pre></body></html>`,
			want: `{"a":1}`,
			ok:   true,
		},
		{
			name: "entities decoded",
			dump: `<pre id="ds-share-json">&lt;tag&gt; &amp; &quot;quoted&quot;</pre>`,
			want: `<tag> & "quoted"`,
			ok:   true,
		},
		{
			name: "attribute order does not matter",
			dump: `<pre style="display:none" id="ds-share-json">ok</pre>`,
			want: "ok",
			ok:   true,
		},
		{
			name: "missing element",
			dump: `<html><body>nothing here</body></html>`,
			ok:   false,
		},
		{
			name: "empty element",
			dump: `<pre id="ds-share-json"></pre>`,
			ok:   false,
		},
		{
			name: "whitespace only element",
			dump: `<pre id="ds-share-json">   </pre>`,
			ok:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractParkedJSON(tt.dump)
			if tt.ok && err != nil {
				t.Fatalf("extractParkedJSON() error = %v", err)
			}
			if !tt.ok && err == nil {
				t.Fatal("extractParkedJSON() error = nil, want error")
			}
			if tt.ok && got != tt.want {
				t.Errorf("extractParkedJSON() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseShareContent(t *testing.T) {
	t.Run("fragments schema", func(t *testing.T) {
		conv, err := parseShareContent([]byte(richSharePayload))
		if err != nil {
			t.Fatalf("parseShareContent() error = %v", err)
		}
		if conv.Title != "Shared Conversation" {
			t.Errorf("Title = %q", conv.Title)
		}
		if len(conv.Messages) != 2 {
			t.Fatalf("len(Messages) = %d, want 2", len(conv.Messages))
		}
		if conv.Messages[0].Role != "USER" || len(conv.Messages[1].Fragments) != 2 {
			t.Errorf("unexpected message shape: %+v", conv.Messages)
		}
	})

	t.Run("legacy schema", func(t *testing.T) {
		payload := `{"code":0,"data":{"biz_code":0,"biz_data":{"title":"T","messages":[` +
			`{"role":"ASSISTANT","status":"FINISHED","content":"回答","thinking_content":"思路"}]}}}`
		conv, err := parseShareContent([]byte(payload))
		if err != nil {
			t.Fatalf("parseShareContent() error = %v", err)
		}
		if conv.Messages[0].Content != "回答" || conv.Messages[0].ThinkingContent != "思路" {
			t.Errorf("legacy fields not decoded: %+v", conv.Messages[0])
		}
	})

	t.Run("server code error", func(t *testing.T) {
		_, err := parseShareContent([]byte(`{"code":42,"msg":"nope"}`))
		if err == nil || !strings.Contains(err.Error(), "42") {
			t.Errorf("error = %v, want server code 42", err)
		}
	})

	t.Run("business code error", func(t *testing.T) {
		_, err := parseShareContent([]byte(`{"code":0,"data":{"biz_code":7,"biz_msg":"bad share"}}`))
		if err == nil || !strings.Contains(err.Error(), "bad share") {
			t.Errorf("error = %v, want biz message", err)
		}
	})

	t.Run("no messages", func(t *testing.T) {
		_, err := parseShareContent([]byte(`{"code":0,"data":{"biz_code":0,"biz_data":{"messages":[]}}}`))
		if err == nil || !strings.Contains(err.Error(), "no messages") {
			t.Errorf("error = %v, want no-messages error", err)
		}
	})

	t.Run("garbage", func(t *testing.T) {
		_, err := parseShareContent([]byte(`not json`))
		if err == nil || !strings.Contains(err.Error(), "parse response") {
			t.Errorf("error = %v, want parse error", err)
		}
	})
}

func TestRenderShareMarkdown(t *testing.T) {
	t.Run("fragments schema", func(t *testing.T) {
		conv := &shareConversation{
			Title: "Shared Conversation",
			Messages: []shareMessage{
				{
					Role: "USER",
					Fragments: []shareFragment{
						{Type: "FILE", Files: []shareFile{
							{ID: "f1", FileName: "ICON.png", SignedPath: "/file?file_id=f1&state=abc"},
						}},
						{Type: "REQUEST", Content: "图片中是什么？"},
					},
				},
				{
					Role: "ASSISTANT",
					Fragments: []shareFragment{
						{Type: "THINK", Content: "用户想知道图片中是什么。", ElapsedSecs: f64(0.5)},
						{
							Type:    "TOOL_SEARCH",
							Content: "Found 15 web pages",
							Queries: []shareQuery{{Query: "QQ企鹅图标 介绍"}},
							Results: []shareSource{{
								URL:   "https://www.ithome.com/0/416/456.htm",
								Title: "20岁了！腾讯QQ分享企鹅图标进化史、诞生史",
							}},
						},
						{Type: "TOOL_OPEN", Result: &shareSource{
							URL:   "https://m.ithome.com/html/749221.htm",
							Title: "诞生 25 年之际，腾讯 QQ 披露企鹅 LOGO 设计细节",
						}},
						{Type: "TOOL_OPEN", Result: &shareSource{
							URL:   "https://baijiahao.baidu.com/s?id=1649327108266282862",
							Title: "QQ为什么是一只企鹅？腾讯官方公布答案！",
						}},
						{Type: "RESPONSE", Content: "图片中是**腾讯QQ的经典企鹅图标**。"},
					},
				},
			},
		}
		want := `# Shared Conversation

> Source: https://chat.deepseek.com/share/xgdk5g2bzpey39eko5
>
> This shared conversation is generated by AI, for reference only.

---

## 👤 User

📎 [ICON.png](https://chat.deepseek.com/file?file_id=f1&state=abc)

图片中是什么？

---

## 🤖 Assistant

> 💭 Thought for 0.5s
>
> 用户想知道图片中是什么。

> 🔍 Found 15 web pages
>
> - Query: QQ企鹅图标 介绍
> - [20岁了！腾讯QQ分享企鹅图标进化史、诞生史](https://www.ithome.com/0/416/456.htm)

> 📄 Read 2 pages
>
> - [诞生 25 年之际，腾讯 QQ 披露企鹅 LOGO 设计细节](https://m.ithome.com/html/749221.htm)
> - [QQ为什么是一只企鹅？腾讯官方公布答案！](https://baijiahao.baidu.com/s?id=1649327108266282862)

图片中是**腾讯QQ的经典企鹅图标**。
`
		if got := renderShareMarkdown(conv, shareURL); got != want {
			t.Errorf("renderShareMarkdown() mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
		}
	})

	t.Run("legacy schema", func(t *testing.T) {
		conv := &shareConversation{
			Title: "T",
			Messages: []shareMessage{{
				Role:            "ASSISTANT",
				Content:         "答案",
				ThinkingContent: "思路",
				ThinkingElapsed: f64(2.5),
				Files:           []shareFile{{FileName: "a.png"}},
			}},
		}
		want := `# T

> Source: u
>
> This shared conversation is generated by AI, for reference only.

---

## 🤖 Assistant

> 💭 Thought for 2.5s
>
> 思路

答案

📎 a.png
`
		if got := renderShareMarkdown(conv, "u"); got != want {
			t.Errorf("renderShareMarkdown() mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
		}
	})

	t.Run("empty messages are dropped", func(t *testing.T) {
		conv := &shareConversation{
			Title:    "T",
			Messages: []shareMessage{{Role: "USER"}, {Role: "USER", Fragments: []shareFragment{{Type: "REQUEST", Content: "唯一内容"}}}},
		}
		got := renderShareMarkdown(conv, "u")
		if strings.Contains(got, "👤 User\n\n\n") || strings.Count(got, "👤 User") != 1 {
			t.Errorf("empty message not dropped:\n%s", got)
		}
		if !strings.Contains(got, "唯一内容") {
			t.Errorf("content missing:\n%s", got)
		}
	})
}

func TestFetchShareUsesShareAPI(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	writeFakeBin(t, argsFile)
	clearProxyConfig(t)
	t.Setenv("FAKE_MODE", "share-ok")
	t.Setenv("FAKE_SHARE_OUT", shareEnvelope(t, richSharePayload))

	out, err := Fetch(context.Background(), shareURL, FetchOptions{})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	for _, want := range []string{"# Shared Conversation", "## 👤 User", "你好", "## 🤖 Assistant", "回答"} {
		if !strings.Contains(out, want) {
			t.Errorf("Fetch() output missing %q:\n%s", want, out)
		}
	}

	args := readArgs(t, argsFile)
	fetches := 0
	for _, a := range args {
		if a == "fetch" {
			fetches++
		}
	}
	if fetches != 1 {
		t.Errorf("expected 1 lightpanda invocation, got %d (args: %v)", fetches, args)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"--inject-script",
		"--wait-selector #" + shareJSONElementID,
		"--dump html",
		"x-client-bundle-id", "x-client-platform", "x-client-version",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("share invocation missing %q", want)
		}
	}
}

func TestFetchShareFallsBackToDOM(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	writeFakeBin(t, argsFile)
	clearProxyConfig(t)
	t.Setenv("FAKE_MODE", "share-broken")

	out, err := Fetch(context.Background(), shareURL, FetchOptions{})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if !strings.Contains(out, "Fake Markdown") {
		t.Errorf("Fetch() = %q, want the DOM-dump fallback", out)
	}

	args := readArgs(t, argsFile)
	fetches, injects := 0, 0
	for _, a := range args {
		switch a {
		case "fetch":
			fetches++
		case "--inject-script":
			injects++
		}
	}
	if fetches != 2 || injects != 1 {
		t.Errorf("expected share attempt + DOM fallback (2 fetches, 1 inject), got %d fetches, %d injects", fetches, injects)
	}
}

func TestFetchShareForcedProxy(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	writeFakeBin(t, argsFile)
	clearProxyConfig(t)
	config.Set("lightpanda-http-proxy", "socks5h://localhost:9999")
	t.Setenv("FAKE_MODE", "share-ok")
	t.Setenv("FAKE_SHARE_OUT", shareEnvelope(t, richSharePayload))

	if _, err := Fetch(context.Background(), shareURL, FetchOptions{ForceProxy: true}); err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	joined := strings.Join(readArgs(t, argsFile), " ")
	if !strings.Contains(joined, "--http-proxy socks5h://localhost:9999") {
		t.Errorf("share invocation missing forced proxy:\n%s", joined)
	}
	// Forced proxy means a single attempt with the full http timeout.
	if !strings.Contains(joined, "--http-timeout "+strconv.Itoa(httpTimeoutMS)) {
		t.Errorf("forced-proxy share invocation should use the full http timeout:\n%s", joined)
	}
	if strings.Contains(joined, "--http-timeout "+strconv.Itoa(probeTimeoutMS)) {
		t.Errorf("forced-proxy share invocation must not use the probe timeout:\n%s", joined)
	}
}

func TestFetchShareSkippedForNonMarkdownDump(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	writeFakeBin(t, argsFile)
	clearProxyConfig(t)

	if _, err := Fetch(context.Background(), shareURL, FetchOptions{Dump: "html"}); err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	joined := strings.Join(readArgs(t, argsFile), " ")
	if strings.Contains(joined, "--inject-script") {
		t.Errorf("html dump must not take the share path:\n%s", joined)
	}
}
