# DSML 包架构设计：internal/dsml

> 注意：本文件为避免 DSML 会话中的 XML 截断问题，不含任何 XML 字面量示例。
> 所有格式示例以现有代码为准（internal/toolcall/dsml_doc.go 的
> dsmlGeneratedDocEntry 生成的就是权威示例，formatDSMLToolResult 生成
> tool_result 块）。需要查看真实示例时请直接读取这些代码文件。

## 1. 背景与目标

当前 DSML 逻辑（工具调用解析、判定、执行、提示词文档生成）全部堆在
`internal/toolcall/dsml.go`（约 1000 行）+ `internal/toolcall/dsml_doc.go`。
本轮目标：

1. 建立 `internal/dsml` 包，把**所有** DSML 工具调用解析、判定、文档生成
   逻辑迁入其中，对外暴露统一入口 `ParseDSMLMessage`。
2. 对"工具调用格式是否严格遵守系统提示词要求"做显式判定（`message.OK`），
   违规但可解析时**照常执行**，仅在结果 warning 中警示并要求下次严格；
   解析失败但明显在发工具调用时，keep 会话并提示**重发**（不再静默退出）。
3. 系统提示词中的格式要求（BuildDSMLToolDoc）升级为严格措辞。

## 2. 包依赖方向

```
internal/dsml
  ├─ imports: internal/prompt（Message/ToolCall/ToolContent/DSMLToolDoc 类型）
  │           internal/toolcall（ToolArgs/ToolDef/注册表/roleToolsSpec——执行内核）
  │           internal/context, internal/outfmt, clog
  └─ 无反向依赖：toolcall 其余文件不引用 dsml 符号（已勘察确认），prompt 不依赖 dsml
```

`ToolContent`、`ToolDef`、`ToolArgs`、`roleToolsSpec`、工具注册表等执行内核
**留在 toolcall 包**（不属于 DSML 解析判定），dsml 通过导入使用它们。

## 3. 对外 API（internal/dsml）

### 3.1 主入口（用户指定签名）

```go
func ParseDSMLMessage(reasoning string, content string) prompt.Message
```

语义（按用户拍板的分层）：

| 情形 | 返回 |
|------|------|
| 解析出工具调用 `len(ToolCalls) > 0` 且无违规 | `OK=true`，Content/ReasoningContent 中已剥离工具调用块 |
| 解析出工具调用且**有违规**（见 §4 清单） | `OK=false`，ToolCalls 照常返回（**照常执行**），Content 保留原始文本（不剥离，供调用方兜底判断） |
| 未解析出调用 `len(ToolCalls) == 0` | `OK=false`，Content 原样；是否"疑似工具调用"由调用方用 `SuspectedDSMLToolCalls` 判定 |
| 解析失败（截断/unclosed > 0） | `len(ToolCalls) == 0`；由 `SuspectedDSMLToolCalls` 判定为疑似 → 调用方 keep + 重发警示 |

解析源规则：**content 优先**；content 中无工具调用时降级解析 `reasoning`
（DeepSeek 可能在 thinking 中预写调用）。content 有调用时 reasoning 不作
执行源，原样保留（打印展示用），不参与 OK 判定。

`OK` 复用 `prompt.Message.OK`（`json:"-"` 内存字段）。注意双语义：history.go
的 JudgeHistory/CleanupReverse 用 OK 表示"历史配对完整性"（DB 加载后重算），
ParseDSMLMessage 的 OK 表示"DSML 格式严格合规"，两条路径互不相交。在
`prompt/message.go` 的 OK 字段注释中写明双语义。

### 3.2 其他导出（迁移保留原名，包名已表意）

```go
func HasDSMLToolCalls(text string) bool            // 存在 named invoke 开标签（疑似判定基础）
func IsPureDSMLToolCalls(text string) bool         // 全部内容=工具调用
func IsDSMLToolCallEnd(text string) bool           // 以 tool_calls 闭标签类结尾
func IsDSMLToolCallCut(text string) bool           // 以截断的闭标签结尾
func IsDSMLToolCallReply(text string) bool         // 执行 gate：完整工具调用意图
func ParseDSMLToolCalls(text string) ([]DSMLCall, error)  // 底层解析（保留）
func StripDSMLToolCalls(text string) string        // 剥离工具调用块
func ExecuteDSMLToolCalls(ctx context.Context, calls []DSMLCall) []string
func BuildDSMLToolDoc(ctx context.Context, role string) prompt.DSMLToolDoc
```

新增：

```go
// SuspectedDSMLToolCalls 报告文本"明显在发工具调用但解析失败/为空"：
// 存在 named invoke 开标签（HasDSMLToolCalls），但 ParseDSMLToolCalls
// 失败或返回 0 个调用。调用方据此决定 keep + 重发警示。
func SuspectedDSMLToolCalls(text string) bool

// InjectStrictWarning 把固定模板 warning 注入每个 tool_result 块的
// warning 字段（outputs 是 ExecuteDSMLToolCalls 的返回，每个元素形如
// tool_result 块，内容为 JSON: result/warning/error，omitzero）。
// 若已有 warning 则追加（换行分隔）。无 result/error/warning 的空块跳过。
func InjectStrictWarning(outputs []string) []string

// 固定模板（英文，与提示词一致；模型侧上下文是英文提示词）：
const StrictWarning = "WARNING: your tool-call markup did not strictly " +
    "follow the required format (e.g. extra attribute such as justification, " +
    "missing string attribute, unclosed or stray tags); the call was " +
    "accepted but you MUST strictly follow the exact format in the tool " +
    "schema for every future call."
// 重发模板（疑似失败场景，发给模型的反馈正文）：
const ReissueWarning = "WARNING: your reply appears to contain a DSML tool " +
    "call but it could not be parsed. Please re-send the tool call(s) " +
    "strictly following the required format: one or more invoke blocks " +
    "(each with a name attribute and parameter children carrying the " +
    "string attribute) wrapped in a tool_calls block, exactly as shown in " +
    "the tool schema section of your instructions. Do not include extra " +
    "attributes such as justification."
```

## 4. 违规判定清单（message.OK=false 的构成）

原则：**凡是解析器为让调用跑起来而容忍/修复的偏离严格格式之处，均记违规**；
成功解析出调用即照常执行，违规只影响 OK 与 warning，不影响执行。判定为
bool 汇总即可（无需精确违规项列表——用户拍板：固定模板 warning，具体违规
不提，避免模型越改越乱）。

实现方式：在解析路径各容错点置位一个 `violated bool`（或计数），最后汇总。

| # | 违规点 | 现有代码位置 |
|---|--------|--------------|
| 1 | 参数含 `justification` | `normalizeDSMLInvoke` 静默丢弃处（continue）→ 记违规 |
| 2 | normalize 改变了文本（全角→半角、实体解码、零宽清除、junk 清理） | `normalizeDSMLText` 前后对比 `normalized != original` → 违规 |
| 3 | 多余 invoke 闭标签（stray close） | `dsmlBlockRanges` 返回的 `strays`（现有代码忽略）→ 记违规 |
| 4 | 缺 invoke 闭标签隐式关闭 | `dsml_missinginvoke_test.go` 覆盖的容错分支 → 记违规 |
| 5 | wrapper typo：`/_calls`、`_calls`、截断闭标签、缺 tool_calls 闭标签 | `dsmlToolCallsCloseEndRe`/`dsmlToolCallsCloseCutRe` 容错路径 → 记违规 |
| 6 | 裸 invoke 无 tool_calls wrapper | `dsml_bareinvoke_test.go` 容错路径 → 记违规 |
| 7 | parameter 缺 string 属性 | `dsmlParamRe` 的 string 组为空时 → 记违规 |
| 8 | 参数值中内嵌 DSML 被掩码/误解（嵌套 block 处理） | `ParseDSMLToolCalls` 嵌套掩码路径 → 记违规（低优先，实现者按代码实际路径定） |

注意：**重复 parameter（数组语义）不算违规**（DeepSeek 对 []string 参数
发两个同名 parameter 是 schema 的正常表达）。

## 5. 提示词严格化（BuildDSMLToolDoc）

在现有 Intro（工具注册段）追加严格格式声明（在 "You have access to a set
of tools..." 段落附近，措辞由实现者润色，必须覆盖以下语义；XML 示例不用
写进本文件——照着 dsmlGeneratedDocEntry 生成的示例措辞即可）：

- 每次调用必须输出完整 tool_calls 包裹的 invoke 块（带 name 属性），
  参数必须用 parameter 标签并携带 string 属性（true/false 按类型）；
- **不允许**添加任何额外属性（如 justification）或示例之外的参数；
- 所有标签必须闭合（invoke 与 tool_calls 的闭标签都必须输出）；
- 一次回复可含多个 invoke（独立调用），依赖调用必须等上一轮结果；
- 结尾 Schemas 段保留并强化现有 "You MUST strictly follow the above
  defined tool name and parameter schemas to invoke tool calls." 措辞。

模型输出应当与 dsmlGeneratedDocEntry 生成的示例**逐字节同构**（示例就是
格式规范的唯一权威）。核对现有示例已体现全部参数带 string 属性（现状应已
如此，验证即可）。

## 6. 循环改造（internal/lp/handle.go handleWebChatToolLoop）

替换现 gate + parse + 执行三段（参考现有约 513-560 行），新流程（伪码，
XML 均以描述代替，避免协议截断）：

```
for round := 1; round <= handleWebChatMaxDSMLRounds; round++ {
    msg := dsml.ParseDSMLMessage(lastReasoning, message)
    switch {
    case len(msg.ToolCalls) > 0 && dsml.IsDSMLToolCallReply(message):
        // 执行路径（用户点 4：OK=false 也执行！）
        calls, _ := dsml.ParseDSMLToolCalls(message)   // 拿 DSMLCall 执行
        outputs := dsml.ExecuteDSMLToolCalls(ctx, calls)
        if len(outputs) == 0 { ...现有 cleanExit 分支（非可执行调用）... }
        feedback := buildWebChatFeedback(outputs)
        if !msg.OK {                                   // 用户点 2/4：warning 警示
            feedback = dsml.InjectStrictWarning(feedback)
        }
        send(feedback, keep=convURL) → 下一轮
    case len(msg.ToolCalls) > 0 && !dsml.IsDSMLToolCallReply(message):
        // 引用示例/长回复中引用了调用（非执行意图）→ 不执行，按普通回复退出
        return cleanExit()
    case len(msg.ToolCalls) == 0 && dsml.SuspectedDSMLToolCalls(message):
        // 用户点 1/4：疑似工具调用但解析失败 → keep + 重发警示（不退出）
        send(dsml.ReissueWarning, keep=convURL) → 下一轮
    default:
        // 普通回复（含纯散文、无调用）→ 退出
        return cleanExit()
    }
}
```

要点：
- `lastReasoning` 变量：每轮保存 `res.Reasoning` 传给下一轮
  ParseDSMLMessage（首次进入循环用 first.Reasoning）。
- 重发/违规反馈同样走 `handleWebChatSend`（keep 同一会话 URL），
  受 `handleWebChatMaxDSMLRounds` 上限保护（现有防死循环保险不变）。
- 打印（printRound）仍打印远程原始回复（含 DSML 块），不改变。
- cleanExit 的 strip 语义不变（角色会话剥离 DSML 露出散文）。
- stderr 的 "请求执行 N 个工具调用" 等提示保持；可加一条中文 stderr
  提示 "工具调用格式不够严格，已执行并告警"（用户侧）。

## 7. 迁移清单

| 动作 | 文件 |
|------|------|
| 移动+包名改 `dsml` | `internal/toolcall/dsml.go` → `internal/dsml/dsml.go` |
| 移动+包名改 `dsml` | `internal/toolcall/dsml_doc.go` → `internal/dsml/doc.go` |
| 移动+包名改 `dsml` | DSML 相关测试：dsml_test.go、dsml_doc_test.go、dsml_bareinvoke_test.go、dsml_missinginvoke_test.go、dsml_realpayload_test.go、dsml_strayclose_test.go、dsml_strayclose_chop_test.go（internal/toolcall/ 下）+ dsml_exec_test.go（internal/toolcall/ask/ 下） |
| 更新调用点 | `internal/lp/handle.go`（dsml.* 前缀）、`internal/toolcall/ask/{code_dev,code_review,quality_assurance}.go`（BuildDSMLToolDoc → dsml.BuildDSMLToolDoc 或 dsml 包下同名）、`webchat_cmd.go` 注释 |
| 注释更新 | `prompt/message.go` OK 字段双语义说明 |
| 清理 | toolcall 包中 dsml.go/dsml_doc.go 迁移后删除（残留空壳不留） |

实现者以实际文件为准，凡 import toolcall 且测试内容为 DSML 解析/判定的，
迁入 internal/dsml/。注意 `internal/toolcall/ask/dsml_exec_test.go` 可能在包
`ask` 下，迁入后需要跟随新 import 路径。

## 8. 测试要求

1. **迁移后原测试全绿**（语义不变的部分：解析、剥离、doc 生成、执行计划）。
2. **新增 ParseDSMLMessage 单测**（internal/dsml/）：
   - 严格合规调用 → OK=true，ToolCalls>0，Content 已剥离；
   - 含 justification → OK=false，ToolCalls>0，Content 保留原文；
   - 多余 invoke 闭标签 → OK=false，ToolCalls>0；
   - 缺 string 属性 → OK=false；
   - 截断调用 → ToolCalls==0，SuspectedDSMLToolCalls==true；
   - 纯散文无调用 → ToolCalls==0，Suspected==false；
   - content 无调用、reasoning 有合规调用 → ToolCalls 来自 reasoning；
   - content 有调用、reasoning 也有 → 以 content 为准。
3. **InjectStrictWarning 单测**：结果 JSON warning 字段注入、已有 warning 追加、
   空块跳过、无输出时返回空。
4. **lp 循环集成测试**（handle_test.go 跟进现有桩测试，handleWebChatExecDSML
   已是可替换 hook）：
   - 违规调用（justification）→ 执行 + 下一轮 feedback 含 StrictWarning；
   - 疑似失败（截断）→ 发送 ReissueWarning 且循环继续；
   - 持续截断超过上限 → 触发上限退出（现有 1024 保险验证）；
   - 引用示例长回复 → 不执行直接退出（回归保护）。
5. **提示词严格化后**：dsml_doc_test.go 断言新严格措辞存在
   （如 "MUST strictly" 强化段、禁止 justification 的措辞）。

## 9. 验收标准

- [ ] `go build ./...` 通过
- [ ] `go vet ./...` 通过
- [ ] `go test ./...` 全绿（含新增用例）
- [ ] `make fmt-check` 通过（gofumpt + goimports）
- [ ] toolcall 包不再含任何 DSML 解析判定代码（仅执行内核 ToolArgs/ToolDef/registry 留守）
- [ ] handleWebChatToolLoop 使用 dsml.ParseDSMLMessage 作为唯一判定入口
- [ ] 提交为英文 commit message，工作树干净
