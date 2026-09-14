# task-webchat-resume: WebChat 会话恢复（resume）语义

> 状态: 已实现（本仓库直接开发）；实机验证通过（2026-09-14，真会话 ff6ac088 恢复：执行挂起 `<shell>` 块并续跑到模型下一轮）
> 日期: 2026-09-14
> 相关: `docs/task-shell-block.md`（`<shell>` 块通道）、`internal/lp/handle.go`（`HandleWebChatResume`）

## 1. 触发与行为

`dscli webchat --keep=<会话ID|会话URL|last>` 在消息为空时进入恢复模式（不发送
新内容；空消息无 `--keep`、或空消息带附件，都会被明确拒绝）：

1. 重新读取会话尾部（最新一条消息；见 §2：来自站点服务器，不是本地缓存）；
2. 尾部是未执行的 `<shell>` 块（或待执行的 DSML 调用）时，本地执行并把输出回填
   （scriptN.txt / tool_result），继续循环直到模型给出终报；
3. 无待恢复内容时，原样输出最后一条回复。

`code_dev` / `quality_assurance` 的 keep 续跑复用同一条 `HandleWebChatResume`。

## 2. 读取路径：服务器为准，本地只当游标

会话是共享状态：用户可能在自己浏览器里继续它，其他机器/进程也可能触碰它。因此
尾部读取直接走站点自己的同步接口（与会话页面同源的 `fetch`，与网页客户端每次打开
会话时调用的是同一个请求）：

| 数据 | 角色 | 恢复时如何使用 |
|------|------|----------------|
| 会话列表 `webchat_sessions.json` | Keep→URL 解析缓存 | 未命中且形如会话 ID 时直接构造站点规范 URL（`resolveWithFallback`），由站点校验存在性；不提供任何会话内容 |
| 站点 IndexedDB 记录 | **同步游标** | 只读 `version`（cache_version）与 `cacheResetAt` 两个数字，**不读消息内容**；首次加载的会话由应用在数秒内写入，读取侧轮询等待 |
| 旧记录 / HTTP 缓存 | 仅供事后诊断 | 不参与任何执行决策 |

读取步骤（`webchatReadTail`，常量与片段见 `internal/lp/webchat.go`）：

1. 打开会话页（登录态 + 页面上下文），等待输入框（登录失效快速失败）；
2. 取游标：`version` / `cacheResetAt`（IndexedDB 记录，轮询至出现；超时后退化为
   从头拉取：`cache_version=0`，不带 `cache_reset_at`）；
3. 页面内调用
   `GET /api/v0/chat/history_messages?chat_session_id=..&cache_version=N[&cache_reset_at=R]`，
   `Authorization: Bearer <userToken.value>`（取自页面 `localStorage.userToken`）；
   返回列表（旧→新）中 `message_id` 最大的一条即会话尾部；ID 非数字时退化为取
   末元素；
4. `delta(N)` 为空（"本地游标已是最新"）时改查 `N-1`，该响应自带最新一条消息——
   **尾部内容永远来自服务器**；
5. 返回尾部字段：`role`（ASSISTANT = 会话以回复结尾，可续跑；USER = 有消息在等
   回复，拒绝恢复）、`status`、`content`（模型原始 markdown，含工具块）。

## 3. 二次读取确认（防"其他端推进"）

待执行内容判定之后、执行之前，再做一次完整读取（游标 + 服务器查询）：

- 两次读取的尾部一致（trim 后内容 + role）→ 执行；
- 不一致（内容变化 / 尾部变成 USER / 第二次读取失败）→ **什么都不执行**，
  报错并让调用方按新状态重试。

由于每次读取都直接问服务器，确认步骤能看到两次读取之间"其他地方"造成的任何
变化；残余竞窗只有"二次读取完成 → 脚本真正启动"之间的数秒。

## 4. 验证

- 单元：`internal/lp/handle_test.go`（尾部守卫、二次读取门）、
  `internal/lp/webchat_test.go`（服务器读取片段 / 游标片段的回归钉）、
  `webchat_cmd_test.go`（空消息路由、空输入校验）。
- 实机（只读，不发送 / 不执行）：

```bash
DSCLI_LIVE_RESUME_PROBE='<会话ID或URL>' go test -v -timeout 300s -run TestLiveResumeProbe ./internal/lp/
```

- 实机（端到端，2026-09-14 晚）：`dscli webchat --keep=ff6ac088-4397-4f55-9c17-17cc5fb6719a`
  （空消息）→ 读取 912 字符尾部（含挂起 `<shell>` 块）→ 二次确认 → 执行
  script1.sh（exit=0，175ms）→ 回填 script1.txt → 模型续答，共 20.5s。
