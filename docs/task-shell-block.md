# task-shell-block: `<shell>` 块工具（网页模型工具调用）

> 状态: 已实现（本仓库直接开发，见 §11）；真机冒烟通过（§7），待 code_dev 真任务验收
> 依据: `scripts.txt`（64 轮手工实验完整对话，12127 行）+ `codereview.md`（现行工具文档）
> 日期: 2026-09-13
> 修订: 开工前核对完成；用户确认 attach 回填、dscli-shell 不构成规范；档案补证（围栏引用 / 拦截 / 分级）落库为 §10；2026-09-14 决策：移除 `--shell` 开关，`dscli webchat` 恒定启用 `<shell>` 块通道（webchat 的工具只保证 shell 一个）

## 1. 目标与背景

为网页模型（webchat 通道）提供工具调用能力，替换 DSML：

1. 用 `<shell>` 块换 DSML，规避 DSML 截断 / 徽章渲染问题；
2. 工具精简为一个 `shell`，提高工具调用稳定性；
3. 目标是"能用、稳定"，不追求强。

使用者：`code_dev`（开发助理，自动启用）；`dscli webchat`（恒定启用；2026-09-14 起无开关——webchat 的工具通道只保证 `<shell>` 一个）。
code_review 走无工具的检视流（`codereview.md`），不在本任务范围。

## 2. 协议契约（模型侧）

模型在一轮回复中最多发一个块：

```xml
<shell>
<script>
#!/bin/bash
...
</script>
<summary>short intent</summary>
<timeout>120</timeout>
</shell>
```

四条格式规则（逐字保留，告警文案复用）：

1. `<shell>` must be the leading line of the shell block and it fills the whole line,
2. `</shell>` must be the ending line of the shell block and it fills the whole line,
3. `<script>` must be the leading line of the script block and it fills the whole line,
4. `</script>` must be the ending line of the script block and it fills the whole line.

字段约定（来自 codereview.md）：

- `<summary>`: <= 40 chars，意图声明，人可见审计行；
- `<timeout>`: 默认 120s，跑测试 / 构建时可上调（如 1200）；本实现上限 1800s。

另需注入 "Iterate inside the script, not across rounds" 段：单轮自包含、`set -euo pipefail`、
exit status 是信号而不是装饰、把下一步决策所需的信息一次打印完。

## 3. 已验证事实（scripts.txt 取证）

- 原始输出全部为**纯净独占行**；`#` 前缀 0 次（`#` 只出现在本地 bash 化落档 `scriptN.sh`，那是手工期把整条消息变可执行文件的产物，不进解析器）。
- 对话内共 35 次执行回填（rounds 30-64）。其中 `<shell>` 形态 30 次（rounds 30-59），DSML 形态 5 次（rounds 60-64，用户主动邀请的实验，非模型漂移）。
- `<shell>` 块出场 33 次：30 次执行、2 次打回、1 次在讨论轮被人工跳过（用户直接文字回应，未执行）。2 次打回（rounds 43/53）失败形状相同：闭合行被频道渲染成全角竖线 DSML 徽章残形（`dsml.go` 的 `dsmlTagJunkRe` 修的就是这族）；处置 = 原样重发四规则契约，模型下一轮修复继续执行（output43 / output53）。
- 全部块自带 `<summary>` + `<timeout>`；timeout 实测 100 / 120 / 300。
- "一块一轮" 0 例外；无块轮次最短 113 字符 → "<=100 字才告警" 无误伤余量。
- `<script.sh>` 字面量 / 围栏 / heredoc 均无害（探针 61/62/63）；唯一致命族 = 边界越过 `</script>` 吞散文（不可复现）。行级精确匹配 + 失败即打回可消灭此族。
- 结果回填 = 文件附件（当年通道 = chat.deepseek.com file API；dscli 侧复用现有附件通道）。

## 4. 判定 / 提取 / 告警（internal/shellblock）

判定表（对每轮模型回复；判定文本 = 清洗后的 content，content 为空时才用 reasoning 兜底）：

| 条件 | 动作 |
|---|---|
| 无 `<shell>` 行，文本 <= 100 字 | 格式告警（input 回填，同会话继续） |
| 无 `<shell>` 行，文本 > 100 字 | 视为最终回复，退出循环 |
| 有 `<shell>` 行，四标签各恰好一次、成行、按序，body 非空 | 提取并执行 |
| 其余一切（缺 / 重复 / 乱序 / 徽章残形 / DSML 形状 / 空 body / 多个块） | 格式告警（注明 issue） |

引用区域先剔除，再套用判定表：标签行落在行锚定围栏（反引号或波浪线，≥3 个，CommonMark 行首开合）或行内代码跨度之内 → 不算块（引用示例，永不执行）。复用 `dsml.CodeRanges`（自 `dsmlCodeRanges` 导出）；行锚定规则不要重造：scripts.txt L2963 / L7267 记录，模型读 AGENTS.md 会引用示例，这是"最阴的坑"。

首轮入口（HandleWebChat 路由）与循环内同用此表：仅"最终回复"不进循环，其余（执行 / 各类告警）进入 `handleWebChatShellLoop`。

无块回复的补充：含 DSML 调用形状（`<invoke` / 全角竖线序列）→ 一律格式告警（不静默）；仅提及 `<tool_calls` 字样且足够长的报告仍按最终回复处理。

提取器（不做任何"聪明"处理）：

- 只认整行严格相等：`<script>` 起、`</script>` 止，之间逐行原样（围栏、heredoc、行内标签字面量一律不改写）；
- `<summary>` / `<timeout>` 在 `</script>` 之后、`</shell>` 之前宽松解析（取首个匹配；缺失用默认）；
- "有 `<shell>` 行" = 存在严格成行的 `<shell>` 且不在引用区域内；徽章残形 / 缩进变体不算块，但要在告警 issue 里点名；
- 引用剔除只作用于块边界判定；`<script>` 与 `</script>` 之间的正文逐行原样提取（正文内的围栏 / heredoc / 字面量不改写、不参与剔除）；
- 标签取"首个"成链（首个 `<shell>` → 其后首个 `<script>` → 首个 `</script>` → 首个 `</shell>`）；块体内的 `<shell>` / `<script>` 行是数据（不参与结构判定）；首个整行 `</script>` 结束正文——正文内不得出现整行 `</script>`（协议硬边界，行为已由测试钉住）；`</shell>` 之后再出现 `<shell>` 开头 → 判"多个块"告警；其余块外多余标签行宽容（防止把合法块误判为重复）。

告警文案（英文，按 input 回填；结构 = 问题一行 + 四规则 + 示例）：

- 默认模板："Your reply contains a malformed `<shell>` block (<issue>). ..." + rules + example；
- DSML 形状（检测 `<tool_calls` / `<invoke` / 全角竖线序列）：追加 "do not use the DSML shape; this channel badges/mangles it - use a `<shell>` block instead."；
- 无块短回复："Your reply neither contains a `<shell>` block nor reads as a final report. ..."；
- 拦截拒收（脚本命中破坏性命令）："Your `<shell>` block was NOT executed: it contains a blocked destructive command pattern ..."（点名命令族 + 要求改写后重发）。

循环控制（failsafe）：

- 连续告警 3 次 → 中止报错（格式告警与拦截拒收均计数）；总轮次上限 1024；告警轮计入轮次。

## 5. 执行与回填

- 落点：项目根 `scriptN.sh` / `scriptN.txt`；N = 现存最大编号 + 1（当前从 65 起）。codedev 的 .gitignore 已覆盖 `script*.sh` / `script*.txt`；其他目标仓库会以未跟踪文件出现——首版与实验一致，不自动改目标仓库配置。
- 执行：提取脚本原样写 `scriptN.sh`；`bash scriptN.sh`，cwd = 项目根；超时（默认 120s，上限 1800s）到点杀进程组。
- 执行前拦截（安全边界，不许降级）：整段脚本先过 `dsml.BlockedCmdRe`（自 `dsmlBlockedCmdRe` 导出，名单一字不动）；命中 → 本轮不写文件、不执行，回填拦截拒收警告（要求改写重发；计入连续告警）。依据 scripts.txt L2965：这一层不能省，且要比 DSML 更严——实现取 fail-closed（`shellblock.Run` 内置同一拦截作兜底）。
- 回填 `scriptN.txt`（dscli-shell 是验证期 312 字节临时脚本，不构成规范；保留 脚本段 + output 段 + exit code 三段结构。定稿微调：头部写真实命令 `bash scriptN.sh:`、尾部补 `exit code: N`）：

```
bash scriptN.sh:
```xml
<script body verbatim>
```
output:
<stdout+stderr merged>
exit code: N
```

- 发送：文件附件（attach）+ 一行极短文字（含文件名），走现有 webchat 发送流程；逐轮上传，附件用绝对路径（用户已确认 attach 即可）。
- 合并输出超过 ~256KB 时截断并注明（可调）。

## 6. 集成点

| 模块 | 改动 |
|---|---|
| `internal/shellblock/`（新增） | judge（含引用区域剔除）/ extract / warning / runner；表驱动测试 + testdata |
| `internal/dsml/` | 导出 `CodeRanges`（原 `dsmlCodeRanges`）与 `BlockedCmdRe`（原 `dsmlBlockedCmdRe`）供 shellblock 复用；行为不变，补 doc 注释 |
| `internal/prompt/` | `RenderPromptForRoleWithShellTool` 注入（新字段 `ShellToolDoc`，优先于 DSML 段）；文档正文由 `shellblock.BuildToolDoc` 生成（源：codereview.md L45-86 + 被拦截命令族 + 收尾说明） |
| `internal/prompt/dev.md` | dev 角色注入切换（`{{if .ShellToolDoc}}` 优先分支） |
| `internal/lp/` | `WebChatOptions.ShellTool bool`；入口路由（判定非"最终"才进循环）；`handleWebChatShellLoop`（镜像 handleWebChatToolLoop 骨架：printRound / Printed / 上限 1024 / 连续告警 3 / 本地执行 stderr 警告 / exec mock 变量）；`HandleWebChatResume` 分支；截断续传提示的 shell 变体（指向 `<shell>` 块）；逐轮 attach 回填 |
| `internal/toolcall/ask/code_dev.go` | 接线（自动启用 shell 模式；跳过 DSML 工具集检查）；`code_dev.md` 契约描述同步 |
| `webchat_cmd.go` | 无开关：恒启用 `<shell>` 块通道（2026-09-14 决策移除 `--shell`；原为手动测试开关） |
| `docs/` | 本文件 |
| `AGENTS.md` | 架构表补 `internal/shellblock` 一行；`internal/dsml` 行补共享原语说明；补一句 dev/`code_dev` 走 `<shell>` 块通道（其余角色仍 DSML） |

默认方案 A（2026-09-14 修订）：code_dev 自动带；`dscli webchat` 恒定带（无开关）；其它入口（ask 桥非 dev 角色）行为不变。

## 7. 测试与验收

- 包测试：判定表全分支；提取边界（徽章残形 / 堆叠开标签 / 第二整块 / 体内字面标签行 / 乱序 / 围栏 / heredoc / `<script.sh>` 字面量）；引用区域剔除（围栏内块不执行 / 围栏示例后跟真块 / 行内代码 / 未闭合围栏吞块）；拦截拒收（不执行、不落文件；`Run` 内置兜底）；告警文案（含 DSML 形状追加句）；runner 超时（含后台子进程持管道）/ 退出码 / 合并输出 / 截断。
- lp 集成：mock transport 覆盖 入口路由 → 执行 → 附件回填 → 告警（格式 / 拦截）→ 上限 → resume；截断续传变体。
- 真机：`make install`（v0.9.3-25-gf59716c）后 `dscli webchat --shell --role dev` 冒烟通过（2026-09-13 14:13：只读 git 任务 → script65.sh 执行 → script65.txt 附件回填 → 纯散文终报；34s；会话 keep:b9dd7edb）；待重启会话后由 code_dev 做一次真任务验收。
- `go test ./...` + `make fmt-check` 全绿。

## 8. 提交计划（英文，codedev 分支）

1. `feat(shellblock): add <shell> block judge/extract/warning/runner`（包 + 单测）
2. `feat(lp): add <shell> block tool loop for webchat`（lp + prompt + toolcall 接线 + --shell）
3. `docs: task record for shell-block tool`（本文件 + AGENTS 行 + .gitignore 补充 + 未提交的审计修正）

## 9. 微调与默认（已锁定）

- 回填通道 = attach 文件附件（用户确认）；头部 `bash scriptN.sh:`、尾部 `exit code: N`、围栏保留（见 §5）。
- dscli-shell 不构成规范（用户确认）：它是验证期临时脚本，实现取 §5 定稿格式。
- `.gitignore` 追加 Emacs 残件模式（`\#*#`、`.#*`），清掉 git status 噪音（只加模式，不删文件）。

## 10. 开工前修订（档案补证，实现必须遵守）

- 引用区域剔除（围栏 / 行内代码）：scripts.txt L2963 / L7267；复用 `dsml.CodeRanges`，不重造行锚定规则。
- 破坏性命令拦截不降级：scripts.txt L2965；单一名单（`dsml.BlockedCmdRe` 导出复用），命中即拒（fail-closed）。
- 失败分级三档映射（scripts.txt L2961）：可解析但有瑕疵 → 照常执行（缺字段用默认）；想发但没解析出来（含 `</shell>` 错位 / 截断）→ 打回重发（原样重发四规则契约）；畸形（标签拼错 / 徽章残形 / DSML 形）→ 绝不执行、打回。§4 判定表即该分级的落地。
- 不落库副作用（scripts.txt L2967）：本通道不产生 tool 消息（回填走附件），天然不触碰 `CleanupReverse` 配对；实现勿引入任何 toolcall 消息写入。

## 11. 实现须知（dev 会话）

- 实施通道：code_dev 不可用（本任务即其修复对象）→ 由仓库会话直接实施（09-12 先例）；检视走已修复的 code_review（附件流）。以下为会话内工作约定。

- 先读：`AGENTS.md` + 本文件全部；工具文档取 `codereview.md` L45-86（gitignored 临时文件；内容写入嵌入资产，该文件本身不提交）；判定纪律参考 `internal/dsml/`；循环骨架参考 `internal/lp/handle.go`（handleWebChatToolLoop）。
- 工作区已有未提交改动（`.gitignore`、`AGENTS.md` 审计修正、本文件）——保留，纳入 §8 提交计划。
- 会话内完成 build + 单元 / mock 集成测试，`go test ./...` 与 `make fmt-check` 全绿；测试隔离 ambient state（见 AGENTS.md）。真机两段（§7 末两条：webchat 冒烟 + code_dev 真任务）在 dev 会话之外进行，勿在会话内跑实时网页会话。
- 三笔提交（§8），英文 message；完成后工作区必须干净；回报实现内容 / 测试结果 / 提交哈希 / conversation_id。
