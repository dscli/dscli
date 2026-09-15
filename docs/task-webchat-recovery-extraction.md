# task-webchat-recovery-extraction: 恢复轮的提取必须只取当前轮回复（2/2 版本）

> 状态: 已实现（2026-09-15）；`go test ./...`、`make fmt-check` 全绿；探针实测与提交见 §10。
> 依据: 用户 bug 报告（截图 `屏幕截图_20260915_155040.png`、`屏幕截图_20260915_155147.png`、`屏幕截图_20260915_155417.png`，2026-09-15）+ 会话转写取证（`~/.dscli/outputs/nanjunjie/2026-09-15-15-20-36.org`，会话 `https://chat.deepseek.com/a/chat/s/ff6ac088-4397-4f55-9c17-17cc5fb6719a`）
> 前置: 自动恢复状态机（「继续生成」/「重新生成」）——本任务只修**恢复之后的提取**，不动恢复状态机。
> 用户指示: 本修复涉及 code_dev / webchat 通道本身（该缺陷会让两者会话在恢复轮被坏文本带偏），由架构师会话直接实现；修复落地后可恢复对 code_dev 的委托。

## 0. 实现须知

1. 先读 `AGENTS.md`，再通读本文件；用 `make dev-test` 迭代，交付前 `go test ./...` + `make fmt-check` 全绿。
2. 顺序建议：**修复 1（核心）→ 修复 2 → 修复 3（调查）**；修复 1、2 各自 fix + test 分开提交；本文档最后以 docs 一笔提交（它当前是未跟踪文件，属于本任务）。
3. **不要 push**；不要 `make install`（review 后由架构师统一处理）。
4. 你自己的会话走同一通道：若本任务期间你的会话出现恢复轮、并被伪 `tags appear more than once` 警告或 `IndexedDB 提取失败` 提示影响，属同一缺陷的表现——按本设计推进，不要被它误导；每轮回复始终只发一个 `<shell>` 块。
5. 交付回报必须含：改了什么、跑了哪些测试及结果（含探针输出摘要）、IDB 调查的输出与结论（如做了）、commit 列表、工作区干净。

## 1. 问题（用户报告）

「已停止」或服务器繁忙引起的自动重试（「继续生成」/「重新生成」）**本身是正确的**：站点随后显示 1/2、2/2 两个版本（2/2 = 重试结果，工具调用格式正确，见截图 2）。但 dscli **取的文本不对**——把旧内容（上一轮消息，甚至已被执行过的块）一起当成本轮回复，于是判定器报告格式错误：

> Your reply contains a malformed `<shell>` block (the tags appear more than once - send exactly one block).

大语言模型被误导（截图 3；模型随后甚至开始相信自己"重复发坏块"）。同一会话（90+ 轮）共发生 6 次：15:27、15:35、15:39、15:44、15:46、15:48 左右；判定失败落在第 23/29/36/39/46 轮（另有第 60 轮 `no-block-short`）。

## 2. 取证（实现以此为准）

### 2.1 事件序列（以 15:46 为例，转写 6695-6766 行）

```
🕐15:45:09  🐋 根据标尺，page-6 底部两图范围是 y≈2040–2250。精裁。
<shell> …块 A（随后被执行成 script91）… </shell>
▶ script91.sh: 精裁 page-6 底部两图（exit=0 …；输出 script91.txt）
📋 继续会话: …（script91 输出回发）
🔄 检测到生成中断（已停止、无输出），已点击「重新生成」重试（1/3）...
⚠️ IndexedDB 提取失败（重试 3 次），使用 DOM 提取文本，可能不完整
🕐15:46:36  🐋 [提取文本 = 块 A（上一轮、已执行的回复，路径被链接化）
                + “裁到页脚了。重新按正确范围裁切。”+ 块 B（2/2 的正确回复）]
⚠️ 专家 的回复未通过 `<shell>` 格式判定（the tags appear more than once …），已请求重发（第 39/1024 轮）
```

- 6 次恢复**全部**伴随 `IndexedDB 提取失败…使用 DOM 提取文本`；正常轮（IDB 成功）从不出现该问题——缺陷只在 DOM 兜底文本被当作回复时显形。
- 提取输出每次从「上一轮消息」开始：上一轮完整消息（甚至已执行过）被重复送进判定器，或直接整段当作本轮回复（15:27 的 script62 即上一轮块被再执行一次）。

### 2.2 根因：DOM 兜底提取没有按「本轮」定界

- 站点会话消息列表按**窗口**挂载（分享页已确证虚拟列表；本会话提取输出每次从"上一轮"开始，与窗口平移一致）——DOM 里并不总有完整历史。
- 现行主提取 = `getAssistantText`（**全部** `.ds-assistant-message-main-content` 拼接）− `stripBaselinePrefix(mdBaseline)`。`stripBaselinePrefix` 是 all-or-nothing 的 `HasPrefix` 前缀剥离；发送本身会 `Navigate` 重建页面，恢复轮等待中页面状态还在变化，**基线前缀匹配不可靠**——实测剥离失效（no-op）→ 窗口内的**发送前内容**（上一轮消息）随全量文本一起返回。
- 现有 fallback-1 `lastAnswerText` 本来就是正确的锚定语义（"sent message 气泡之后的最后一个 main-content"），但主路径先返回非空文本、**提前 return**，它永远没被用到。
- 修复方向：**把锚定语义提升为主路径**，且保持 markdown 还原能力（innerText 会丢围栏/行内代码）。

### 2.3 次要缺陷：自动链接污染代码（同族："取到的结果不对"）

站点把回复中的 URL 形态文本自动包成 `<a>`（含 shell 块内路径）；`markdown.go` 的 `writeInline` case "a" 一律输出 `[text](href)`，于是脚本正文里的路径被注入 `https://` 与 `[]( )`：

- script62（15:27 恢复后）以被链接化的路径执行 → `exit=1`；
- 模型在推理中形成错误认识："the user seems to have some post-processing that converts paths into markdown links"。

证据：转写 3683、4642、6576、6722、6750、7344 行的 `[[https://…][…]]` 形态；对照 IDB 成功轮的干净路径（如 3652、6687 行）。

### 2.4 待查：恢复轮 IDB 读取为何 100% 失败（6/6）

- `webchatExtractReloadedIDB` → `idbGetAssistant`：newest-first 记录里取"第一个 USER + 第一个 ASSISTANT"，要求 `status == "FINISHED"` 且文本可用（`answerUsable`）。
- 假设（需证据）：恢复（新分支/续写同一消息）后，记录里"最新 ASSISTANT"可能是被停止的旧版本（空正文/WIP），或分支顺序与单候选不匹配 → 一直轮询到 `15s × 3` 超时。
- 采集方法建议：新增 **gated 只读 live 探针**（模式参照 `resume_probe_live_test.go`：打开可见浏览器窗口、使用已登录的 dscli profile、不发消息不执行块），导航到上面的会话 URL，dump `deepseek-chat` / `history-message` 记录的元数据：`rec.version`、`rec.timestamp`，以及最后 ~40 条消息的 `{role, status, fragments 类型与长度, 顺序}`，重点定位 script91/92 区域。注意：本机 profile（`~/.dscli/chrome-profile`）的 ldb 值有压缩，**不要离线解**，用 CDP dump。
- 处置规则：
  - 若记录中确有 2/2 且能被更准确的候选选择读到 → 修改 `idbGetAssistant` 选择逻辑（例如：在 newest-first 中取"位于最新 USER 之前（索引更小）、最新且 `FINISHED`、文本可用"的 ASSISTANT；**防陈旧守卫不变**），同步更新 `TestJsIDBGetAnswerFmtNewestFirst` 回归；
  - 若记录写入时序/结构不可控 → 把结论写进本文件，依赖修复 1 让 DOM 兜底正确即可（兜底恢复为"可信但降级"）。
- **已查明（2026-09-15，只读 live 探针）**：记录中该轮助手组 = [被停止的 1/2（仅 THINK、无 RESPONSE）, 2/2（THINK+RESPONSE）]（创建顺序）；旧"第一个 ASSISTANT"规则取到空文本的被停止版本 → 6/6 失败。已按第一条处置规则修复（见 §10）。

## 3. 设计 A（must）：按 sentMessage 锚定本轮回复

### 3.1 新 JS 常量 `jsAssistantTextAfterFmt`（internal/lp/webchat.go）

- 复用 `jsLastAnswerAfterFmt` 的锚点扫描：把所有 `.ds-message` 按 document 顺序编号，从后往前找 `firstLine === want || t.trim().indexOf(want) === 0` 的气泡；`want` 为空直接返回空结果。
- 返回**锚点之后**的全部 `.ds-assistant-message-main-content`（限定在 `.ds-message` 内）的 `innerHTML` 数组；若为空，再退回锚点之后的 `.ds-markdown`（沿用 `jsGetAssistantText` 的"排除已思考块"检查）。
- 与既有 JS 常量同风格；注释写 why（虚拟列表 + 发送/重载使基线前缀不可靠；锚定不依赖任何基线快照）。
- `%s` 为 `quoteJS(sentMessage)`；JS 内不得出现 `.click(` / `dispatchEvent(`（沿用探针回归断言）。

### 3.2 Go 包装 `assistantTextAfterMessage(ctx, sentMessage) string`

- `sentMessage` 为空 → `""`；`chromedp.Evaluate(fmt.Sprintf(jsAssistantTextAfterFmt, quoteJS(sentMessage)), &parts)` → `joinMarkdown(parts)`；求值错误 → `""`。

### 3.3 `stabilityStep` 接线（主路径替换，控制流尽量少动）

选择顺序（新）：

1. **主**：`assistantTextAfterMessage(ctx, in.sentMessage)`；非空 → `stripUIChromePrefix` → 与现行主分支相同的 busy / truncated / complete 检查语义（有文本但不可提取时 `return false` 继续轮询）。
2. 兜底链（沿用现有 fallback 语义与顺序）：`lastAnswerText` → `extractAfterMessage` → **旧全量路径降为最后**（`getAssistantText` + `stripBaselinePrefix(mdBaseline)`）。
3. 全空 → 现有空稳定计数逻辑不变。

要点：

- `mdBaseline` 继续采集（仅旧兜底用）；`webchatSend`、恢复状态机（`continueRecovery` / `jsContinueGeneration` / `jsRegenerateStopped`）、`handle.go` 重试策略**一律不改**（用户已确认"重试是对的"）。
- 更新 `webchatWait` 顶部 doc 注释：主提取以 `sentMessage` 锚定；基线剥离仅为最后兜底；说明 why。
- 非目标：不处理"非激活版本仍挂载"的假设场景——重新生成只发生在"无输出"状态，1/2 版本无正文，不可能被带上（写入 §9 风险，出现新症状再查）。

## 4. 设计 B（应做）：站内自动链接不进代码

- `markdown.go` `writeInline` case "a"：新增判定 `isAutoLink(href, text)`（`href == text`、`href == "http://"+text`、`href == "https://"+text`）→ **只输出 `text`**；否则维持 `[text](href)`。
- 注释写 why：站点把 URL 形态文本自动包链接（含 shell 块与行内代码内），`[text](href)` 会把 `https://` 和 `[]( )` 注入代码/路径文本，破坏执行（script62 实证）。
- 测试：`markdown_test.go` 增加用例——自动链接 → 原样文本；真链接 `[描述](url)` → 保持；`http://` 前缀变体；行内代码内的自动链接。
- 已知取舍：模型有意写"描述恰等于 URL 去 scheme"的链接时会退化为纯文本——可接受（内容不丢，仅丢链接形态），写进注释。

## 5. 设计 C（调查）：恢复轮 IDB 读取

见 §2.4。先取证（gated 只读探针），再按处置规则决定改不改；无论结论如何，把证据与结论写回本文件 §状态。

## 6. 改动文件（预期）

| 文件 | 改动 |
|---|---|
| `internal/lp/webchat.go` | `jsAssistantTextAfterFmt` + `assistantTextAfterMessage` + `stabilityStep` 接线 + 注释 |
| `internal/lp/webchat_test.go` | `TestJsAssistantTextAfterFmt` 字符串回归（含空 want 守卫、无 `.click(`/`dispatchEvent(`） |
| `internal/lp/extract_probe_live_test.go`（新增） | gated fixture 探针：锚定提取只取 2/2 内容、链接不污染代码 |
| `internal/lp/markdown.go` + `markdown_test.go` | 自动链接保真 |
| `internal/lp/*_probe_live_test.go`（可选，新增） | 只读 IDB 记录 dump 探针（调查用） |
| `internal/lp/webchat.go` + `webchat_test.go`（仅当 IDB 修复成立） | 候选选择修正 + 回归 |
| `docs/task-webchat-recovery-extraction.md` | 本文件（回填状态/实测） |
| `AGENTS.md` | `internal/lp/` 行补一句：提取以 sentMessage 锚定（英文） |

## 7. 测试与验收

A. **字符串回归** `TestJsAssistantTextAfterFmt`：必须包含 `ds-message`、`ds-assistant-message-main-content`、`closest`、`innerHTML`、`ord`、空 want 守卫；必须不包含 `.click(`、`dispatchEvent(`。
B. **fixture 探针** `TestLiveScopedExtractProbe`（gated，headless + `file://`，模式同 `continue_probe_live_test.go`）：
   1. 页含 [更早助手消息（带块） + 用户气泡（含 sentMessage 文本） + 已停止 v1（无正文） + 2/2 消息（带正确块）] → 断言 `assistantTextAfterMessage` 只返回 2/2 内容，不含更早内容；
   2. 2/2 内容含 `<a href="https://…">…</a>` 形态路径 → 断言 `joinMarkdown` 输出为纯文本（无 `](`）；
   3. 锚点缺失页 → 返回空（落兜底）。
C. **markdown 单测**（设计 B）。
D. **IDB 调查产物**：探针输出摘要 + 结论（改 / 不改 + 理由），回填本文件。
E. **全量**：`go test ./...`、`make fmt-check`、`make gofmt` 全绿；改动文件 gocyclo ≤ 20。
F. **交付回报**：上述证据 + commit 列表（英文）+ 工作区干净 + 未 push。

## 8. 提交计划（英文，可合并但 fix/test/docs 可分辨）

1. `fix(lp): scope the DOM reply extraction to the sent message`
2. `test(lp): pin the scoped extraction and its fixture probe`
3. `fix(lp): keep auto-linked URLs literal in DOM extraction`
4. （仅当调查支持）`fix(lp): read the regenerated answer from IndexedDB after a recovery`
5. `docs: task record for the recovery extraction fix`

## 9. 开放风险

- 站点虚拟列表窗口策略变化：锚定只依赖"sentMessage 气泡与回复元素同窗"，若气泡被回收 → 返回空 → 落旧兜底（行为=现状，记录在案）。
- 锚点误配（sentMessage 在页面别处重复出现）→ 取**最后**一个匹配气泡，仍在本轮下方。
- 设计 B 取舍：自题链接退化为纯文本。
- 非激活版本若未来在 DOM 中仍挂载正文（当前不可能，因重新生成仅发生在无输出状态）→ 需补"可见性过滤"，届时再加。
- 锚点回显防线（跳过带 main-content 的回复气泡）依赖该 class 存在；其被移除的降级布局下回显风险回归为"返回空→落兜底"，不会取错文本。
- DOM 路径拼接锚点之后的**所有** main-content：本任务的自动恢复只产生"1/2 无正文"，不会触发；但若对**已完成**回复手动「重新生成」，可能出现两个带正文尝试被拼接（backlog——若要修，需在 DOM 侧按可见版本过滤）。
- 跳过"含 main-content 的气泡"是无条件启发式：若站点把引用旧回复渲染进用户气泡且带该 class，锚点会落空（→ 返回空→落兜底，不会取错文本）；无实证据，列为观察项。

## 10. 实现记录（回填，2026-09-15）

- **修复 1（锚定提取）**：新增 `jsAssistantTextAfterFmt` / `assistantTextAfterMessage`；`stabilityStep` 主路径改为"sentMessage 气泡之后的答案块"（innerHTML→markdown），旧"全量 + `stripBaselinePrefix`"降为最后兜底，`classifyReply` 收敛共享的 busy/truncated/complete 判定。夹具探针 `TestLiveScopedExtractProbe`：只取 2/2、排除已执行旧轮与空 1/2、自动链接保持字面、锚点缺失返回空。
- **修复 2（自动链接保真）**：`isAutoLink`（`href == text` / `scheme + text`）→ 纯文本；`TestMarkdownFragmentAutoLinkLiteral`、`TestIsAutoLink`。
- **修复 3（IDB 读取）**：只读 live 探针 `TestLiveIDBRecordProbe`（`DSCLI_IDB_PROBE_MATCH` 窗口模式）在真实记录中定位事件轮（会话 `ff6ac088-…`，索引 59-61）：`[USER(script91 反馈), ASSISTANT(被停止：仅 THINK、无 RESPONSE), ASSISTANT(2/2：THINK + RESPONSE「裁到页脚了…」)]`，两者 `status` 均为 `FINISHED`——旧规则取"第一个 ASSISTANT"即空文本的被停止版本 → `answerUsable` 恒拒绝 → 6/6 失败。读取改为在"命中 USER 之后、下一 USER 之前"的助手组内取**最后一个非空 RESPONSE** 版本；`TestJsIDBGetAnswerFmtRegeneratedRound` + `TestLiveIDBSelectProbe`（5 用例）钉住。版本顺序以实测（创建顺序、被停止版本在前）为准；"取最后一个非空"对两种顺序均安全（探针第 5 用例）。
- **验证**：`go test ./...`、`make fmt-check`、`make gofmt` 全绿；`DSCLI_LIVE_EXTRACT_PROBE=1 go test -v -run 'TestLiveScopedExtractProbe|TestLiveIDBSelectProbe' ./internal/lp/` 全过。
- **提交**：`43211ac`（修复 1 + 3）、`b641de1`（修复 2）、`05ea1dc`（测试与探针）、本文件 docs 一笔。
- **review 跟进（2026-09-15）**：锚点回显守卫（跳过带 main-content 的回复气泡）；探针补"回显不夺锚"、"两个都有正文的尝试取最新"、"尝试在 USER 之前"三个用例；新增 `TestClassifyReply`；`jsIDBGetAnswerFmt` 注释统一为"轮间 newest-first、轮内创建顺序"；AGENTS 行精简。round-2 跟进：锚点守卫的非 gated 字符串钉、`jsLastAnswerAfterFmt` 镜像同款守卫、`TestClassifyReply` 长度边界行、探针措辞（§9 增 backlog 两条）。
- **开放项（真机验收）**：下一次真实恢复轮（服务器繁忙/停止 → 自动重试）确认判定器看到 2/2 完整回复，且不再在 `IndexedDB 提取失败` 之后出现伪 `tags appear more than once`。
