// Package lp provides web page reading and DeepSeek web interactions.
//
// # Web page reading
//
// Fetch runs the `lightpanda fetch` CLI as a one-shot subprocess and returns
// the page dump (markdown, html, semantic tree). This is the transport used
// by the web_fetch tool and the webget command - a fresh process per call,
// so a hung page cannot poison later calls. Proxying is handled internally:
// hosts known to need a proxy go straight through the configured proxy
// (config key lightpanda-http-proxy), other hosts are fetched directly first
// and retried via the proxy when the direct attempt fails.
//
// DeepSeek share pages (chat.deepseek.com/share/<id>) are a special case:
// their message list is virtualized, so a DOM dump only carries the first
// screenful of a conversation. For markdown dumps Fetch pulls the full
// conversation from the share-content API through an injected page script
// and renders it locally; see share.go. Any failure there falls back to the
// regular dump.
//
// # DeepSeek web interactions
//
// WebChat drives a local Chrome/Chromium via chromedp to chat with
// chat.deepseek.com (used by the chat command's web fallback), and
// deepseek_login.go automates the DeepSeek sign-in flow, sharing cookies
// so logins survive across sessions.
//
// WebChatWithOptions performs one send (one browser launch); HandleWebChat
// is the high-level entry point shared by the ask_expert tool and the
// webchat CLI command: it renders role/system prompts, retries transient
// server overload and truncation with backoff, and runs every reply through
// the `<shell>` tool loop - a `<shell>` block the expert emits is executed
// locally (see internal/shellblock) and its output fed back into the same
// conversation until a final answer arrives. Every send of a HandleWebChat
// call shares one browser session (booted lazily on the first send, closed
// when the call returns).
//
// # History
//
// The package previously talked to LightPanda over MCP (lightpanda mcp
// subprocess, stdio, or LightPanda Cloud over SSE). That transport was
// removed - MCP servers are now managed generically by internal/mcphub,
// configured via the mcp-servers config block.
//
// Config keys used:
//
//	lightpanda-http-proxy              = socks5h://localhost:8777
//	lightpanda-additional-proxy-domains = ["github.io"]

package lp
