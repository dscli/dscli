# 任务：DSML 包重组（internal/dsml）

> 本文件不含 XML 字面量（避免 DSML 会话截断）。真实格式示例参考现有代码：
> internal/toolcall/dsml_doc.go（dsmlGeneratedDocEntry）与 dsml.go（formatDSMLToolResult）。

## 目标

1. 新建 internal/dsml 包，把 internal/toolcall/dsml.go（约 1000 行）与
   dsml_doc.go 全部逻辑迁入（包名 dsml）。toolcall 其余文件（ToolArgs、
   ToolDef、ToolContent、registry、roleToolsSpec 等执行内核）原地不动，
   dsml 导入 toolcall 使用（无环：已确认 toolcall 非 dsml 文件不引用其符号）。
2. 新增统一入口 ParseDSMLMessage(reasoning, content) prompt.Message。
3. 提示词严格化：BuildDSMLToolDoc 追加严格格式声明。
4. lp/handle.go 的 handleWebChatToolLoop 改用新入口。

## ParseDSMLMessage 语义

| 情形 | 返回 |
|------|------|
| 解析出调用且无违规 | OK=true，Content/ReasoningContent 已剥离调用块 |
| 解析出调用但**有违规** | OK=false，ToolCalls 照常返回（**照常执行**），Content 保留原文不剥离 |
| 未解析出调用 | OK=false，Content 原样 |
| 解析失败（unclosed>0 截断） | len(ToolCalls)==0，由 SuspectedDSMLToolCalls 判定 |

- 解析源：content 优先；content 无调用时降级解析 reasoning。content 有调用
  时 reasoning 不参与判定、原样保留。
- OK 复用 prompt.Message.OK（json:"-"）。注意双语义：history.go
  JudgeHistory/CleanupReverse 用它表示历史配对完整性，互不相交；在
  prompt/message.go 的 OK 字段注释写明双语义。

## 违规清单（OK=false 构成；bool 汇总即可，不需精确列表）

| # | 违规点 | 代码位置 |
|---|--------|----------|
| 1 | 参数含 justification | normalizeDSMLInvoke 静默丢弃处 |
| 2 | normalizeDSMLText 改变了文本（normalized != original） | normalizeDSMLText 入口对比 |
| 3 | 多余 invoke 闭标签（stray close） | dsmlBlockRanges 返回的 strays（现有忽略）|
| 4 | 缺 invoke 闭标签隐式关闭 | missinginvoke 容错分支 |
| 5 | wrapper typo：_calls 类、截断闭标签、缺 tool_calls 闭标签 | dsmlToolCallsCloseEndRe/CloseCutRe 路径 |
| 6 | 裸 invoke 无 tool_calls wrapper | bareinvoke 容错路径 |
| 7 | parameter 缺 string 属性 | dsmlParamRe string 组为空 |
| 8 | 参数值内嵌 DSML 嵌套掩码 | ParseDSMLToolCalls 嵌套掩码路径（低优先）|

重复 parameter（数组语义）**不算违规**。

## 新增导出

```go
func SuspectedDSMLToolCalls(text string) bool // HasDSMLToolCalls 且 ParseDSMLToolCalls 失败或 0 调用
func InjectStrictWarning(outputs []string) []string // 固定模板注入每个 tool_result 的 warning 字段
const StrictWarning = "WARNING: your tool-call markup did not strictly follow the required format (e.g. extra attribute such as justification, missing string attribute, unclosed or stray tags); the call was accepted but you MUST strictly follow the exact format in the tool schema for every future call."
const ReissueWarning = "WARNING: your reply appears to contain a DSML tool call but it could not be parsed. Please re-send the tool call(s) strictly following the required format: invoke blocks (name attribute + parameter children with string attribute) wrapped in a tool_calls block, exactly as shown in the tool schema section of your instructions. Do not include extra attributes such as justification."
```

其他函数迁移保留原名：HasDSMLToolCalls、IsPureDSMLToolCalls、
IsDSMLToolCallEnd、IsDSMLToolCallCut、IsDSMLToolCallReply、ParseDSMLToolCalls、
StripDSMLToolCalls、ExecuteDSMLToolCalls、BuildDSMLToolDoc。

## 提示词严格化

BuildDSMLToolDoc 的 Intro 追加段落（英文，"You have access to a set of
tools..." 之后）：
- 每次调用必须输出完整 tool_calls 包裹的 invoke 块（name 属性），参数必须
  用 parameter 标签带 string 属性；
- 禁止额外属性（如 justification）或示例之外的参数；
- 所有标签必须闭合（invoke 与 tool_calls）；
- 多个 invoke 可同轮（独立调用），依赖调用等上轮结果；
- Schemas 段保留强化现有 "You MUST strictly follow..."。

## handleWebChatToolLoop 改造（internal/lp/handle.go，约 513-560 行）

```
for round := 1; round <= handleWebChatMaxDSMLRounds; round++ {
    msg := dsml.ParseDSMLMessage(lastReasoning, message)
    switch {
    case len(msg.ToolCalls) > 0 && dsml.IsDSMLToolCallReply(message):
        calls, _ := dsml.ParseDSMLToolCalls(message)
        outputs := dsml.ExecuteDSMLToolCalls(ctx, calls)
        if len(outputs) == 0 { /* 现有非可执行分支：stderr 提示 + cleanExit */ }
        feedback := buildWebChatFeedback(outputs)
        if !msg.OK { feedback = dsml.InjectStrictWarning(feedback) }
        // handleWebChatSend(feedback, keep=convURL) → 下一轮
    case len(msg.ToolCalls) > 0 && !dsml.IsDSMLToolCallReply(message):
        return cleanExit()   // 引用示例等：不执行，普通退出
    case len(msg.ToolCalls) == 0 && dsml.SuspectedDSMLToolCalls(message):
        // handleWebChatSend(dsml.ReissueWarning, keep=convURL) → 下一轮
    default:
        return cleanExit()
    }
}
```

- lastReasoning 记录每轮 res.Reasoning（首轮 first.Reasoning）。
- 重发/违规反馈走 handleWebChatSend，受 maxRounds 上限保护。
- printRound/cleanExit/stderr 提示保持；stderr 可加一条中文"工具调用格式
  不够严格，已执行并告警"。

## 迁移清单

- internal/toolcall/dsml.go → internal/dsml/dsml.go；dsml_doc.go → internal/dsml/doc.go
- 测试：toolcall 下 7 个 dsml_*_test.go + ask/dsml_exec_test.go 迁入 internal/dsml/
- 调用点：internal/lp/handle.go、internal/toolcall/ask/{code_dev,code_review,quality_assurance}.go
  （BuildDSMLToolDoc → dsml.BuildDSMLToolDoc）、webchat_cmd.go 注释
- prompt/message.go OK 注释
- 迁移后删除 toolcall 下的 dsml.go/dsml_doc.go（不留空壳）

## 测试

1. 迁移后原测试全绿。
2. 新增 ParseDSMLMessage 单测：合规→OK=true+剥离；justification→OK=false+执行+Content 保留；
   stray close→OK=false；缺 string→OK=false；截断→ToolCalls==0+Suspected=true；
   纯散文→Suspected=false；reasoning 降级解析；content 优先。
3. InjectStrictWarning：注入、已有 warning 追加、空块跳过、空输入返回空。
4. lp 集成测试（沿用 handleWebChatExecDSML hook）：违规→执行+feedback 含 StrictWarning；
   疑似截断→发 ReissueWarning 循环继续；引用示例→不执行退出。
5. dsml_doc_test：断言新严格措辞存在。

## 验收

go build ./... / go vet ./... / go test ./... 全绿 / make fmt-check 通过 /
toolcall 无 DSML 解析判定代码 / 英文 commit / 工作树干净。
