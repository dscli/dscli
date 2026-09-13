# task-continue-generation: 服务器繁忙导致生成中断时自动点击「继续生成」

> 状态: 已实现（codedev 分支；`go test ./...` 与 `make fmt-check` 全绿）
> 依据: 用户 bug 报告（截图 `屏幕截图_20260913_145522.png`，code_dev 会话）+ 现行站点 bundle 逆向取证（`main.d69e3d8c16.js`，页头 commit-id `5d128f98`，2026-09-13）
> 日期: 2026-09-13
>
> 实测摘要（夹具探针，`DSCLI_LIVE_CLICK_PROBE=1 go test -v -run TestLiveContinueProbe ./internal/lp/`，headless Chromium + `file://` 夹具 + 临时 profile，不触网；各断言为独立子测试）：
> - 探测器 `present=true, clickable=true`，`label="继续生成"`，坐标 `x=177.8, y=29.5`，`elementFromPoint` 命中按钮；
> - `clickTrustedAt` 后守卫收到 `isTrusted=true` 且可信计数 = 1；
> - JS 合成 `el.click()` 被守卫拒绝：可信计数 = 0、合成计数 = 1、`isTrusted=false`（本设计的直接验证）；
> - 仅含「重新生成」的诱饵页与空页均 `present=false`。
>
> 复审跟进（2026-09-13）已落地：检测层区分 present/clickable 且评估错误上抛；恢复基准优先取本轮 assistant 内容；连续点击派发失败 3 次即 `ErrTruncated`；`continueRecovery` 可注入探针 + 表驱动单测；`webchatWait` 机械拆分（gocyclo 33 → 17）。
>
> 真机验收（会话外，无法主动复现 busy-stop）：待下一次服务器繁忙自然发生后观察；本条为开放项，见 §6。

## 1. 问题（用户报告）

chat.deepseek.com 服务器繁忙时，生成会在中途被站点终止：消息显示「已停止」状态，并在消息行底部渲染一个「继续生成」按钮。此时页面是静止的，且部分输出在结构上"看起来完整"，于是 `webchatWait` 把这段**半截回复**当作本轮最终答复返回：

- `code_dev`（shell 通道）：半截文本没有 `<shell>` 块，`shellblock.Judge` 判定为"最终回复"，本轮直接退出，模型被中断的工作丢失（用户截图正是此现场：模型停在 "Let me read the docs and current code."）；
- 其他角色（expert/review/test）：半截咨询结果被当作完整回答返回；
- 后续 IndexedDB 提取还会因记录不是 `FINISHED` 而做满 3 轮重试（约 92s）再降级到 DOM 文本，纯属浪费。

用户手动点击「继续生成」后生成正常续写。**需求：检测出该按钮并自动点击，让本轮自动续写完成，无需人工干预。**

## 2. 站点事实（bundle 取证，实现必须以这些为准）

1. 按钮文本：`continueGenerationButton:"继续生成"`（英文包 `"Continue"`）。
2. DOM 结构：assistant 消息行 = `[头像, .ds-message 气泡, 底部操作栏]`。气泡根节点带字面类 `ds-message`（`um` 组件：`className: cx(s, "ds-message", ...)`）；「继续生成」组件（`ur`）挂在底部操作栏（`ud`）内，是 `.ds-message` 气泡的**兄弟节点**（同一行容器的直接子元素），不在气泡内部。答案正文节点仍是稳定的 `ds-assistant-message-main-content`。
3. 显示条件 `sc()`（逐字）：

   ```js
   sc=(e,t,n)=>{let r=e.getMessage(t,n);if(!r||r.role!==er.B.MessageRole.ASSISTANT)return!1;let s=ry.h.getMessageUIStatus(r);return!ry.h.isWIP(s)&&s===er.B.MessageStatus.INCOMPLETE&&!r.childIds.length}
   ```

   即：assistant 消息 + 非 WIP + 状态 `INCOMPLETE` + **无子消息（叶子）**。站点自身保证按钮只出现在最新一条被中断的消息上（一旦有新消息成为其子节点，按钮即消失）——自动化侧不需要再做"哪一轮"的推测。

4. **按钮的 onClick 拒绝合成点击**（全 bundle 唯一一处 `isTrusted` 校验，vendors 包中为零）：

   ```js
   onClick:e=>{(e=>{try{let t=e.nativeEvent;if(!t)return!1;return t.isTrusted&&t instanceof Event}catch(e){return!1}})(e)&&u({chatSessionId:t,messageId:n,allowParallelStreams:d})}
   ```

   `el.click()` / `dispatchEvent(new MouseEvent(...))` 产生 `isTrusted=false`，会被静默忽略。**必须用 CDP 输入事件（真实鼠标点击）**：`chromedp.MouseClickXY(x, y)`（button 视口中心坐标）产生的事件经浏览器输入管线合成，`isTrusted=true`。注意：本仓库既有的 JS 合成点击（重发按钮 `jsResendFailedFmt`、上传按钮）不受影响，因为那些按钮没有该校验——不要"统一"成合成点击。

5. 点击后的行为：`continueCompletion` → 消息状态为 `INCOMPLETE`/`localInterrupted` 时走 resume 执行器（`resumeStream`，同一条消息内追加续写），并 `markMessageAsContinuing`。服务端错误码可致 toast：`CURRENT_MESSAGE_IS_NOT_A_LEAF_MESSAGE` →「无法继续生成此消息」；`CURRENT_MESSAGE_IS_LOCKED` →「有消息正在生成，请稍后再试」。
6. 旁证字符串：`messageThinkStopped:"已停止"`、`completionServerBusyToast:"服务器繁忙，请稍后再试"`（忙提示行，仅当消息存在 `incomplete_message` 时渲染）。「已停止」文本**不作为检测信号**（答案正文里可能出现同样字样；按钮才是唯一无歧义的可操作信号）。

## 3. 设计（internal/lp/webchat.go）

### 3.1 检测 JS：`jsContinueGeneration`

- 候选：`button, [role="button"]`；跳过 `b.disabled`、`aria-disabled="true"`；可见性沿用重发匹配器的既有口径（`offsetParent === null` 时以零尺寸 rect 判 display:none）。
- 文本：trim + lowercase 后**精确**匹配 `继续生成` / `continue`（textContent / aria-label / title 三处）；精确匹配天然排除「重新生成 / 重新回答」。
- 归属校验：从按钮向上走，找到"其父容器拥有直接子级 `.ds-message`"的那一层，取该 `.ds-message` 为所属气泡；无气泡（不在消息行内）→ 不点击。命中多个候选时取 DOM 中**最后一个**（最新消息）。
- 定位：若按钮不在视口内先 `scrollIntoView({block:'center'})`；取 `getBoundingClientRect` 中心为视口坐标；用 `document.elementFromPoint(x,y)` 复核命中（含子元素）。
- 返回 `{found:true, clickable, label, x, y}`：**存在但被遮挡**时返回 `clickable:false`（而非 `found:false`）- 把"被遮挡"折叠成"不存在"会让等待层误判本轮健康、在半截文本上走提取。
- **JS 内不做任何点击**（含 `.click()` 字样都不得出现，见 §5 回归断言）。

### 3.2 真实点击：Go 侧

新增小助手（命名建议，保持本文件风格）：

```go
// continueGenerationButton 检测+定位（Evaluate jsContinueGeneration）
// 返回三态：present（按钮存在）/ clickable（存在且未被遮挡），Evaluate 错误上抛。
func continueGenerationButton(ctx context.Context) (continueDetect, error)

// clickTrustedAt 派发真实鼠标事件：MouseMoved + MouseClickXY
func clickTrustedAt(ctx context.Context, x, y float64) error
```

`clickTrustedAt` 的 doc 注释必须写明 isTrusted 校验与 bundle 取证（2026-09-13），防止将来被"优化"回合成点击。

### 3.3 `webchatWait` 接线

新常量（含注释）：

```go
webChatMaxContinues        = 3               // 单次等待内自动点击「继续生成」上限
webChatContinueCooldown    = 8 * time.Second // 两次点击最小间隔（UI 状态翻转时间）
webChatContinueResumeWindow = 45 * time.Second // 点击后等待"续写已恢复"证据的上限
```

恢复状态收拢在 `continueRecovery` 结构中（字段 `clicks` / `lastAt` / `pending` / `failures` / `warned` / `base` / `baseFromAnswer` / `bodyBase` / `deadline`），并带可注入探针 `now/active/detect/clickAt`（nil = 生产实现），供 `continue_recovery_test.go` 的表驱动单测使用。

- `base` **优先取本轮 assistant 内容**（`cleanBodyResponse(lastAnswerText(...))`，含继续会话的基线剥离），取不到才回退 body 文本；`baseFromAnswer` 记录来源，`bodyBase` **始终**记录。恢复证据 = `isGenerationActive` 或**同源文本**变化 - 整页 body 会被点击自身引发的 UI 变化（提示行/按钮状态）翻动，用 body 比较可能提前清除 pending。
- **证据回退**：`baseFromAnswer` 为真但此刻 answer 读取为空（评估失败 / 锚点丢失）时，退回 `body != bodyBase` 比较；两者都不可用才保持 pending（不提前判失败，等恢复窗口）。
- 点击派发失败不计入点击预算，但**连续失败**达 `webChatMaxContinueClickFailures`（3）即返回 `ErrTruncated`，并同时包装最后一次 CDP 错误（`%w` 两次：`errors.Is` 可达 `ErrTruncated` 与原错误），不再以泛化 poll 超时收场。
- **检测错误只 hold、不设上限**（明确取舍）：检测每轮都跑（含健康轮次），把偶发 CDP 抖动升级为 `ErrTruncated` 会让调用方重试，而重试会在生成可能仍在进行时向同一会话再发消息；因此保持 hold（不提取，由轮询预算兜底）。理由写在 `continueRecovery.step` 的检测错误分支注释中。
- resend 重启轮次时重置整个 `continueRecovery`（旧轮次的 pending/deadline 不得影响新发送）；该逻辑抽为带 seam 的 `resendStep`（镜像 `continueRecovery` 的可注入写法），由 `webchat_step_test.go` 覆盖。
- send-ack 阶段抽为带 seam 的 `sendAckStep`，**动作枚举是唯一事实源**（签名不再返回 `acked`）。契约：
  - `webChatProceed` + `err=nil`：已 ack（body 变化 / generation active / textarea cleared）；
  - `webChatAckPending` + `err=nil`：未 ack、无重发，调用方**应**刷新 `lastText`；
  - `webChatNextPoll` + `err=nil`：刚做过过期 textarea 重发，调用方**不**更新 `lastText`；
  - `err != nil`：本轮失败，此时动作恒为 `webChatAbort` 且无进一步含义，**调用方先查 `err` 并中止本轮**，不解读动作。
  动作到循环状态的映射抽为纯函数 `ackLoopEffectFor` 并以表驱动钉住（含 `webChatAbort`/未知动作的穷尽性防护，未知动作返回错误而非静默落到 recovery/stability）。
- 点击派发失败上限的 `ErrTruncated` 同时用两次 `%w` 包装最后一次 CDP 错误（`errors.Is` 可达两者）。

轮询循环内的顺序（在既有 resend 检查与 send-ack 窗口**之后**、稳定性/提取逻辑**之前**）。核心不变式：**只要「继续生成」按钮可见，或点击后的续写尚未确认恢复，就绝不走提取**（此刻的稳定文本是中断前残段，返回即静默截断）：

1. **恢复证据门**（`continuePending` 时）：`isGenerationActive(ctx) || current != continueBase` → 清除 pending（续写已恢复；文本比较用于捕捉极快的完整续写）；仍未恢复且超过 `continueDeadline` → 返回 `ErrTruncated`（包装说明"点击后生成未恢复"）。
2. **检测**（每轮都做，不受冷却限制）：命中「继续生成」（present）进入 3/4 分支；未命中且非 pending → 进入既有稳定性/提取逻辑。**检测评估出错同样 hold**（绝不等价于"无中断"）。
3. **点击**（`present && clickable` 且距 `lastContinueAt` 超过冷却）：`continueClicks >= webChatMaxContinues` → 返回 `ErrTruncated`（包装说明"自动点击 N 次仍未完成"）；未超预算 → `clickTrustedAt`；成功则 `continueClicks++`、更新 `lastContinueAt`、设 `continuePending/continueBase/continueDeadline`、清零 `stableCount/emptyStableCount/lastText`、stderr 打印 `🔄 检测到生成中断（%s），已点击「继续生成」继续（N/3）...`、`continue`；点击失败则打印 warning（不计数、仍更新冷却，下轮重试）。
4. **保持**（present 但不可点击、冷却未过 = 刚点过按钮尚未消失；或未命中但 pending；或检测出错）：`lastText = current`、清零 `stableCount/emptyStableCount` 后 `continue`，禁止提取。清零是为了阻止中断前的稳定计数在 hold 结束后直接命中提取。检测每轮都跑而冷却只约束"点击"，正是为了让"按钮仍可见"这一事实无条件压住提取（即使 pending 被 UI 噪声提前清除，可见按钮也不会漏网）。存在但不可点击时持续 hold（下轮 `scrollIntoView` 重试），由轮询预算兜底。

要求：
- 预算/窗口两类失败一律包装 `ErrTruncated`（既有可重试语义；follow-up 重试会发 `webChatContinueWarning`(Shell)，正是"从断点续写"的正确指令），错误文本口语化说明真实原因（服务器中断续写未完成）。
- 更新 `ErrTruncated` 的 doc 注释：新增"生成被服务器中断且自动续写未成功"一档。
- 更新 `webchatWait` 的 doc 注释：恢复机制清单补「继续生成」自动续写一条（含 isTrusted 事实与"合成点击会被静默忽略"）。
- 不改动 `handle.go` 重试策略（ErrTruncated 已有通路）；不改 `jsResendFailedFmt` 等既有合成点击。

## 4. 改动文件

| 文件 | 改动 |
|---|---|
| `internal/lp/webchat.go` | `jsContinueGeneration` + `continueGenerationButton` + `clickTrustedAt` + `webchatWait` 接线 + 常量/注释更新 |
| `internal/lp/webchat_test.go` | `TestJsContinueGeneration` 字符串回归断言（含坐标字段断言） |
| `internal/lp/continue_recovery_test.go`（新增） | `continueRecovery` 表驱动单测（注入 now/active/detect/clickAt） |
| `internal/lp/continue_probe_live_test.go`（新增） | 门控夹具探针 `TestLiveContinueProbe`（子测试，见 §5） |
| `docs/task-continue-generation.md` | 本文件 |
| `AGENTS.md` | `internal/lp/` 表行补一句自动续写（英文；已于前一提交 `e62577f` 落地，本轮无改动） |
| `internal/lp/webchat_step_test.go`（新增） | `sendAckStep` / `resendStep` 表驱动单测（注入 seams） |

## 5. 测试与验收

1. **字符串回归**（`TestJsContinueGeneration`，风格同 `TestJsResendFailedFmt`）：
   - 必须包含：`继续生成`、`continue`、`b.disabled`、`aria-disabled`、`offsetParent`、`getBoundingClientRect`、`scrollIntoView`、`elementFromPoint`、`ds-message`；
   - **必须不包含**：`.click()`、`dispatchEvent(`（isTrusted 陷阱：任何合成点击都会被静默忽略，这是本次最关键的回归防线）。说明文字写在 Go 注释里，不要让字面量进入 JS 正文。
2. **门控夹具探针**（`DSCLI_LIVE_CLICK_PROBE=1`，默认 skip，不触网、不碰真实登录 profile）：
   - 用 `findChrome()` 启动 headless Chromium（临时 user-data-dir），打开 `file://` 夹具页：完整复刻 §2.2 的行结构（`.ds-message` 气泡 + 其后底部栏按钮）+ 一个"重新生成"诱饵行 + 模拟站点的 isTrusted 守卫（监听器记录 `e.isTrusted` 与可信点击数）；
   - 断言（各为独立子测试）：探测器 `present=true, clickable=true` 且 `label="继续生成"`；返回坐标经 `elementFromPoint` 命中按钮；`clickTrustedAt` 后守卫收到 `isTrusted=true` 且可信计数=1；JS 合成 `el.click()` **不**通过守卫（可信计数保持 0、合成计数=1、`isTrusted=false`，钉死本设计的存在理由）；只有"重新生成"的夹具与空页夹具均 `present=false`。
   - 交付前必须在本机跑一次并把输出摘要写进回报（本机有 `/usr/bin/chromium`）。
3. `go test ./...` 与 `make fmt-check` 全绿；`make gofmt`。
4. **真机验收（会话外，无法主动复现 busy-stop）**：下一次服务器繁忙自然发生后，观察本轮自动续写、不再需要人工点击；可选代理尝试：生成中手动点站点「停止」按钮，若同样出现「继续生成」则也应被自动接管（该路径未实证，仅作线索）。

## 6. 非目标 / 开放风险

- 不处理 `ua`（resume-fail「消息加载失败 / 重试」，localInterrupted 且消息仍 WIP 的网络掉线路径）——本次无证据，另案。
- 不改 `webchatReadLastAssistant` / resume 读取路径；不改 DSML 通道本身（修复在 wait 层，全部调用方自动受益）。
- 风险：点击按坐标派发，若按钮在检测与点击之间位移则可能落空；缓解 = 同轮内检测后立即点击 + 冷却重试（预算）+ 恢复窗口失败后走 ErrTruncated 重试。
- 风险：站点改版（按钮文案 / 结构 / 守卫变化）。字符串与结构均取自现行 bundle 并在文档中留有引用；探针夹具钉住机制，文案漂移需下次取证更新。

## 7. 提交计划（英文，codedev 分支）

1. `fix(lp): auto-resume a server-stopped webchat generation`
2. `test(lp): pin the continue-button detector and trusted click`
3. `docs: task record for continue-generation recovery`
