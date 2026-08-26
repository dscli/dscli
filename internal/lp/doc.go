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
// # DeepSeek web interactions
//
// WebChat drives a local Chrome/Chromium via chromedp to chat with
// chat.deepseek.com (used by the chat command's web fallback), and
// deepseek_login.go automates the DeepSeek sign-in flow, sharing cookies
// so logins survive across sessions.
//
// WebChatWithOptions performs one send; HandleWebChat is the high-level
// entry point shared by the ask_expert tool and the webchat CLI command:
// it renders role/system prompts, retries transient server overload and
// truncation with backoff, and - for role-driven consultations - executes
// DSML tool calls the expert embeds in its reply, feeding results back into
// the same conversation until a final answer arrives.
//
// # Layering exception
//
// HandleWebChat imports internal/toolcall (DSML parsing/execution) and
// internal/prompt (role rendering), so it transitively pulls in the tool
// framework. This inverts the usual "transport is below tools" layering, but
// is accepted deliberately: DSML tool calls are a WebChat-specific wire
// format produced only by role-driven web consultations, and the executor
// needs the tool registry (toolcall.ExecuteDSMLToolCalls). There is no
// import cycle because toolcall never imports lp. If a browser-only binary
// ever split off, HandleWebChat would move with the DSML layer.
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
