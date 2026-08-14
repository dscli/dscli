# Web Fetch

Fetch a web page in one shot and return its content - no MCP server, no
goto→markdown round trips. Runs `lightpanda fetch` as a one-off process.
Prefer it for read-only page retrieval: search results, docs, articles.

## Parameters

- `url` (required): the page to fetch.
- `dump`: output format. `markdown` (default), `html`, `semantic_tree`
  (JSON DOM), or `semantic_tree_text` (pruned plain-text tree). Prefer
  `markdown` or `semantic_tree_text` - `html` can be very large.
- `output`: save the result to a file too. `path` overwrites; `path:N`
  inserts the content at line N (1-based, original line N shifts down);
  N beyond the last line appends. If the file is missing it is created
  and N is ignored; content is still returned. Path is relative to the
  dscli working directory.

## Proxy

- Read from config (`lightpanda-http-proxy` in `~/.dscli/config.dscli`
  or `~/.dscli/dscli.env`) - you never pass it; the AI does not know
  the network setup.
- Google, YouTube, Wikipedia, Twitter/X, Facebook, Instagram, Telegram
  use the proxy directly. Other hosts: direct first, retry via proxy on
  failure or empty content.
- Use `socks5h://` (proxy-side DNS): socks5 resolves DNS locally, and
  polluted local DNS returns fake IPs that time out at the tunnel exit.

## Notes

- Each call is a fresh process: no shared state.
- HTTP timeout 300s, page JS capped at 60s (330s backstop) - endless
  scripts cannot hang the call.
- **Google search is usually blocked** (Lightpanda is fingerprint-
  detected; `Sec-Ch-Ua` always says `"Lightpanda"` - it does not
  impersonate browsers): expect 429, an interstitial, or no content;
  the tool reports this as an error. **Use Bing instead** - it works
  well over the same proxy.
- For interactive browsing (click, fill, multi-step) use the MCP path
  (tools from your configured MCP servers, named `serverName_toolName`).
