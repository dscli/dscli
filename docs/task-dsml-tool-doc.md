# Task: DSML Tool Doc 重构 — Intro 按工具分段、Schemas 退役、dev.md 措辞调整

## Background

`internal/dsml/doc.go` 的 `BuildDSMLToolDoc` 生成 DSML 工具注册段的两部分,注入 WebChat 角色提示词:

- `Intro`: `## 🛠️ Available Tools: ...` 头部 + 格式要求 + 每工具一个 ```xml 示例 + string= 编码规则 + 参数说明列表(`- \`name\`: ...`)
- `Schemas`: `### Available Tool Schemas` + 每个工具完整 JSON schema(很长)+ 严格遵循句

用户反馈(当前版本实测确认):

1. **JSON Schemas 无益**:JSON 格式很长、工具多,展示不能引起 LLM 注意,反而挤占上下文。决定:五个角色模板全部去掉 `{{.DSMLToolDoc.Schemas}}`,`BuildDSMLToolDoc` 不再生成 Schemas 段,`prompt.DSMLToolDoc.Schemas` 字段**删除**(退役,无其他用途)。
2. **Intro 改造为按工具分段**:把原来只在 Schemas JSON 里出现的工具描述(`def.Description`)拿出来,每个工具一段:名称标题 + 工具描述 + 参数说明 + ```xml 示例。
3. **dev.md L34 "Ask, don't pretend" 移除**:不再鼓励/暗示 code_dev ask user(ask user 是 architect 的事)。把该条移到 `{{else}}`(无工具兜底 Capabilities)分支——有工具的 code_dev 会话不展示。

## Goal

改动后 `BuildDSMLToolDoc("dev")` 的 `Intro` 输出精确格式如下(**这就是唯一权威格式,字段顺序、标题级别、空格必须一致**):

```
## 🛠️ Available Tools: `shell`, `read_file`

You have access to a set of tools to help answer the user's question. Call the
tools with DSML markup in your reply. Independent calls may be issued in one
reply; dependent calls must wait for previous results.

You MUST follow this exact format for every tool call:
- Wrap all calls in a tool_calls block; each call is an invoke block with a name attribute.
- Every argument MUST be a parameter tag carrying a string attribute (string="true" for text, string="false" for numbers/booleans/arrays/objects).
- Do NOT add any extra attribute (such as justification) or any argument outside the examples.
- Every invoke tag and every tool_calls tag MUST be closed (the close tag must be present).
- You may emit several invoke blocks in one reply (independent calls); dependent calls must wait for the previous round's results.

## `shell`

Run a shell script.

Parameters: `script` (string, required) — Shell script content.; `summary` (string, required) — Brief summary.; `timeout` (integer, optional) — Timeout in seconds.

```xml
<tool_calls>
<invoke name="shell">
<parameter name="script" string="true">...</parameter>
<parameter name="summary" string="true">...</parameter>
<parameter name="timeout" string="false">0</parameter>
</invoke>
</tool_calls>
```

## `read_file`

Read a file or line range.

Parameters: `path` (string, required) — File path.; `start_line` (integer, optional) — Start line.; `end_line` (integer, optional) — End line.

```xml
<tool_calls>
<invoke name="read_file">
<parameter name="path" string="true">...</parameter>
<parameter name="start_line" string="false">0</parameter>
<parameter name="end_line" string="false">0</parameter>
</invoke>
</tool_calls>
```

String parameters should be specified as is and set `string="true"`. For all
other types (numbers, booleans, arrays, objects), pass the value in JSON format
and set `string="false"`.

You MUST strictly follow the above defined tool name and parameter schemas to invoke tool calls.
No extra attributes (such as justification), no extra arguments, and every tag closed - output the exact DSML shape shown above.
```

要点:
- 工具段标题是二级标题 `## \`name\``(与用户要求的 `## 工具名1` 一致;工具名保留反引号引用风格,与旧 `Available Tools` 行内一致)。
- 每工具段顺序:标题 → 工具描述(def.Description,原文)→ `Parameters: ...` 行 → ```xml 示例。
- `Parameters:` 行 = 原 `paramsLine` 去掉工具名前缀:`Parameters: \`script\` (string, required) — Shell script content.; ...`(参数间 `; ` 分隔,末尾无句号;无参数工具为 `Parameters: (no parameters)`)。
- Intro 末尾必须保留这两句(原 Schemas 段的):
  `You MUST strictly follow the above defined tool name and parameter schemas to invoke tool calls.`
  `No extra attributes (such as justification), no extra arguments, and every tag closed - output the exact DSML shape shown above.`
- 其余头部段落(前 5 条格式要求 bullet、`String parameters should be specified...` 编码规则段)保持现状原文。
- `BuildDSMLToolDoc` 不再生成 `### Available Tool Schemas` 与任何 JSON;返回 `prompt.DSMLToolDoc{Intro: intro}`(Schemas 字段已删除)。

## File-level changes

### 1. `internal/dsml/doc.go`
- `dsmlDocTool` 结构:删除 `schema map[string]any` 字段;新增 `description string` 字段(取 `def.Description`)。
- `dsmlGeneratedDocEntry`:不再构建 `schema` map;填充 `description`;example/paramsLine 生成逻辑不变(保留 `paramsLine` 的组装,仅 Intro 渲染处去掉前缀)。
- `BuildDSMLToolDoc`:
  - 头部 `## 🛠️ Available Tools: ...` 与格式要求 5 条 bullet 原样保留。
  - 循环渲染每工具:标题 → description → `Parameters: ...` → ```xml 示例(示例原样,含 `\n` 处理与 ``` 围栏)。
  - 删掉末尾统一参数列表(`- \`name\`: ...` 循环)。
  - 编码规则段原样保留,其后追加上述两句严格遵循句。
  - 不再有 `schemas` 构建;`encoding/json` import 若不再使用则删除。
  - 返回 `prompt.DSMLToolDoc{Intro: intro}`。
- 文件头注释如有 "Schemas" 相关描述,同步更新。

### 2. `internal/prompt/prompt.go`
- `DSMLToolDoc` 结构:删除 `Schemas string` 字段;更新注释只描述 Intro(可用工具标题、每工具名称/描述/参数/示例、严格格式要求)。注释中 `toolcall.BuildDSMLToolDoc` 改为 `dsml.BuildDSMLToolDoc`(如方便)。
- `promptConfig.DSMLToolDoc` 注释中 "与 JSON schemas" 措辞同步为 "(名称、描述、参数说明、示例)"。

### 3. 角色模板(5 个,删除 Schemas 引用行)
- `internal/prompt/dev.md`:删除 `{{.DSMLToolDoc.Schemas}}` 行(保留前后空行结构)。
- `internal/prompt/architect.md`、`internal/prompt/review.md`、`internal/prompt/expert.md`、`internal/prompt/test.md`:同样删除 `{{.DSMLToolDoc.Schemas}}` 行。
- 注意各模板 `{{if .DSMLToolDoc.Intro}}` 分支内,删除 Schemas 行后剩余的段落间空行要合理(一行内容后接 `{{else}}`,保持一个空行分隔)。

### 4. `internal/prompt/dev.md` — L34 移动
- 从 `## 🧠 Thinking Principles` 里删除:
  `- **Ask, don't pretend**: ask the user or experts rather than pretending to know`
- 在 `{{else}}` 分支的 `## 🛠️ Capabilities` 段落末尾(原文 `If no tools are available in this session, state that limitation instead of claiming actions you could not perform.` 之后)追加一句(空行分隔):
  `If information is missing, ask the user or experts rather than pretending to know.`
- 其余 Thinking Principles 条目(Logical rigor / Systems thinking / Depth-first)不动。不动 `{{if .DSMLToolDoc.Intro}}` 判断条件。

### 5. 测试更新
- `internal/dsml/doc_test.go`:
  - `TestBuildDSMLToolDocDefaults`: `doc.Intro == "" && doc.Schemas == ""` → 只判 `doc.Intro == ""`;`doc.Intro == "" || doc.Schemas == ""` 的检查删除/改为只检查 Intro。
  - `TestBuildDSMLToolDocContent`:
    - 删除断言 `### Available Tool Schemas`、`"name": "shell"`、`"name": "read_file"`、`- \`shell\`: \`script\``、`- \`read_file\`:`。
    - 新增断言:工具描述 `Run a shell script.`、`Read a file or line range.` 出现在 `doc.Intro`。
    - 修改:参数断言改为 `Parameters: \`script\` (string, required) — Shell script content.`。
    - 保留:`## 🛠️ Available Tools:`、`<tool_calls>`、`<invoke name="shell">`、`<parameter name="script" string="true">`、`String parameters should be specified as is and set \`string="true"\``、`You MUST strictly follow the above defined tool name and parameter schemas to invoke tool calls.`(现在在 Intro 中)。
    - `exec_command`、`{{` 泄漏检查照旧(去掉 `doc.Schemas` 相关分支)。
    - 测试注释同步(不再提 JSON schemas;说明新格式:每工具 名称/描述/参数/示例)。
  - `TestBuildDSMLToolDocRoleConfig`: `doc.Intro != "" || doc.Schemas != ""` → 只判 `doc.Intro != ""`。
  - `TestBuildDSMLToolDocGenerated`: 删除 `"name": "probe_tool"` 断言;新增 `probe description`(描述)出现在 Intro;其余 `\`query\` (string, required) — SQL query` 等参数断言改为 `Parameters: \`query\` (string, required) — SQL query; \`limit\` (integer, optional) — Row cap`(按新 `Parameters:` 行格式校准)。
- `internal/prompt/prompt_test.go`:两处 `DSMLToolDoc{Intro: ..., Schemas: ...}` 构造删除 `Schemas` 字段(只留 Intro);如有对 `Schemas` 的断言一并处理。

### 6. 文档同步
- `AGENTS.md`(约 L161-168):把 "formatting aligned with DeepSeek V4's tool template (string= attribute rules, `### Available Tool Schemas`)" 改为描述新格式:每工具 `## \`name\`` 标题 + 描述 + 参数 + 示例,无 JSON schemas 段。
- `internal/skills/dscli-skill.md`(L11): "The system prompt already includes tool JSON schemas" → 改为 "The system prompt already includes per-tool descriptions, parameters, and examples"。
- `docs/architecture-dsml-pkg.md`:更新对 Schemas 段的描述(约 L138-139、L127-131 等处),说明 Schemas 已退役、描述并入 Intro 每工具段。`docs/task-dsml-pkg.md`/`docs/task-dsml-continue.md`/`internal/dsml/testdata/case5_agent_task.txt` 是历史任务文档,不改。
- `internal/dsml/dsml.go` L879 附近注释若提到 schemas(dsml_doc.go),按需微调;`internal/prompt/architect.md` 等模板注释无需改(模板只有注入行)。

## Constraints

- 不要改动 `BuildDSMLToolDoc` 的签名之外的调用方接口(`PromptInfo.DSMLToolDoc` 字段名、`RenderPromptForRoleWithTools` 逻辑、`{{if .DSMLToolDoc.Intro}}` 判断全部保持)。
- 不要改动 DSML 解析/执行逻辑(dsml.go、lp/handle.go、webchat)。
- `dsmlGeneratedDocEntry` 的 example 生成(参数默认值 `...`/`0`/`true`/`[]`/`{}`)保持原样。
- 工具名列表 `## 🛠️ Available Tools: \`a\`, \`b\`` 行保持现状。
- 所有 git commit 消息用英文。

## Acceptance criteria

1. `go build ./...` 通过;`go test ./...` 全绿(尤其 `internal/dsml`、`internal/prompt` 包)。
2. `make fmt-check` 通过(gofumpt + goimports);改动文件用 `make gofmt` 格式化。
3. 运行时验证(可写临时 Go test 或直接用 `go test -run TestBuildDSMLToolDocContent` 的输出):`BuildDSMLToolDoc(ctx, "dev")` 返回的 Intro 与上面的权威格式逐段一致;`DSMLToolDoc.Schemas` 已不存在(编译期保证)。
4. 五个角色模板中不再出现 `{{.DSMLToolDoc.Schemas}}`(grep 确认)。
5. dev.md 中不再出现 "Ask, don't pretend";`{{else}}` 分支 Capabilities 段末尾包含 "ask the user or experts rather than pretending to know"。
6. 提交流程:实现 → `go test ./...` → `make fmt-check` → 单一英文 commit(如 `refactor(dsml): per-tool intro docs, retire JSON schemas`),工作树干净,报告 commit hash 与测试结果。
