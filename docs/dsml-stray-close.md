# DSML 容错：完整工具调用后多余的 `</invoke>`

## 背景（真实事故）

2026-08-30 QA 会话中的工具调用从未执行，反复卡在同一调用上。现场输出（chat.deepseek.com 存储内容，org 记录）为：

```
<invoke name="shell">
<parameter name="script" string="true">sed -n '190,225p' .../runewidth.go</parameter>
<parameter name="summary" string="true">Read Condition.Truncate body</parameter>
</invoke>
</invoke>
```

模型在 `</invoke>` 之后又输出了一行 `</invoke>`（重复闭合标签，token 伪影）。该调用在 dscli 侧被当作普通文本——从未执行、无反馈，专家的 web 会话无限等待。

## 根因链（lp/handle.go 门控）

`lp/handle.go:355` 以 `toolcall.IsDSMLToolCallReply(res.Content)` 门控是否进入工具循环。该函数 = `IsDSMLToolCallEnd || IsDSMLToolCallCut || IsPureDSMLToolCalls`：

| 分支 | 对 `<invoke>…</invoke></invoke>` | 结果 |
|------|------|------|
| `IsDSMLToolCallEnd` | 无 `</tool_calls>`/`</_calls>` wrapper | false |
| `IsDSMLToolCallCut` | 无截断 close（`</` 结尾） | false |
| `IsPureDSMLToolCalls` | Parse 成功（状态机把栈空 close 当 noise 忽略，得到 1 个完整 block）；但 `StripDSMLToolCalls` 剥离 block 后**残留孤立 `</invoke>`** → 非空 | **false** |

三路全 false → 工具循环不进入 → 调用静默丢失。**根因是 `StripDSMLToolCalls` 不剥离孤立 close，导致 IsPure 判定失败**（Parse 本身已经正确）。

## 设计（精确、最小）

### 1. `dsmlBlockRanges` 记录孤立 close（栈空事件）

状态机 `case 'c':` 目前只在 `paramDepth == 0 && len(stack) > 0` 时配对；其余情况直接忽略。改为：

- `paramDepth == 0 && len(stack) > 0` → 配对（现状不变）
- `paramDepth == 0 && len(stack) == 0` → **外部孤立 close**（token 伪影），记录其字节区间 `[pos, end)` 到新返回值 `strays`
- `paramDepth > 0` → 参数值内字面量（内容，不记录，现状不变）

函数签名：`dsmlBlockRanges(text string) (blocks []dsmlBlockRange, unclosed int, firstUnclosed int, strays []dsmlStrayClose)`（`dsmlStrayClose` 可用轻量类型 `struct{ pos, end int }`；也可复用 `dsmlBlockRange`，实现自定，语义必须清晰）。

**为什么不能全局正则剥 `</invoke>`**：fenced code / 内联 code（`dsmlCodeRanges` 使这些位置的事件根本不生成）与参数值内（`paramDepth > 0`）的字面 `</invoke>` 是**内容而非结构**（现有"引用示例安全边界"，见 dsml.go 注释与 `dsml_bareinvoke_test.go`/`dsml_realpayload_test.go` 用例）。strayst 事件天然排除这两类位置——精确且零误伤。

### 2. `StripDSMLToolCalls` 把 strays 纳入删除区间

现有逻辑：按 blocks 区间拼接保留文本（嵌套跳过 + `firstUnclosed` chop + wrapper/cut 清理）。扩展：

- 将 strays 区间并入删除处理。strays 全部位于块外、按 pos 递增且互不重叠（同一位置只有一种事件）；构造统一删除区间并排序后切片即可，或与 blocks 循环并列处理（保留段内再切 strays）。
- 保留既有行为不变：`first >= 0` 时 chop 尾部（stray 位于 chop 点之后自然被裁）；嵌套 block 跳过；末尾 `dsmlToolCallsCloseCutRe` 清理；`<tool_calls>`/`<tool_result>` 残留清理。

### 3. `ParseDSMLToolCalls` / `IsPureDSMLToolCalls` / `IsDSMLToolCallReply`

- `ParseDSMLToolCalls`：忽略 strays（行为不变——孤立 close 本就不参与配对；调用点补 `_`）。
- 修复后 `<invoke>…</invoke></invoke>` → Parse 1 call、Strip 空 → `IsPureDSMLToolCalls` true → `IsDSMLToolCallReply` true → 工具循环正常执行。
- `IsDSMLToolCallEnd` / `IsDSMLToolCallCut` **不改**（无 wrapper 的形状由 IsPure 分支覆盖）。

### 4. 不做的事

- 不整体"修复" XML（如容忍缺 `</invoke>` 的隐式 close 只在 wrapper 结尾信号存在时触发——已有该机制，不扩大）。
- 不剥 `</parameter>` 多余闭合（无观察证据，避免扩大安全面）。
- 不改 normalizeDSMLText。

## 测试矩阵（internal/toolcall/dsml_test.go 或新文件 dsml_strayclose_test.go）

复现夹具：消息以 `<invoke name="shell">…</invoke>\n</invoke>` 结尾（含空行，模拟 org 记录原文）。

1. **尾随多余 close（单/双）**：`<invoke>A</invoke></invoke>` 与 `<invoke>A</invoke></invoke></invoke>` →
   - `ParseDSMLToolCalls` 返回 1 call 且 err == nil
   - `IsPureDSMLToolCalls` == true
   - `StripDSMLToolCalls` == ""
   - `IsDSMLToolCallReply` == true
2. **多调用 + 尾随多余**：`<invoke>A</invoke><invoke>B</invoke></invoke>` → 2 calls、strip ""、IsPure true。
3. **中间多余 close**：`<invoke>A</invoke></invoke><invoke>B</invoke>` → 2 calls、strip ""（通用化容错）。
4. **安全边界（必须不回归）**：
   - 参数值内字面 `</invoke>`（完整调用，value 携带 `... </invoke> ...`）：Parse 成功且参数值**原样保留**；strip 不误删。
   - fenced code（``` 块）内引用 `<invoke>…</invoke>` 示例：`IsPureDSMLToolCalls` == false（引用非指令）；strip 保留 code 内容原样（包括块内 close）。
   - pure prose + 孤立 `</invoke>`（无任何 open）：strip 结果 == prose（孤立 close 被剥）；IsPure false（prose 残留）。
5. **现有全套**（53 个函数测试，尤其 dsml_realpayload / dsml_doc / dsml_missinginvoke / dsml_bareinvoke / dsml_test）必须全绿——零行为回归。

## 验收

- `go test ./...` + `make fmt-check` 全绿。
- 上述真实夹具（org 原文形状）进入测试，确保事故形状被锁死。
- commit 中文档入仓：英文 message，如 `fix(dsml): tolerate stray </invoke> after complete tool calls`。
