# 任务记录：webget 读取 DeepSeek 分享链接（完整会话）

记录一次问题调查与实现决策：`dscli webget https://chat.deepseek.com/share/<id>`
只能拿到约 1/10 的会话内容。

## 根因

分享页是 React 应用，消息列表为**虚拟滚动**（DOM 中可见 `--dsl-virtual-list-*`
自定义属性）：DOM 只包含初始视口内的消息，长会话的 dump 必然严重残缺
（实测 52 条消息只渲染 3 条）。等待更久、注入滚动脚本均无效——实测
`scrollTop` 赋值后列表不重渲染（lightpanda 0.3.6）。

## 调查发现（复现与证据）

1. 页面通过 `GET /api/v0/share/content?share_id=<id>` 获取会话数据；直接抓该
   接口可解析出全部 52 条消息（`biz_data.messages`）。
2. **该接口按请求头返回两种 schema**：
   - 携带 `x-client-*`（`x-client-bundle-id: com.deepseek.chat`、
     `x-client-platform: web`、`x-client-version: 2.4.0`、`x-client-locale`、
     `x-client-timezone-offset`）时，返回**新 schema**：消息由 fragments 数组
     构成（`REQUEST` / `THINK` / `TOOL_SEARCH` / `TOOL_OPEN` / `RESPONSE` /
     `FILE`），含思考过程、搜索词与结果链接、打开过的页面、附件签名路径。
   - 否则返回**旧 schema**：纯 `content` / `thinking_content` 字符串，搜索
     来源与附件全部丢失（`search_results: null`）。
3. `lightpanda fetch` 无法自定义请求头；curl 直连 API 会被反爬拦截
   （429 Request Blocked）。→ 采用**页面上下文注入脚本**：XHR 带 `x-client-*`
   头请求 API，把响应 JSON 停靠进隐藏 `<pre id="ds-share-json">` 元素，
   由 `--dump html` 带出。
4. `--wait-selector '#ds-share-json'` 在元素出现时立即放行（实测 ~1.9s），
   `--wait-ms` 是其**上限**；脚本自带 15s 兜底定时器，超时停靠错误标记，
   避免等待悬空。

## 实现决策

- 落点 `internal/lp/share.go`，挂在 `Fetch` → `fetchResolved` 分流：
  仅当 `chat.deepseek.com/share/<id>` 且 dump 为 markdown（默认）时走分享
  链路；其他 dump 格式（html / semantic_tree）保持原样输出原始页面结构。
- **任何失败都回退**到普通 DOM 抓取（部分内容好于失败），并 `clog.Debug`
  留痕；回退对用户透明。
- 渲染为 markdown：`# 标题` + 来源块 + 每消息 `## 👤 User` / `## 🤖 Assistant`；
  THINK 渲染为 `> 💭 Thought for Ns` 引用块，TOOL_SEARCH 渲染状态行 + 查询 +
  结果链接，连续 TOOL_OPEN 归并为一个 `> 📄 Read N pages` 块（与页面一致），
  FILE 渲染为 `📎 [名称](签名路径)`；旧 schema 走 `legacyMessageBody` 兜底渲染。
- 该修复对 `dscli webget` 与 `web_fetch` 工具**同时生效**（同走 `lp.Fetch`）。

## 验证

- 单测：share_test.go（URL 识别、脚本生成、DOM 停靠提取、两种 schema 解析、
  渲染黄金串、fake lightpanda 集成：share 优先 / 失败回退 / 非 markdown
  不触发）。
- 实机：`dscli webget <本链接>` 输出 52 条消息全文（含搜索来源与附件）；
  普通页面与既有效果不变。
