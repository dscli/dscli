package lp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"

	"github.com/dscli/dscli/internal/config"
)

// DeepSeek share pages (chat.deepseek.com/share/<id>) are React apps whose
// message list is virtualized: only the messages inside the initial
// viewport are ever in the DOM, so an html/markdown dump of a long
// conversation comes back drastically truncated (measured: 3 of 52
// messages). Programmatic scrolling does not help - the list does not
// react to scrollTop assignments under lightpanda.
//
// The complete conversation is served by /api/v0/share/content, but the
// server returns two shapes: with DeepSeek's client identity headers
// (x-client-*) it answers with the current fragments schema - thinking,
// search sources, opened pages and attached files included; without them
// it serves a legacy variant that drops all of that and keeps only plain
// message text.
//
// lightpanda fetch cannot set request headers, so Fetch injects a script
// into the share page: it requests the API with the client headers and
// parks the JSON response in a hidden <pre id="ds-share-json"> element,
// which the html dump then carries out of the page. The parked JSON is
// parsed and rendered as markdown; any failure falls back to the regular
// DOM dump (partial content beats no content).

const (
	// shareJSONElementID is the DOM id of the element the injected script
	// parks the share JSON in; the fetch dump carries it out of the page.
	shareJSONElementID = "ds-share-json"

	// shareWaitMS bounds how long the fetch waits for the parked element.
	// The selector triggers the dump as soon as the element appears (about
	// two seconds in practice); the cap only applies when the script never
	// parks one.
	shareWaitMS = 20000

	// shareScriptTimeoutMS is the injected script's own deadline: when the
	// API request has not completed by then, the script parks an error
	// marker so the fetch does not sit out the full wait cap.
	shareScriptTimeoutMS = 15000

	// shareBaseURL serves both the share pages and the share content API;
	// file links in the rendered output resolve against it.
	shareBaseURL = "https://chat.deepseek.com"
)

// shareIDRe bounds the share id charset (ids look like
// "xgdk5g2bzpey39eko5"). Anything else does not reach the API URL.
var shareIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// deepSeekShareID reports whether rawURL is a chat.deepseek.com share page
// and returns its share id.  Only /share/<id> paths qualify; other hosts,
// paths and subpaths are left to the regular fetch path.
func deepSeekShareID(rawURL string) (string, bool) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", false
	}
	if !strings.EqualFold(u.Hostname(), "chat.deepseek.com") {
		return "", false
	}
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segs) != 2 || segs[0] != "share" || !shareIDRe.MatchString(segs[1]) {
		return "", false
	}
	return segs[1], true
}

// shareScriptTemplate is the page script injected into a share page.  It
// requests the share content API with the same client identity headers the
// DeepSeek web app sends and parks the JSON response in a hidden <pre>
// element.  The %-placeholders are filled by shareInjectScript; the
// x-client-* headers are what makes the server serve the fragments schema.
const shareScriptTemplate = `(function () {
  var done = false;
  function park(text) {
    try {
      var el = document.createElement('pre');
      el.id = '%ELEMENT_ID%';
      el.style.display = 'none';
      el.textContent = text;
      (document.body || document.documentElement).appendChild(el);
    } catch (e) {}
  }
  function parkOnce(text) {
    if (done) return;
    done = true;
    park(text);
  }
  function grab() {
    try {
      var x = new XMLHttpRequest();
      x.open('GET', '/api/v0/share/content?share_id=' + encodeURIComponent(%SHARE_ID%), true);
      x.setRequestHeader('x-client-bundle-id', 'com.deepseek.chat');
      x.setRequestHeader('x-client-platform', 'web');
      x.setRequestHeader('x-client-version', '2.4.0');
      x.setRequestHeader('x-client-locale', 'en_US');
      x.setRequestHeader('x-client-timezone-offset', '28800');
      x.setRequestHeader('accept', '*/*');
      x.onload = function () { parkOnce(x.responseText || ''); };
      x.onerror = function () { parkOnce('{"__error__":"share API request failed"}'); };
      x.send();
    } catch (e) {
      parkOnce('{"__error__":' + JSON.stringify(String(e)) + '}');
    }
  }
  setTimeout(function () { parkOnce('{"__error__":"share API request timed out"}'); }, %TIMEOUT_MS%);
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', grab);
  } else {
    grab();
  }
})()`

// shareInjectScript renders shareScriptTemplate for one share id.  The id
// is passed as a JS string literal; callers have validated its charset.
func shareInjectScript(shareID string) string {
	return strings.NewReplacer(
		"%SHARE_ID%", strconv.Quote(shareID),
		"%ELEMENT_ID%", shareJSONElementID,
		"%TIMEOUT_MS%", strconv.Itoa(shareScriptTimeoutMS),
	).Replace(shareScriptTemplate)
}

// fetchShareMarkdown retrieves a shared conversation and renders it as
// markdown.  It is the share-URL branch of fetchResolved: on any error the
// caller falls back to the regular DOM dump.  Proxy handling mirrors
// fetchWithDepth: known-blocked hosts go straight through the proxy,
// everything else is tried directly first and retried via the proxy.
func fetchShareMarkdown(ctx context.Context, rawURL, shareID string, opts FetchOptions) (string, error) {
	path, err := exec.LookPath("lightpanda")
	if err != nil {
		return "", fmt.Errorf("lightpanda not found in PATH: %w", err)
	}
	proxy := opts.Proxy
	if proxy == "" {
		proxy = config.Get("lightpanda-http-proxy", "", "lightpanda-proxy")
	}
	term := opts.TerminateMS
	if term == 0 {
		term = terminateMS
	}
	script := shareInjectScript(shareID)

	var parked string
	switch {
	case proxy != "" && (opts.ForceProxy || needsProxy(rawURL)):
		parked, err = shareFetchOnce(ctx, path, rawURL, script, proxy, httpTimeoutMS, term)
	default:
		parked, err = shareFetchOnce(ctx, path, rawURL, script, "", probeTimeoutMS, term)
		if err != nil && proxy != "" {
			parked, err = shareFetchOnce(ctx, path, rawURL, script, proxy, httpTimeoutMS, term)
		}
	}
	if err != nil {
		return "", err
	}
	conv, err := parseShareContent([]byte(parked))
	if err != nil {
		return "", err
	}
	return renderShareMarkdown(conv, rawURL), nil
}

// shareFetchOnce runs one injected lightpanda fetch and returns the parked
// share JSON.  The intermediate dump is html (not markdown) because the
// parked element only survives in a DOM dump.
func shareFetchOnce(ctx context.Context, path, rawURL, script, proxy string, timeoutMS, termMS int) (string, error) {
	args := []string{
		"fetch", rawURL,
		"--dump", "html",
		"--json",
		"--inject-script", script,
		"--wait-selector", "#" + shareJSONElementID,
		"--wait-ms", strconv.Itoa(shareWaitMS),
		"--http-timeout", strconv.Itoa(timeoutMS),
		"--terminate-ms", strconv.Itoa(termMS),
	}
	if proxy != "" {
		args = append(args, "--http-proxy", proxy)
	}
	out, err := runFetch(ctx, path, args...)
	if err != nil {
		return "", err
	}
	res, err := decodeFetchResult(out)
	if err != nil {
		return "", err
	}
	dump, err := validateFetchResult(rawURL, res, proxy)
	if err != nil {
		return "", err
	}
	parked, err := extractParkedJSON(dump)
	if err != nil {
		return "", err
	}
	// The script parks an error marker instead of JSON when its request
	// fails; a real response never carries the "__error__" key.
	var marker struct {
		Err string `json:"__error__"`
	}
	if json.Unmarshal([]byte(parked), &marker) == nil && marker.Err != "" {
		return "", fmt.Errorf("share content script: %s", marker.Err)
	}
	return parked, nil
}

// extractParkedJSON returns the text of the parked share-JSON element from
// an html dump.  The parser decodes HTML entities, so the JSON comes back
// byte-exact regardless of how the serializer escaped it.
func extractParkedJSON(dump string) (string, error) {
	doc, err := html.Parse(strings.NewReader(dump))
	if err != nil {
		return "", fmt.Errorf("parse share page dump: %w", err)
	}
	el := findElementByID(doc, shareJSONElementID)
	if el == nil {
		return "", errors.New("share page dump carries no parked JSON element")
	}
	text := nodeText(el)
	if strings.TrimSpace(text) == "" {
		return "", errors.New("share page dump carries an empty parked JSON element")
	}
	return text, nil
}

// findElementByID returns the first element whose id attribute matches.
func findElementByID(n *html.Node, id string) *html.Node {
	if n.Type == html.ElementNode && attr(n, "id") == id {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findElementByID(c, id); found != nil {
			return found
		}
	}
	return nil
}

// shareAPIResponse mirrors the /api/v0/share/content envelope.
type shareAPIResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data *struct {
		BizCode int                `json:"biz_code"`
		BizMsg  string             `json:"biz_msg"`
		BizData *shareConversation `json:"biz_data"`
	} `json:"data"`
}

// shareConversation is the conversation payload inside biz_data.
type shareConversation struct {
	Title    string         `json:"title"`
	Messages []shareMessage `json:"messages"`
}

// shareMessage is one conversation turn.  The fragments schema is current;
// the content/thinking_content/files fields belong to the legacy schema the
// server serves without the client headers, and are kept for defensiveness.
type shareMessage struct {
	Role            string          `json:"role"`
	Status          string          `json:"status"`
	Content         string          `json:"content"`
	ThinkingContent string          `json:"thinking_content"`
	ThinkingElapsed *float64        `json:"thinking_elapsed_secs"`
	Files           []shareFile     `json:"files"`
	Fragments       []shareFragment `json:"fragments"`
}

// shareFragment is one rendered piece of a message: REQUEST / THINK /
// TOOL_SEARCH / TOOL_OPEN / RESPONSE / FILE.
type shareFragment struct {
	Type        string        `json:"type"`
	Content     string        `json:"content"`
	ElapsedSecs *float64      `json:"elapsed_secs"`
	Queries     []shareQuery  `json:"queries"`
	Results     []shareSource `json:"results"`
	Result      *shareSource  `json:"result"`
	Files       []shareFile   `json:"files"`
}

// shareQuery is one search query inside a TOOL_SEARCH fragment.
type shareQuery struct {
	Query string `json:"query"`
}

// shareSource is one search result or opened page.
type shareSource struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

// shareFile is one attached file.
type shareFile struct {
	ID         string `json:"id"`
	FileName   string `json:"file_name"`
	SignedPath string `json:"signed_path"`
}

// parseShareContent decodes and validates the share content API response.
func parseShareContent(data []byte) (*shareConversation, error) {
	var resp shareAPIResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("share content: parse response: %w", err)
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("share content: server error code %d: %s", resp.Code, strings.TrimSpace(resp.Msg))
	}
	if resp.Data == nil {
		return nil, errors.New("share content: response has no data")
	}
	if resp.Data.BizCode != 0 {
		return nil, fmt.Errorf("share content: server error code %d: %s", resp.Data.BizCode, strings.TrimSpace(resp.Data.BizMsg))
	}
	if resp.Data.BizData == nil || len(resp.Data.BizData.Messages) == 0 {
		return nil, errors.New("share content: response carries no messages")
	}
	return resp.Data.BizData, nil
}

// renderShareMarkdown renders a whole conversation: a titled header with
// the source URL, then one section per message.
func renderShareMarkdown(conv *shareConversation, sourceURL string) string {
	var b strings.Builder
	title := strings.TrimSpace(conv.Title)
	if title == "" {
		title = "Shared Conversation"
	}
	b.WriteString("# " + title + "\n\n")
	b.WriteString("> Source: " + sourceURL + "\n>\n")
	b.WriteString("> This shared conversation is generated by AI, for reference only.\n\n")

	blocks := make([]string, 0, len(conv.Messages))
	for _, m := range conv.Messages {
		if s := shareMessageMarkdown(m); s != "" {
			blocks = append(blocks, s)
		}
	}
	if len(blocks) == 0 {
		// Defensive: parseShareContent rejects empty conversations, but
		// every message could still render empty.
		return b.String()
	}
	b.WriteString("---\n\n")
	b.WriteString(strings.Join(blocks, "\n\n---\n\n"))
	b.WriteString("\n")
	return b.String()
}

// shareMessageMarkdown renders one message; messages with no renderable
// content are dropped entirely.
func shareMessageMarkdown(m shareMessage) string {
	body := strings.TrimSpace(shareMessageBody(m))
	if body == "" {
		return ""
	}
	return shareRoleHeading(m.Role) + "\n\n" + body
}

// shareRoleHeading maps a role to its section heading.
func shareRoleHeading(role string) string {
	switch strings.ToUpper(role) {
	case "USER":
		return "## 👤 User"
	case "ASSISTANT":
		return "## 🤖 Assistant"
	default:
		return strings.TrimSpace("## " + role)
	}
}

// shareMessageBody renders the message body, preserving fragment order.
// Consecutive TOOL_OPEN fragments are grouped into one "Read N pages"
// block, mirroring the share page's own rendering.
func shareMessageBody(m shareMessage) string {
	if len(m.Fragments) == 0 {
		return legacyMessageBody(m)
	}
	var parts []string
	frags := m.Fragments
	for i := 0; i < len(frags); {
		if frags[i].Type == "TOOL_OPEN" {
			j := i
			for j < len(frags) && frags[j].Type == "TOOL_OPEN" {
				j++
			}
			if s := opensBlock(frags[i:j]); s != "" {
				parts = append(parts, s)
			}
			i = j
			continue
		}
		if s := shareFragmentMarkdown(frags[i]); s != "" {
			parts = append(parts, s)
		}
		i++
	}
	return strings.Join(parts, "\n\n")
}

// shareFragmentMarkdown renders a single non-grouped fragment.
func shareFragmentMarkdown(f shareFragment) string {
	switch f.Type {
	case "REQUEST", "RESPONSE":
		return strings.TrimSpace(f.Content)
	case "THINK":
		return thinkBlock(f.Content, derefFloat(f.ElapsedSecs))
	case "TOOL_SEARCH":
		return searchBlock(f)
	case "TOOL_OPEN":
		return opensBlock([]shareFragment{f})
	case "FILE":
		return filesBlock(f.Files)
	default:
		// Unknown fragment types degrade to their text, if any.
		return strings.TrimSpace(f.Content)
	}
}

// legacyMessageBody renders the pre-fragments schema (content /
// thinking_content / files), for responses that lack fragments.
func legacyMessageBody(m shareMessage) string {
	var parts []string
	if s := strings.TrimSpace(m.ThinkingContent); s != "" {
		parts = append(parts, thinkBlock(s, derefFloat(m.ThinkingElapsed)))
	}
	if s := strings.TrimSpace(m.Content); s != "" {
		parts = append(parts, s)
	}
	if s := filesBlock(m.Files); s != "" {
		parts = append(parts, s)
	}
	return strings.Join(parts, "\n\n")
}

// thinkBlock renders a thinking fragment as a labeled blockquote.
func thinkBlock(content string, secs float64) string {
	header := "💭 Thought"
	if secs > 0 {
		header += " for " + strconv.FormatFloat(secs, 'f', 1, 64) + "s"
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return "> " + header
	}
	return "> " + header + "\n>\n" + quoteLines(content)
}

// searchBlock renders a TOOL_SEARCH fragment: its status line, the queries
// and the result links.
func searchBlock(f shareFragment) string {
	header := strings.TrimSpace(f.Content)
	if header == "" {
		header = "Search"
	}
	lines := make([]string, 0, len(f.Queries)+len(f.Results))
	for _, q := range f.Queries {
		if q.Query != "" {
			lines = append(lines, "- Query: "+q.Query)
		}
	}
	for _, r := range f.Results {
		if line := sourceLine(r); line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return "> 🔍 " + header
	}
	return "> 🔍 " + header + "\n>\n" + quoteLines(strings.Join(lines, "\n"))
}

// opensBlock renders a run of TOOL_OPEN fragments as one "Read N pages"
// block.
func opensBlock(opens []shareFragment) string {
	lines := make([]string, 0, len(opens))
	for _, f := range opens {
		if f.Result == nil {
			continue
		}
		if line := sourceLine(*f.Result); line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	noun := "pages"
	if len(lines) == 1 {
		noun = "page"
	}
	header := fmt.Sprintf("> 📄 Read %d %s", len(lines), noun)
	return header + "\n>\n" + quoteLines(strings.Join(lines, "\n"))
}

// filesBlock renders attached files, linking the signed path when present.
func filesBlock(files []shareFile) string {
	lines := make([]string, 0, len(files))
	for _, f := range files {
		name := strings.TrimSpace(f.FileName)
		if name == "" {
			name = strings.TrimSpace(f.ID)
		}
		if name == "" {
			continue
		}
		if f.SignedPath != "" {
			lines = append(lines, "📎 ["+name+"]("+shareBaseURL+f.SignedPath+")")
		} else {
			lines = append(lines, "📎 "+name)
		}
	}
	return strings.Join(lines, "\n")
}

// sourceLine renders one link list item; it returns "" when the source has
// neither a URL nor a title.
func sourceLine(s shareSource) string {
	title := strings.TrimSpace(s.Title)
	if s.URL == "" && title == "" {
		return ""
	}
	if s.URL == "" {
		return "- " + title
	}
	if title == "" {
		title = s.URL
	}
	return "- [" + title + "](" + s.URL + ")"
}

// derefFloat returns the pointed-to value, or 0 when nil.
func derefFloat(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}
