# lightpanda-fetch

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
- `terminate-ms`: hard deadline in ms. Set for pages with endless
  scripts (e.g. live feeds) so the fetch cannot hang.
- `proxy`: HTTP proxy URL, e.g. `socks5h://localhost:8777`. **Use
  `socks5h`, not `socks5`** - socks5 resolves DNS locally, and polluted
  local DNS returns fake IPs that time out at the tunnel exit; socks5h
  resolves at the proxy. Falls back to the `lightpanda-proxy` value in
  `~/.dscli/dscli.env` when unset.

## Notes

- The HTTP transfer timeout is fixed at 300s (lightpanda's 10s default
  is too tight for slow sites over a proxy); the tool call itself times
  out at 330s. Use `terminate-ms` for a tighter per-page deadline.
- Each call is a fresh process: no long-running server, no shared state.
- Google frequently answers 429 /sorry from datacenter IPs (proxy or
  cloud alike) - Bing works well over a proxy.
- For interactive browsing (click, fill, multi-step sessions) use the
  MCP path instead (`lightpanda_goto`, `lightpanda_markdown`, etc.).
