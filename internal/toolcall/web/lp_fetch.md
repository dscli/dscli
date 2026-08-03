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
- `strip-mode`: comma-separated tag groups to remove, e.g. `"js,css,ui"`.
  Values: `js`, `css`, `ui` (img/video/svg), `invisible`, `full`.
- `wait-until`: event to wait for before dumping - `load`,
  `domcontentloaded`, `networkalmostidle`, `networkidle`, `done`
  (default). Use `domcontentloaded` for search engines that stream
  results, `networkidle` for fully dynamic pages.
- `wait-ms`: wait time in milliseconds (default 5000).
- `terminate-ms`: hard deadline in ms. Set for pages with endless
  scripts (e.g. live feeds) so the fetch cannot hang.
- `proxy`: HTTP proxy URL, e.g. `socks5h://localhost:8777`. **Use
  `socks5h`, not `socks5`** - socks5 resolves DNS locally, and polluted
  local DNS returns fake IPs that time out at the tunnel exit; socks5h
  resolves at the proxy. Falls back to the `lightpanda-proxy` config
  value when unset.

## Notes

- Each call is a fresh process: no long-running server, no shared state.
- Google frequently answers 429 /sorry from datacenter IPs (proxy or
  cloud alike) - Bing works well over a proxy.
- For interactive browsing (click, fill, multi-step sessions) use the
  MCP path instead (`lightpanda_goto`, `lightpanda_markdown`, etc.).
