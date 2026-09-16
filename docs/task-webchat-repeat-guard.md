# task-webchat-repeat-guard: endless repetition 的检测与 CALM DOWN 打断（含两个小项）

> 状态: 已实现（f8eb8b4 / 377edde / 63b1ba2 / fd951d0；后续 review 修复见文末「后续项」）
> 依据: 用户报告（2026-09-16）——WebChat 通道的模型输出会陷入无限重复（endless repetition）：
> 如整段 `</parameter> </invoke>` 反复输出、永不结束。目前 dscli 只能等到 poll 预算耗尽
> （默认 300×2s=600s，或 ctx deadline）后报 `response timeout`，无法自救。
> 用户指示:
> ① 检测 endless repetition，检测到回复 `CALM DOWN! NO PANIC!`（原文: 「检测出来的话很容易制止，
> 只需要回一句：CALM DOWN! NO PANIC! 就好了」）；
> ② 给 shell block 的打印加 fence（工作区已有用户草稿）；
> ③ `scriptN.txt` 回填去掉 script 正文以省 token（工作区已有用户草稿）。
> 阅读前提: `internal/lp/webchat.go` 的 `webchatWait` / `continueRecovery` / `sendAckStep` / `resendStep` /
> `clickTrustedAt` / `jsContinueGeneration`；`internal/lp/handle.go` 的 `handleWebChatShellLoop` /
> `printRound`；`internal/shellblock/runner.go` 的 `FormatBackfill` / `Run`。

## 0. 实现须知

1. （历史语境）本设计起草时工作区已有**用户的两处未提交改动**（`internal/lp/handle.go` 的
   fence 草稿、`internal/shellblock/runner.go` 的去掉 script 草稿）。实现以它们为种子前进、
   未回退：两处草稿分别由 `377edde`（fence 重构）与 `f8eb8b4`（backfill 去 script）收尾。
2. 实现顺序（先让测试恢复绿色，再做核心）：
   1. 小项 ③ backfill 去 script（当前 `internal/shellblock` 测试是红的：用户草稿删了头部与
      正文，测试还在断言旧形态）；
   2. 小项 ② fence 重构 + 测试；
   3. 核心 ① 检测器 + 打断状态机 + 测试。
   每项一笔独立 commit；本文件最后单独一笔 docs 提交。
3. 迭代 `go test ./internal/shellblock/ ./internal/lp/`；交付前 `go test ./...` 与
   `make fmt-check` 全绿（按仓库规范过 gofumpt；用户草稿里的 closure 需要格式化）。
4. 不 `push`、不 `make install`（架构师统一处理）；结束时工作区必须干净。
5. **不改动**既有 `continueRecovery` / `resendStep` / `sendAckStep` 的行为，只在本设计指定处接入
   新步骤；复用它们的机制与风格（可注入 seams、stderr 公告行、`%w` 错误包装、包级 var 供测试调参）。
6. 交付回报必须含：改了什么、跑了哪些测试及结果、commit 列表、工作区干净确认。

## 1. 三个目标与验收

- **① 无限重复检测 + 打断**：在 `webchatWait` 轮询里新增「重复检测 + CALM DOWN 打断」状态机。
  触发后：停止生成（best-effort 点击停止按钮）→ 发送 `CALM DOWN! NO PANIC!` → 模型对该消息
  的回复成为**本轮回复**，会话照常继续（shell loop 按既有判定消费它）。
  验收：检测器纯函数单测（正/负样本）+ 状态机单测（seam 注入覆盖各分支）；全量测试绿。
- **② fence**：shell loop 每轮打印（`printRound` → `outfmt.PrintContent`）中，`<shell>` /
  `</shell>` 独占行前后出现 ```xml / ``` 围栏；首行、末行、双块、已围栏文本都正确。
  验收：`fenceShellBlocks` 单测（见 §5）。**只影响打印**，协议/存储文本绝不加围栏。
- **③ backfill 去 script**：`scriptN.txt` 只含 output 与 exit 状态，不再含 script 正文与 mode 头部。
  验收：`internal/shellblock` 测试更新为新形态 + 反向断言（不含 script/头部）。

## 2. 背景：为什么现在会挂死

- 重复期间文本**持续在变**（越滚越长），`stabilityStep` 的 `stableCount` 永远不会到 3，不会进入
  提取；`cont.step`（「继续生成」/「重新生成」）只在站点**主动停止**时才有按钮可点——生成仍
  active 时无按钮可用；于是只能轮询到 poll 预算耗尽，报错收场（模型侧则持续生成到资源上限）。
- 用户经验：人工在网页端回一句 `CALM DOWN! NO PANIC!` 即可让模型脱离重复、继续工作。这正是
  本设计要自动化的动作。
- 旁证：`jsIsGenerationActive` 的 Signal 2（生成中 textarea 可能 disabled）说明「生成中直接
  发送」未必可行，所以先停再发、发送失败有降级路径（见 §4）。

## 3. 检测器（纯函数，新文件 `internal/lp/repeat.go`）

- **输入**：本轮回复文本，取
  `cleanBodyResponse(extractAfterMessage(current, sentMessage))`——纯字符串操作、无浏览器调用；
  它含本轮 deep-think 与答案的尾部，`cleanBodyResponse` 已剔除已知 UI 行（页脚免责声明、
  「已阅读 N 个网页」、引用行等）。`current` 是 `webchatWait` 每 poll 已有的 body innerText。
- **直觉**：无限重复的定义特征就是「结尾处同一块内容精确重复」。尾锚定即可（重复总是发生在最新
  输出处）。
- **算法**（示意，细节可调但语义必须保持）：

  ```go
  // detectRepeatTail 报告文本结尾处是否存在精确重复的“块”：unitLen 为块长，
  // copies 为连续拷贝数。窗口上限 32KB；p ∈ [1, 1024]。
  func detectRepeatTail(s string) (unitLen, copies int, ok bool) {
      if len(s) > repeatWindowBytes { s = s[len(s)-repeatWindowBytes:] }
      for p := 1; p <= repeatMaxUnit && p*repeatRichMinCopies <= len(s); p++ {
          unit := s[len(s)-p:]
          k := 1
          for off := 2 * p; off <= len(s); off += p {
              if s[len(s)-off:len(s)-off+p] != unit { break }
              k++
          }
          run := k * p
          distinct := distinctNonSpaceRunes(unit)
          // 富块（≥3 种非空白 rune）：2KB / 24 份即可
          if distinct >= 3 && run >= repeatRichMinRun && k >= repeatRichMinCopies { return p, k, true }
          // 贫块（1~2 种，含单字符长串）：要求 8KB / 64 份
          if distinct >= 1 && distinct <= 2 && run >= repeatUniformMinRun && k >= repeatUniformMinCopies { return p, k, true }
      }
      return 0, 0, false
  }
  ```

  常量（包级 const，注释写清调参依据）：
  `repeatWindowBytes=32<<10`、`repeatMaxUnit=1024`、`repeatRichMinRun=2048`、`repeatRichMinCopies=24`、
  `repeatUniformMinRun=8192`、`repeatUniformMinCopies=64`。
- **关键点**：
  - 纯空白 unit 永不触发（distinct==0 两个分支都不满足）。
  - 尾锚定、精确字节比较；多字节 UTF-8 无碍（比较的是同一条流上的等长块，周期性使其自对齐；
    单元可能以 mid-rune 截断也不影响相等比较）。
  - 成本：早退使扫描很便宜（32KB 窗口 + p≤1024，毫秒级），每 poll 调用无压力；实现时对
    `k`/`run` 不达标的 p 尽早 `continue`。
  - 检测器是**保守**的：误报（打断一轮正常生成）代价大于漏报（维持现状），所以阈值宁高勿低。
- **单测**（`repeat_test.go`，表驱动；样本可由 `strings.Repeat` 构造，含用户样本形态）：
  - 正样本：`strings.Repeat("</parameter> </invoke>\n", N)`；无换行变体；`strings.Repeat("</parameter>\n</invoke>\n", N)`；
    CJK `"。"` 长串（贫块分支）；`"，。"` 长串。
  - 负样本：正常中英文长文（含较长段落）；含 `strings.Repeat("a", 3000)` 的代码文本（低于贫块
    run 门槛，且富块 distinct 不满足）；`strings.Repeat("ab", 1200)`（2400B < 8192，贫块不达）；
    60 字符 `---` 分隔行；表格/列表文本；`"\n\n\n…"` 纯空白长串（**必须永不触发**）。
  - 边界：恰好达到阈值与差一字节/一份的输入；重复段不在结尾（尾部还有一句普通话）→ 不触发
    （尾锚定，明确接受）。

## 4. 打断状态机（`repeatRecovery`，建议同放 `repeat.go`，测试 `repeat_recovery_test.go`）

模仿 `continueRecovery`：每 `webchatWait` 调用一个实例（per-call，不跨轮共享），全部依赖走
可注入 seams（nil=生产实现），用假实现做表驱动测试。

### 4.1 集成点（`webchatWait` 主循环）

在 send-ack 阶段之后、`cont.step` 之前调用（此刻 `current`、`sentMessage` 均在手）：

```go
repAct, rerr := rep.step(ctx, repeatInput{current: current, sentMessage: sentMessage})
if rerr != nil {
    return "", rerr
}
switch repAct.kind {
case repeatBusy: // 打断流程进行中（已点击停止、等待生效）
    stableCount, emptyStableCount = 0, 0
    lastText = current
    continue // 跳过 cont.step 与 stabilityStep
case repeatNudged: // CALM DOWN 已发出：把“本轮回复”的锚点切到新消息
    sentMessage = repAct.message
    mdBaseline = repAct.mdBaseline   // 仅刷新 markdown 基线
    cont = continueRecovery{}        // 新发送丢弃旧 continue 状态（同 resendStep）
    stableCount, emptyStableCount, lastText = 0, 0, ""
    continue
case repeatAwaiting: // nudge 已提交、待本状态机的 awaiting 阶段确认
    stableCount, emptyStableCount, lastText = 0, 0, ""
    continue // 只跳过内容与通用 ack 阶段；resendStep 仍每轮在前运行
}
// repeatNone：正常继续（cont.step → stabilityStep）
```

### 4.2 阶段（每 poll 推进一次；状态在实例内）

- **idle**：读检测文本（尾锚定于当前 `sentMessage`）。未触发 → `repeatNone`。触发了：
  - 预算检查：`webChatMaxRepeatInterrupts`（包级 var，默认 1）。已用尽 → 见 **spent**。
  - 公告 `🛑 检测到输出无限重复（endless repetition），正在停止生成…`
  - 若 `isGenerationActive` 且 stop 探针命中且 `clickable` → 记「已点击」、phase=stopping；
    否则（不 active / 探针未命中 / 不可点）→ phase=sending（降级直接发送；探针未命中时记一行日志）。
- **stopping**：每 poll 检查 `isGenerationActive`；不再 active → phase=sending。最多
  `webChatRepeatStopGracePolls=3` 个 poll（约 6s），超时也进入 sending（发送自身还有 ack 窗口兜底）。
  本阶段返回 `repeatBusy`。
- **sending**（一次调用内完成）：snapshot `mdBaseline`（`jsGetAssistantText` → `joinMarkdown`）→
  `webchatSetValue(ctx, webChatCalmDownNudge)` → `jsSendEnter` 派发 → 记录 pre-send marker 计数
  `nudgeSeenBefore = strings.Count(in.current, webChatCalmDownNudge)` → 返回
  `repeatNudged{message: webChatCalmDownNudge, mdBaseline}`；phase=awaiting。**确认由本状态机的
  awaiting 阶段负责**：通用 `sendAckStep`/`ackLoopEffectFor` 在 generation 仍 active 时每个信号都
  被平凡满足，故意不用于 nudge，`webchatWait` 也不复位 `sendAck`/`ackPolls`。body `baseline` 有意
  不刷新（其唯一消费者是通用 ack，而该阶段不再运行）。
  公告 `📨 已回复 \`CALM DOWN! NO PANIC!\`，等待模型继续（第 %d/%d 次）…`
- **awaiting**：计数确认——`strings.Count(current, webChatCalmDownNudge) > nudgeSeenBefore` 即确认、
  转 spent；窗口 `webChatConfirmPolls` 内计数未增长 → `ErrTruncated`（"CALM DOWN nudge was not
  acknowledged within N polls"）。期间只跳过内容与通用 ack 阶段（resendStep 仍每轮在前运行，其重发
  对 nudge 落地是良性的）。
- **spent**：只做二次检测。再触发 → 记日志（`⚠️ 无限重复未被打断…`）+ best-effort 停止
  （仍 active 且按钮可点才点）→ 返回 `fmt.Errorf("%w: endless repetition persisted after the CALM DOWN nudge", ErrTruncated)`
  ——复用既有「截断重试」策略（follow-up 会补一条 continue 警告；初始轮按既有语义重试），不新增挂死路径。

### 4.3 集成注意（务必遵守）

- **busy / nudged 分支不得落入 `stabilityStep`**（防止把重复文本当回复提取）；同时天然跳过了
  `cont.step` ——必须跳过它：停止后的消息可能出现「继续生成」按钮，点了会续上垃圾输出。
  rebase 之后 `cont.step` 恢复正常（`jsContinueGeneration` 取最新消息行，不会命中旧消息）。
- rebase 后 `sentMessage` = nudge 文本：后续 `extractAfterMessage` / `assistantTextAfterMessage` /
  `sendAckStep` 都自动锚定新消息，无需额外改动。
- `webchatSend` 末尾的 IDB 复读无需改动（已核对语义：记录含本轮 user message 且写入时间在发送后
  即通过 guard，取「最后一条 assistant 消息」= nudge 的回复，与 wait 返回值一致；如实现中发现
  不符，按此原则修正并在回报中说明）。
- 公告/错误文案用中文、`%w` 包装，风格对齐既有恢复路径（`🔄…`、`⚠️…`）。

### 4.4 新 JS 探针与 Go wrapper（`webchat.go`）

- `jsStopGeneration`（加进 JS const 块）：镜像 `jsContinueGeneration` 的成熟模式——精确
  text/aria/title 匹配（候选 `['停止','stop','停止生成','stop generating','停止输出']`）、跳过
  `disabled`/`aria-disabled`、可见性（offsetParent===null 时用非零 rect 判定）、取**最后一个**
  命中、`scrollIntoView`、`elementFromPoint` 遮挡检查；返回 `{found, clickable, label, x, y}`。
  与 `jsIsGenerationActive` 的 Signal 1 用同一套匹配依据（那个已在真实站点验证过），但点击前
  收紧为**精确匹配**。
- Go wrapper `stopGenerationButton(ctx)`：结构镜像 `continueGenerationButton`（含非数值坐标的
  守卫），供状态机按下（found 但 !clickable 的处理见 §4.2）。
- 点击一律 `clickTrustedAt`（真实 CDP 输入管线；「继续生成」有 isTrusted 守卫，停止按钮未知——
  trusted 路径对两者都安全）。

### 4.5 状态机单测清单（fake seams）

1. idle 无重复 → `repeatNone`，零副作用；
2. 触发 + active + 可点 → click 恰一次 → stopping → `!active` 后 send 恰一次 →
   `repeatNudged{message==webChatCalmDownNudge}`；
3. 触发 + 不 active → 不 click，直接 send；
4. 触发 + 探针未命中 → 不 click，直接 send；
5. 触发 + found 但 !clickable → 不 click，直接 send；
6. grace 用尽仍 active → 直接 send（不挂死）；
7. spent 后再触发 → 不再 send，返回 `ErrTruncated`（`errors.Is` 可判）；
8. 每实例只允许一次打断（budget=1）且预算为包级 var 可调。

## 5. 小项 A：打印 fence（`internal/lp/handle.go`）

现状是用户草稿：`printRound` 内联 `replaceAll`（两个 `strings.ReplaceAll`）。问题：只命中
两侧都有换行的形态（块在内容首/末行时不生效）；已围栏文本会被二次围栏；closure 未 gofmt。

- 抽成 package 级 `fenceShellBlocks(s string) string`，行级实现：
  1. 按行处理，行内容原样保留；
  2. 维护 `inFence`：行首（trim 后）以 ``` 或 ~~~ 开头 → 切换状态，该行原样输出；
  3. `!inFence` 且 trim 后整行 == `<shell>` → 先输出 ` ```xml ` 行，再输出该行；
  4. `!inFence` 且 trim 后整行 == `</shell>` → 输出该行后追加 ` ``` ` 行；
  5. 其余行原样。
- `printRound` 对 reasoning / content 两处调用；`message`、verdict、feedback 等协议文本保持不变。
- 注意 `outfmt.PrintContent` 内部对 ``` 语言行有 `\n$0\n` 补白（`codeFenceRe`），围栏行不必自带
  空行。
- 测试（`handle_fence_test.go`）：中间块 / 首行即块 / 末行即块 / 两个块 / 已围栏（```xml 内的
  `<shell>`）不动 / 无块不动 / 非整行（句子里的 `<shell>`）不动 / 行首尾留白按 trim 匹配。

## 6. 小项 B：backfill 去 script（`internal/shellblock/runner.go`）

用户草稿已把 `FormatBackfill` 改为只写 `output:` + 输出 + exit 状态。收尾：

1. 签名改为 `FormatBackfill(res *RunResult) string`（`n`/`scriptText` 已无用途；`Run` 的调用点
   与全部测试同步更新）。
2. 重写 doc 注释：描述新形态（仅 output + exit/kill 行）；说明为什么不再回显 script——模型自己
   写过该 script，回合的 follow-up 消息已带关联信息（`output of scriptN.sh (attached as
   scriptN.txt):`），回显只花 token。
3. `scriptFileName` 的注释删掉「FormatBackfill heads the backfill with it」的表述。
4. 更新测试（当前红）：
   - `TestRunScript`（≈L71-80）wants：改为 `["output:", "hello", "exit code: 3"]`，并加反向断言
     （不含 `#!/bin/bash`、不含 `xml`、不含 `bash script1.sh:`）；
   - python 用例（≈L309-316）、attach 用例（≈L366-372）同法更新；
   - `TestFormatBackfillModeLabels` → 重写为 `TestFormatBackfillOmitsScript`（三模式均不回显、
     无 mode 头、首行 `output:`）；
   - `TestFormatBackfillNoOutput` wants 更新。
5. 复查：`rg -n 'xml|script1\.sh:|mode-prefixed' internal/shellblock docs internal/lp` 再扫一遍，
   确认无残留引用（已知仅上述位置 + runner.go 注释）。

## 7. 提交计划与验收

- commits（消息用英文、描述性）：
  1. `refactor(shellblock): stop echoing the script in round backfills`
  2. `feat(lp): fence <shell> blocks in printed webchat rounds`
  3. `feat(lp): detect endless repetition and nudge the model to calm down`
  4. `docs: add the repeat-guard task doc`（本文件）
- 验收：`go test ./...` 绿；`make fmt-check` 绿；`git status` 干净；未 push、未 install。

## 8. 非目标与已知限制

- 只检测**精确重复**：递增计数、近似重复（相似但不相等）不检测；周期 >1024B 的重复不检测。
- 正常但重复性强的输出可能误报——阈值保守且可调（包级 const）；误报的后果可控（一次停表 +
  nudge，会话可继续）。
- stop 按钮选择器以真实站点为准做最终验证（复用了已实测的匹配依据，但收紧为精确匹配）；
  探针未命中时降级为直接发送。
- 不新增 live fixture 探针（可选加分项：本地 fixture pin `jsStopGeneration` 的匹配行为，非必需）。
- 不改动 `continueRecovery` / regenerate / IDB 复读等既有行为。

## 9. 后续项（review 判为 follow-up，本轮不做）

1. **gocyclo 拆分**：`webchatWait`（22）与 `handleWebChatShellLoop`（21）的圈复杂度已
   逼近/超过阈值。建议把 `webchatWait` 里 `repeatAction` 的三分支映射抽成纯函数
   `repeatLoopEffectFor(act repeatAction, current string, ...) (loopEffect, error)`（对齐
   `ackLoopEffectFor` 的既有风格），把状态重置的规则收进一个可单测的映射，主循环只做应用。
   shell loop 同理可按「判定 → 执行 → 回填」拆子步骤。
2. **窗口化对话下的 nudge 确认加固**：`stepAwaiting` 的计数比较假设消息 DOM 不窗口化。站点对长
   对话确实做虚拟化，预算 >1 时旧 nudge 气泡可能滚出渲染窗口、计数不增，即使新 nudge 已落地。该
   情形以可重试的 `ErrTruncated` 收尾（不是挂死），也是默认预算保持 1 的原因之一。加固方向：改为
   锚定「最后一条 user 气泡」或结合 IDB/消息 id 判定；需 live 检查确认虚拟化行为。
3. **jsStopGeneration live fixture 探针**：与 `continue_probe_live_test.go` 同法，用本地
   `file://` fixture pin 停止控件的匹配行为（精确标签、last-match、遮挡检查），并顺带确认真实
   DOM 位置（当前刻意不做 `.ds-message` ownership 检查，见 `webchat.go` 注释）。门控在
   `DSCLI_LIVE_*` 环境变量下，默认跳过。
