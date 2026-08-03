# Web Fetch

Fetch a web page in one shot and return its content - no MCP server, no
goto→markdown round trips. Runs `lightpanda fetch` as a one-off process
and returns stdout.

Prefer this tool for read-only page retrieval: search results, docs,
articles, any "give me this page's content" request.

## Parameters

- `url` (required): the page to fetch.
- `dump`: output format. `markdown` (default), `html`, `semantic_tree`
  (JSON DOM), or `semantic_tree_text` (pruned plain-text tree). Prefer
  `markdown` or `semantic_tree_text` - `html` can be very large.

## Proxy

- The proxy is read from config (`lightpanda-http-proxy` in
  `~/.dscli/config.dscli` or `~/.dscli/dscli.env`) - you do not pass it;
  an arbitrary proxy URL would not help, and the AI does not know the
  environment's network setup.
- Hosts that typically need a proxy (Google, YouTube, Wikipedia,
  Twitter/X, Facebook, Instagram, Telegram) go through it directly.
- Other hosts are fetched directly first; if the page fails to load or
  returns no content, the fetch is retried through the proxy.
- Use `socks5h://` (proxy-side DNS), not `socks5` - socks5 resolves DNS
  locally, and polluted local DNS returns fake IPs that time out at the
  tunnel exit.

## Notes

- Each call is a fresh process: no long-running server, no shared state.
- The HTTP transfer timeout is fixed at 300s and page JavaScript is
  capped at 60s, so endless-script pages cannot hang the tool call
  (330s backstop).
- **Google search is usually blocked**: Google fingerprint-detects the
  Lightpanda engine (it deliberately does not impersonate browsers - the
  `Sec-Ch-Ua` header always says `"Lightpanda"`). Expect HTTP 429, an
  anti-bot interstitial, or no content for google.com URLs; the tool
  reports this as an error. **Use Bing for search** - it works well over
  the same proxy.
- For interactive browsing (click, fill, multi-step sessions) use the
  MCP path instead (tools from your configured MCP servers, named
  `serverName_toolName`).
