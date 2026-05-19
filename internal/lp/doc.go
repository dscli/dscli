// Package lp provides web page reading via lightpanda browser with CDP.
//
// # Architecture
//
//	url(req) +------------+   wss    +---------------+ http  +-------------+
//	-------->|  Web       +--------->|               +------>|             |
//	         |  chromedp  |          |  Lightpanda   |       |  WebServer  |
//	<--------+            |<---------+               |<------+             |
//	markdown +------------+ markdown +---------------+  rep  +-------------+
//
// The package replaces the previous approach of using Go's net/http client
// directly. Lightpanda provides a headless browser with CDP support,
// solving these problems:
//  1. JavaScript-rendered pages that return empty content via HTTP
//  2. Geo-restricted sites (e.g. google.com) inaccessible from local network
//  3. Better HTML-to-markdown conversion via LP.getMarkdown CDP command
//
// # Remote vs Local
//
// Lightpanda runs in two modes:
//   - Local:  ws://127.2.2.9:9227 (no auth, user runs lightpanda serve)
//   - Remote: wss://euwest.cloud.lightpanda.io/ws (token auth, 8h/month limit)
//
// Routing decision: if the target host is in the remoteHosts list
// (geo-restricted sites), use remote; otherwise use local.
//
// # Config keys (~/.dscli/config.dscli)
//
//	lightpanda-local-url   = ws://127.2.2.9:9227
//	lightpanda-remote-url  = wss://euwest.cloud.lightpanda.io/ws
//	lightpanda-remote-token = <token>
//
// # Usage
//
//	import "gitcode.com/dscli/dscli/internal/lp"
//
//	markdown, err := lp.Get(ctx, "https://example.com")
//
// # Future work
//   - Web writer support (e.g. interacting with chat.deepseek.com)
package lp
