# task-stop-recovery-and-residue: 「已停止」自动重新生成 + DSML 标记残留及时告警

> 状态: 已实现（codedev 分支；`go test ./...`、`make fmt-check` 全绿）
>
> 实测摘要（2026-09-13）：
> - 夹具探针 `DSCLI_LIVE_CLICK_PROBE=1 go test -v -run 'TestLiveRegenerateProbe|TestLiveContinueProbe' ./internal/lp/`（headless Chromium + `file://` 夹具 + 临时 profile，不触网）：
>   - 「重新生成」目标页：`present=true, clickable=true, buttons=5, x=153.1, y=26.0`；坐标经 `elementFromPoint` 命中第 2 个按钮；`clickTrustedAt` 后 regen 命中计数 = 1；
>   - 反证页（有正文 / 正文含「已停止」/ 无操作栏）均 `present=false`；
>   - 「继续生成」探针保持全绿（`trusted=1 synthetic=0 lastTrusted=true`，合成点击仍被守卫拒绝）。
> - `internal/dsml.MarkerRanges` 覆盖 `testdata/{case1_edit_argument,case2_write_file,real_sample2,case6_bare_short_close}.txt` 真样本（分别命中 41/19/9/4 段），裸竖线不误报。
> - DSML 通道顺手验证：四个真样本经 `ParseDSMLMessage` 均 `OK=false`（strict 路径已覆盖，无缺口，未扩 scope）。
> - 执行路径残留提醒已随反馈发出（`shell_loop_test` 断言带残留→含 `ResidueNote()`，干净→不含，且 `output of scriptN.sh` 前缀不变）。
>
> 真机验收（会话外，无法主动复现 busy-stop）：待下一次服务器繁忙自然发生后观察；开放项，见 §6.G。
> 依据: 用户 bug 报告（截图 `屏幕截图_20260913_165129.png`、`屏幕截图_20260913_161743.png`，2026-09-13 真实 code_dev/shell 会话）+ 现行站点 bundle 逆向取证（`main.d69e3d8c16.js`，页头 commit-id `5d128f98`，2026-09-13；取证缓存 `/tmp/dsk-bundle/`，若已丢失以本文档记载的事实为准）
> 前置: `docs/task-continue-generation.md`（「继续生成」自动续写，已实现）——本任务扩展同一恢复状态机，并补齐 shell 通道的 DSML 残留告警
> 日期: 2026-09-13

## 0. 实现须知（dev）

1. 先读 `AGENTS.md`（构建/测试约定），再通读本文件；
2. 两个问题互不依赖：建议**问题 1（§3）先行成 commit，再做问题 2（§4）**；各自 fix + test 分开提交，最后 docs 一笔；
3. **传输安全约束（重要）**：本仓库的 web 通道会把含「全角竖线序列」的文本篡改/截断（本文件初稿即被截断于此）。因此：
   - 新增代码/测试里构造 DSML 标记样本**一律用 Go 转义（如 `"\uFF5C"`）或运行时拼接**，先例：`internal/dsml/dsml_strayclose_test.go` 头部注释（`lt/gt/q/bt` 运行时构造）——照此办理；
   - 不要经由会话通道粘贴原始的「竖线+DSML+竖线」字节序列到新文件里；
   - 字节精确的真实样本已有：`internal/dsml/testdata/case1_edit_argument.txt`、`case2_write_file.txt`、`real_sample2.txt`（含两种变体），测试可以直接读它们当输入。
4. 交付回报必须含：改了什么、跑了哪些测试及结果（含门控探针 `DSCLI_LIVE_CLICK_PROBE=1` 的本机输出摘要）、commit 列表、工作区干净。

## 1. 问题（用户报告）

两个问题，均出现在服务器繁忙导致的真实中断会话中（截图见上）。

**问题 1 - 「已停止」需要自动点「重新生成」**（截图 1，code_dev/shell 会话 `0683ae46…`）：

服务器繁忙在**思考阶段**就终止了生成。最后一条 assistant 消息只有「已停止」思考头部 + 一条思考内容，**没有答案正文**；消息行底部是常规操作栏（复制 / 重新生成 / 赞 / 踩 / 分享），**没有**「继续生成」按钮；页面静止。用户手工点击操作栏的「重新生成」(↻) 后生成恢复。

现有 `continueRecovery` 只处理「继续生成」按钮，该状态没有任何恢复路径：稳定判定会走到"无内容稳定" → 约 60s（`webChatEmptyStablePolls=30`）后 `ErrServerBusy` → 重试策略把同一条 `output of script222.sh` 整条再发一遍（会话里出现重复消息、模型整轮工作重做），或直接等到轮询预算超时。

**需求：检测该状态，自动点击「重新生成」（与用户手工动作一致的真 CDP 点击）。**

**问题 2 - DSML 标记残留需要及时告警**（截图 1 的消息 A + 截图 2，两个 shell 会话）：

模型回复在**合法 `<shell>` 块之后**带出 DSML 闭合标记残留。截图中的渲染近似（空格为换行/字距所致，非字节）：

```
</ | | DSML | | parameter> </ | | DSML | | invoke> </ | | DSML | | calls>
```

其字节形态为：`</` + 两个全角竖线（U+FF5C）+ 字面 `DSML` + 两个全角竖线 + 标签名 + `>`；另有 ASCII 变体用两个半角 `|`。仓库 `internal/dsml/testdata/` 的多个真实样本即此形态（含 `_calls` 结尾变体）。

dscli 的 shell 判定**先** extract 到合法块 → `ActionExecute`，块正常执行（截图里 script208 / script222 都跑了），残留被**静默忽略**：没有任何警告。**需求：出现 DSML 标记时及时警告。**

## 2. 站点事实（bundle 取证，实现必须以这些为准）

### 2.1 供问题 1（「已停止」态与「重新生成」按钮）

1. 思考头部标题 = `useCollapsibleAreaTitle`：消息若**有答案正文片段**（`getPossibleResponseFragments(msg).length > 0`）→ 「已思考（用时 N 秒）」；否则若 `!isWIP(getMessageUIStatus(msg))` → `messageThinkStopped` = **「已停止」**（英文包 `"Stopped"`）；否则为「正在思考」/工具类文案。
2. 站点自身的「停止」谓词 `useCollapsibleAreaStopped` = `!!msg && !getPossibleResponseFragments(msg).length && getMessageUIStatus(msg) !== WIP` —— 即 **无答案正文 + 非 WIP**。目标状态以此定义。
3. 头部组件（`pj`）外层是 `<div>`（onClick 折叠），标题经 `pT` 渲染为 **`<span>`**；**不是** `button`/`[role=button]`。因此 `jsIsGenerationActive`（扫描 button 文本含 停止/stop）**不会**在该状态误报「生成中」——停止态下 `isGenerationActive` = false（本设计可安全地把 `!isGenerationActive` 当作进入条件）。
4. 操作栏（组件 `cB`，位于消息 footer 内的 flex 容器）：按钮顺序 `[复制(dm), 重新生成(cU), 赞(cH), 踩(cV), 语音(cv，tts 特性关闭时为 null), 分享(cW)]`，渲染顺序固定；「重新生成」恒为**第 2 个**。截图实况 = 5 个图标（📋 ↻ 👍 👎 ↗），与 cv=null 一致。
5. 「重新生成」按钮（`cU`）：icon-only（`tx.$`，`variant:"icon"`），**无 text / aria-label / title**（提示是悬浮 tooltip，不是属性）→ 只能用**位置/结构**识别。onClick 直接调 `regenerateMessage({chatSessionId, childMessageId, searchEnabled, userOptions, getPowRes, thinkingEnabled, allowParallelStreams})`；**无 isTrusted 校验**（全 bundle 唯一的 isTrusted 校验在「继续生成」按钮上）。禁用条件：`(!allowParallelStreams && hasOngoingGeneration) || 状态∈{CONTENT_FILTER, CONTEXT_LENGTH_EXCEEDED} || banRegenerate`；`aria-disabled` 同步设置。
6. 点击行为（`S1.initFakeMessages` + `regenerateMessage`）：取 `childMessageId` 的**父 user 消息**，新建一条 assistant 消息（新 id、新分支）挂到同一父下——即"为同一轮对话重新生成一次新尝试"；被停掉的半截消息保留为旧分支。**这与用户手工点击后的效果一致，是本设计要自动化的动作。**
7. 「继续生成」按钮（`ur`）显示条件不变：`INCOMPLETE + 叶子 + 非 WIP`。目标状态（已停止、无正文）不满足 → 没有该按钮。**恢复优先级：先「继续生成」（存在即可点，同一消息续写），未命中才轮到「重新生成」。**
8. 状态取值未完全取证（终端非 WIP；非 INCOMPLETE——否则会出现「继续生成」按钮）。检测**不依赖具体状态串**，只依赖可观察 UI 事实（见 §3.1）。

### 2.2 供问题 2（DSML 标记形态）

9. 站点 IndexedDB 存的是**渲染形态**：`</` + 全角竖线×2 + `DSML` + 全角竖线×2 + 标签名 + `>`（真实样本见 testdata；ASCII 变体 `</||DSML||invoke>` 亦在样本中出现）。`normalizeDSMLText` 已能把它们正规化回 `</parameter>` / `</invoke>` / `</_calls>`（已知名表 `invoke|parameter|tool_calls|_calls`）。
10. **shell 判定的关键机制**：`shellblock.Judge` 先 `extract()`；一旦找到合法块即 `return Verdict{Action: ActionExecute}`，**其后的所有 DSML 形状检查都不会再跑**。这就是"块执行 + 残留静默"的机制性原因。无块回复的 DSML 形状检查（`hasDSMLShape`，含单字符全角竖线）已存在。
11. DSML 通道（tool loop）对解析出的调用 + stray 闭合已有严格违规路径（strict=true → `InjectStrictWarning` 随 tool_result 注入 + stderr 提示）；shell 通道没有对应物。本任务补齐 **shell 通道**；DSML 通道仅在实现中顺手验证（用 testdata 样本跑一遍现有判定即可），**如发现缺口，记录到回报，不扩scope**。

## 3. 设计 A：自动点击「重新生成」（internal/lp/webchat.go）

### 3.1 检测 JS：`jsRegenerateStopped`（只检测、不点击，纪律同 `jsContinueGeneration`）

返回 `{found:true, clickable, x, y, buttons}` 或 `{found:false}`。判定（全部满足才算 found）：

1. **最后一个** `.ds-message` 气泡（document 顺序取最后一个）存在；无 → false。
2. **无答案正文**：气泡内 `.ds-assistant-message-main-content` 不存在、或 trim 后为空。否则 → false。
3. **「已停止」标记**：气泡内（**排除** `.ds-assistant-message-main-content` 子树；跳过 `script`/`style`）存在叶子元素，其 `textContent` 经 trim + lowercase 后**精确等于** `已停止` 或 `stopped`。否则 → false。（精确匹配 + 子树排除：答案正文/思考 bullet 里的同样字样不会命中；事实 1/2 说明该文案只在"无正文 + 非 WIP"时渲染。）
4. **操作栏定位**：在"拥有直接子级 `.ds-message`"的行容器内（沿用 `jsContinueGeneration` 的所有权 walk），在气泡子树**之外**找**第一个**满足"直接子级里含 **≥4 个可见 icon-only 按钮**（`button, [role=button]`；`textContent` trim 为空；含 `svg`）"的元素 E——即操作栏（事实 4 的 5 连排；跳过了分支切换器/文字按钮）。E 的**第 2 个**这样的按钮子级 = 「重新生成」（事实 4/5）。找不到 E 或第 2 个按钮 → false。
5. `clickable`：按钮 enabled（`!b.disabled && aria-disabled !== 'true'`）、可见（`offsetParent === null` 时以零尺寸 rect 判隐藏，沿用既有口径）、`scrollIntoView({block:'center'})` 后取中心坐标、(x,y) 经 `document.elementFromPoint` 复核命中（含子元素）。存在但不可点 → `clickable:false`（**不得**折叠为 found:false，见 3.3 hold 语义）。
6. JS 内**不得出现** `.click(` / `dispatchEvent(`（探针回归断言）。坐标/字段名与 `jsContinueGeneration` 风格一致。

### 3.2 Go 侧

```go
// regenerateDetect: {present, clickable bool; x, y float64; buttons int}
// regenerateStoppedButton(ctx) (regenerateDetect, error)   // 评估错误上抛
```

`clickTrustedAt` 原样复用（同一 CDP 真点击路径；该按钮虽无 isTrusted 校验，仍走真实输入管线，避免与任何未来守卫/React 合成事件边界纠缠）。**新增常量**：`webChatMaxRegenerates = 3`；冷却与恢复窗口**复用** `webChatContinueCooldown` / `webChatContinueResumeWindow`（注释改为"两类点击共用"）；派发失败上限复用 `webChatMaxContinueClickFailures`。

### 3.3 `continueRecovery` 扩展（核心接线）

新字段：`regenClicks int`（预算 `webChatMaxRegenerates`）、`pendingKind`（这次 pending 来自哪种点击，决定超时错误文案）。`clicks` 保留给「继续生成」。`now/active/detect/clickAt` 的 seam 机制不变，**新增可注入 seam** `detectRegen func(context.Context) (regenerateDetect, error)`（nil = 生产实现）。

`step` 顺序（gate 不变；其余改为）：

```
gate（两类共用 pending/deadline/base/bodyBase/证据语义，不变）
detect「继续生成」：
- 出错 → 记一次 stderr（复用 warned）→ continueHold
- present：
      可点且 ready() → click（既有逻辑）；失败/预算/连续失败 → 既有错误
      未点 → continueHold
- 未 present 且 !activeFn(ctx)（关键守卫：生成活跃时绝不进入）：
detect「重新生成-停止态」：
- 出错 → 记一次 stderr → continueHold
- present：
      可点且 ready() → clickRegen；预算超限 → ErrTruncated("自动点击「重新生成」N 次仍未完成（服务器中断）")；
      派发失败不计预算、连续 3 次失败 → ErrTruncated（双 %w：ErrTruncated + 底层错误）
      未点 → continueHold
- 其余 → 若 pending 则 continueHold，否则 continueNone
```

要点：

- **优先级**：先「继续生成」后「重新生成」；「继续生成」存在时（INCOMPLETE+叶子）点击它，不再进入重新生成分支。
- **点击后**：`pending=true`、`pendingKind=regen`、`bodyBase=body`；`answer()` 此刻为 ""（无正文）→ base 退化为 body 文本，恢复证据 = `isGenerationActive` 或 body 变化（新尝试气泡出现即变化）→ 按既有 `resumed` 逻辑清除 pending。重置 stable 计数（与 continueClicked 相同，返回同一 `continueClicked` 动作）。
- **重复点击**：冷却内不重点；冷却过后仍 present 且预算未耗尽 → 继续点（与「继续生成」一致，容忍首次点击未生效）。点击「重新生成」后若新尝试又秒停（无正文、仍非 WIP）→ 下一轮再次命中 → 预算内再点，直至 `webChatMaxRegenerates`；之后 `ErrTruncated` 交给既有重试策略（follow-up 会发 `webChatContinueWarningShell`，是错误状态下的正确兜底）。
- **stderr 文案**（风格与既有一致）：
- 点击：`🔄 检测到生成中断（已停止、无输出），已点击「重新生成」重试（%d/%d）...`
- 检测失败：`⚠️ 检测「重新生成」按钮失败（将继续轮询）: %v`
- 超时：`%w: 已自动点击「重新生成」%d 次，生成仍未恢复（服务器中断）`
- **不改**：`jsContinueGeneration` 及其"重新生成不得命中"的排除表；`isGenerationActive`（事实 3 证明其安全）；`handle.go` 重试策略；`webchatWait` 主循环结构（恢复逻辑仍收在 `cont.step` 一处）。
- 更新 `webchatWait` 顶部 doc 注释：恢复机制清单补「重新生成」自动恢复一条（含 isTrusted 事实的对照说明：该按钮无守卫但同样走 `clickTrustedAt`）。
- `ErrTruncated` doc 注释补一档："生成被服务器中断（无输出）且自动重新生成未成功"。

## 4. 设计 B：DSML 标记残留告警（internal/dsml + internal/shellblock + internal/lp/handle.go）

### 4.1 新谓词（internal/dsml）

新增导出函数（命名建议 `MarkerRanges`）：

```go
// MarkerRanges reports DSML marker shapes in text as byte ranges [start,end),
// in RAW coordinates so callers can filter by their own ranges (quoted code,
// an executed shell block). Detection arms (case-insensitive):
//   1. noise-prefixed open/close: `</?`, then one or more noise tokens, then a
//      tag name (`invoke|parameter|tool_calls|_calls|calls`). A noise token is
//      a bar (ASCII `|` or full-width U+FF5C) or the literal `DSML` (letters
//      may be whitespace-separated); whitespace between tokens is tolerated.
//      Real stored form (see testdata): `</` + bar*2 + `DSML` + bar*2 + `parameter`.
//   2. plain closes: `</invoke>`, `</parameter>`, `</tool_calls>`, `</_calls>` (whitespace tolerated)
//   3. plain opens: `<invoke` / `<tool_calls` (word boundary)
// Regex arms use `[\x7c\x{FF5C}]` for the bar class and `d\s*s\s*m\s*l` for
// the DSML token (models may split letters). A bare bar in prose never matches
// (arm 1 requires a tag name after the noise).
func MarkerRanges(text string) [][2]int
```

（实现细节可微调，但**必须**满足：原始坐标、大小写不敏感、覆盖 testdata 中的真实样本字节、裸竖线不误报。）

### 4.2 shellblock 接入

- `Verdict` 新增字段 `ResidualMarkers bool`：**所有** verdict 分支都计算——扫描范围 = 原文 −（`dsml.CodeRanges` 引号区）−（ActionExecute 时：extract 得到的块 span = 首个 `<shell>` 行首到 `</shell>` 行尾）。只要 `MarkerRanges` 有命中即置 true。注意：块 span 排除是**必须**的——脚本正文里出现 `DSML` 字样/标签形状是合法内容（本仓库大量存在）。
- `hasDSMLShape`（仅决定警告文案是否附 DSML note）扩一臂：`MarkerRanges(text)` 非空也算。`hasDSMLCallShape`（路由用）**保持不变**。

### 4.3 shell 循环接线（internal/lp/handle.go `handleWebChatShellLoop`）

- `ActionExecute` + `verdict.ResidualMarkers`：块照常执行；**在执行后的反馈消息尾追加提示**（用户要求的"及时"落点=本轮反馈随行，不额外花一轮）：

```
  feedback := fmt.Sprintf("output of script%d.sh (attached as script%d.txt):", ...)
  if verdict.ResidualMarkers {
  feedback += "\n\n" + shellblock.ResidueNote()
  }
```

并打印 stderr：`⚠️ 回复携带 DSML 标记残留（块已执行；提醒已随反馈发出）`。

- `ActionWarn`：无条件沿用既有路径；`MalformedWarning`/`NoBlockWarning` 的 dsmlShape 参数现在也会对 ASCII/混合变体为真（见 4.2），文案自然覆盖。
- `ActionFinal` + `verdict.ResidualMarkers`：返回前打印 stderr `⚠️ 最终回复携带 DSML 标记残留（站点会打徽章/篡改这类标记）`；**不改路由**。
- `internal/shellblock/warning.go` 新增导出 `func ResidueNote() string`（文案要求：说明回复带 DSML 标记残留、本通道会打徽章/篡改、请保持每轮仅一个 `<shell>` 块；风格对齐现有 `dsmlNote`）。同时把既有 `dsmlNote` 的用词与新 note 对齐（可选，保持简单）。
- shell 循环 doc 注释更新：补一句"执行路径同样检查标记残留并随反馈提醒"。

### 4.4 非目标（本问题内）

- 不给 DSML 通道新增 stderr/路由（其 strict 路径已覆盖，测试里顺手验证即可）；
- 不改 `hasDSMLCallShape` / 路由判定（避免把"引用形态"的最终回复误升级为警告轮）；
- 不做流中检测（判定仍以稳定回复为单位——"及时"=判定时刻立即告警，而非等待会话结束）。

## 5. 改动文件

| 文件 | 改动 |
|---|---|
| `internal/lp/webchat.go` | `jsRegenerateStopped` + `regenerateStoppedButton` + `regenerateDetect` + `continueRecovery` 扩展（字段/step/clickRegen/文案）+ 常量 + doc 注释 |
| `internal/lp/webchat_test.go` | `TestJsRegenerateStopped` 字符串回归（含坐标/字段断言、无 `.click(`/`dispatchEvent(` 断言） |
| `internal/lp/continue_recovery_test.go` | 扩展表驱动用例：regen 路径（检测/优先级/守卫/冷却/预算/失败/证据/超时/错误） |
| `internal/lp/regenerate_probe_live_test.go`（新增） | 门控夹具探针 `TestLiveRegenerateProbe`（结构镜像 `continue_probe_live_test.go`） |
| `internal/dsml/dsml.go` | `MarkerRanges` + 注释 |
| `internal/dsml/dsml_markers_test.go`（新增） | 谓词单测（含 testdata 真样本） |
| `internal/shellblock/shellblock.go` | `Verdict.ResidualMarkers` + Judge 计算（quoted/块 span 排除）+ doc 注释 |
| `internal/shellblock/warning.go` | `ResidueNote()` + `hasDSMLShape` 扩臂 |
| `internal/shellblock/judge_test.go` | 残留用例（见 §6.D） |
| `internal/lp/handle.go` | shell 循环：Execute 反馈追加 + stderr；Final stderr；注释更新 |
| `internal/lp/shell_loop_test.go` | 反馈断言扩展（带残留→含 note；干净→不含） |
| `docs/task-stop-recovery-and-residue.md` | 本文件（实现后回填状态与实测摘要） |
| `AGENTS.md` | `internal/lp/` 表行补一句自动重新生成；`internal/dsml/`、`internal/shellblock/` 行补残留告警（英文） |

## 6. 测试与验收

A. **字符串回归**（`TestJsRegenerateStopped`，风格同 `TestJsContinueGeneration`）：
- 必须包含：`已停止`、`stopped`、`ds-message`、`ds-assistant-message-main-content`、`aria-disabled`、`offsetParent`、`getBoundingClientRect`、`scrollIntoView`、`elementFromPoint`、返回字段 `x`/`y`/`buttons`；
- **必须不包含**：`.click(`、`dispatchEvent(`。

B. **恢复状态机单测**（`continue_recovery_test.go` 扩展，注入 seams，无浏览器）。至少覆盖：
1. 停止态命中 + 空闲 → 点击 1 次、动作 `continueClicked`、pending 置位（kind=regen）；
2. `active=true` 时同状态 → **不点**（守卫）；
3. 「继续生成」present 且停止态也在 → 优先点「继续生成」，regen 不被点；
4. regen present 但 `clickable=false` → hold、不点；
5. 冷却内不重点，冷却后且仍 present → 在预算内再点；
6. 预算 3 次耗尽 → `errors.Is(err, ErrTruncated)`；
7. 派发失败不计预算、连续 3 次失败 → ErrTruncated（双 %w 可达底层错误）；
8. 证据：pending + active → 清除；pending + body 变化 → 清除；pending 超窗 → ErrTruncated（文案含「重新生成」）；
9. 检测错误 → hold + 只告警一次；
10. 停止态消失且 pending 已清 → `continueNone`。

C. **门控夹具探针**（`DSCLI_LIVE_CLICK_PROBE=1`，默认 skip；本机有 chromium）：
- 夹具页 1（目标）：行结构 `[avatar, .ds-message(气泡内: 头部 div>span「已停止」; 无 main-content), footer: 5 个 icon `<button>`(含 svg) — 第 2 个=regen]`；断言 detector `present=true, clickable=true`、坐标经 `elementFromPoint` 命中第 2 个按钮；`clickTrustedAt` 后按钮计数=1；
- 夹具页 2（反证）：同结构但 `main-content` 有正文 → `present=false`；
- 夹具页 3（反证）：「已停止」字样出现在 `main-content` 正文里 → `present=false`；
- 夹具页 4（反证）：无操作栏 → `present=false`；
- 交付前本机跑一次并在回报中附输出摘要。

D. **shellblock 判定用例**（`judge_test.go`，样本用转义/运行时构造）：
1. 合法块 + 尾部残留（fullwidth 与 ASCII 各一）→ `ActionExecute` + `ResidualMarkers=true`；
2. 合法块（干净）→ `ResidualMarkers=false`；
3. 残留出现在 **script 正文内**（块 span 内）→ `ResidualMarkers=false`（防误报，本仓库场景）；
4. 残留出现在**引号代码**内 → `false`；
5. 无块 + 残留 → 既有 `ActionWarn`（含 dsml note）+ `ResidualMarkers=true`；
6. 读 `testdata/case2_write_file.txt` 等真实样本构造等价用例（可用子串切片）。

E. **shell 循环反馈断言**（`shell_loop_test.go` 扩展）：
- 带残留的 Execute 轮 → 反馈消息含 `ResidueNote()` 文案；干净轮 → 不含；
- 既有 `output of scriptN.sh (attached as scriptN.txt):` 前缀断言保持通过（note 在 `\n\n` 之后）。

F. **全量**：`go test ./...`、`make fmt-check`、`make gofmt` 全绿。

G. **真机验收（会话外，无法主动复现）**：与「继续生成」同款开放项——下一次服务器繁忙自然产生「已停止」态时，观察无需人工点击即可自动重新生成；残留告警也随之自然出现。回报中列为开放项。

## 7. 非目标 / 开放风险

- **不**处理"有正文 + 无「继续生成」按钮"的其他终端状态（无证据；`ErrServerBusy` 兜底路径保持）。
- **不**做流中检测；判定以稳定回复为单位。
- 风险：站点改版（文案/结构漂移）。字符串与结构均取自现行 bundle 并在此留档；探针夹具钉住机制；漂移需下次取证更新。
- 风险：「已停止」标记在个别布局变体（`useShowCollapsibleGroupTitle` 为假的形态）可能不渲染 → 检测落空 → 退化为既有 60s 兜底。**取舍**：标记为必需信号（宁可漏点，不可误点健康轮次）。
- 风险：未加引号的正文提及标记形状 → 多一条 stderr 告警/反馈 note（廉价；引号代码与块 span 均已排除）。
- 风险：点击按坐标派发，检测与点击之间位移可能落空；缓解 = 同轮检测后立即点击 + 冷却重试 + 预算 + `ErrTruncated` 兜底（与既有「继续生成」同款）。

## 8. 提交计划（英文，codedev 分支）

1. `fix(lp): auto-regenerate a server-stopped round with no output`
2. `test(lp): pin the stopped-state regenerate recovery and probe`
3. `fix(shellblock): flag DSML marker residue after an executed block`
4. `test(dsml,shellblock,lp): pin residue detection and warnings`
5. `docs: task record for stop recovery and residue warnings`

（可合并为更少提交，但 fix/test/docs 必须可分辨；全部英文。）
