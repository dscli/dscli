# 任务：续作 internal/dsml 包重组（进行到一半）

背景：此前一个 dev 会话把 `internal/toolcall/dsml.go` 与 `dsml_doc.go` 的
迁移做了一半后断线。你现在接手完成。**首要动作：运行 `git status` 与
`go build ./...` 查看现场**，然后按下述清单完成。架构规范详见
`docs/task-dsml-pkg.md`（必须完整阅读并遵守其 §ParseDSMLMessage 语义、
§违规清单、§提示词严格化、§循环改造、§测试要求）。

## 已完成的现场（勿重做）

1. git mv 已完成：internal/toolcall/dsml.go → internal/dsml/dsml.go
   （包名已改 package dsml）；dsml_doc.go → internal/dsml/doc.go（已改包名）；
   7 个测试文件 + ask/dsml_exec_test.go → internal/dsml/（测试包名**未改**，
   仍是 package toolcall）。
2. internal/toolcall/dsml_bridge.go 已创建：导出 RoleToolsSpec、
   RoleToolAllowSet、AllowSetFromSpec、ExecuteToolCallsNoSave、UnregisterTool
   （包装 tool.go 的 roleToolsSpec/roleToolAllowSet/allowSetFromSpec/
   executeToolCalls，供 dsml 包与测试使用）。保留它，检查是否完整可用。
3. internal/dsml/dsml.go 已改包名与 import，但**新功能尚未实现**
   （无 ParseDSMLMessage / SuspectedDSMLToolCalls / InjectStrictWarning /
   违规跟踪）。现编译不过：doc_test.go 等测试还在 package toolcall。

## 剩余工作（按顺序）

1. 修全部迁移测试：包名改 package dsml；其引用的 toolcall 内部符号
   （测试注册/清理 helper 等）改用 dsml_bridge.go 导出的接口，或把通用
   helper 迁入 dsml 包内（保持最小改动，能编译通过且语义不变即可）。
2. 实现违规跟踪：ParseDSMLToolCalls 内部加 strict/violated 汇总（§违规清单
   8 条：justification、normalize 改动、stray close、隐式关闭、wrapper typo、
   裸 invoke、缺 string 属性、嵌套掩码）；重复 parameter 不算违规。
3. 实现 ParseDSMLMessage、SuspectedDSMLToolCalls、InjectStrictWarning 与
   StrictWarning/ReissueWarning 常量（严格按 docs/task-dsml-pkg.md）。
   执行路径：dsml 解析出的调用经 ExecuteToolCallsNoSave（bridge）执行，
   输出转 tool_result 块（formatDSMLToolResult 迁移进 dsml 包）。
4. BuildDSMLToolDoc：Intro 追加严格格式声明（禁止 justification 等额外属性、
   必须闭合标签、必须带 string 属性、tool_calls 包裹）。
5. internal/lp/handle.go handleWebChatToolLoop 改造：
   - ParseDSMLMessage 判定 + 三路分支（执行/引用退出/疑似重发）按
     docs/task-dsml-pkg.md §循环改造伪码；
   - !msg.OK 时 feedback = dsml.InjectStrictWarning(feedback)；
   - 疑似失败发 dsml.ReissueWarning 并继续循环（keep=convURL）。
6. 调用点：internal/toolcall/ask/{code_dev,code_review,quality_assurance}.go
   中 toolcall.BuildDSMLToolDoc → dsml.BuildDSMLToolDoc（如果桥接保留
   BuildDSMLToolDoc 转发也行，但规范要求 dsml 包拥有它；调用点用 dsml 包）。
   检查 webchat_cmd.go 现有注释是否需要更新（仅注释，可选）。
7. prompt/message.go：OK 字段注释补充双语义说明（history 配对完整性 vs
   DSML 格式合规）。
8. 新增测试：ParseDSMLMessage（合规/justification/stray close/缺 string/
   截断+疑似/纯散文/reasoning 降级/content 优先）、InjectStrictWarning、
   lp 循环集成（违规执行+warning、疑似重发继续、引用示例退出）、
   dsml_doc_test 严格措辞断言。
9. go build ./... / go vet ./... / go test ./... 全绿 / make fmt-check 通过。
10. 确认 toolcall 包无残留 DSML 解析判定代码（dsml_bridge.go 是执行桥接，
    允许保留）；git 提交（英文 message，如 "refactor(dsml): extract
    internal/dsml package with strict format judgement"），工作树干净。

## 注意

- 若 dev 断线前留下的 dsml.go 内部还有半成品改动（如被拆开的函数），
  以"编译通过 + 测试全绿 + 符合 task-dsml-pkg.md"为准整理，不必追认旧逻辑。
- 不要碰 docs/ 下的两个 md 文件（那是规范）。
- 若 DeepSeek 繁忙/断线，分多次提交也行（每次小步 commit，最后汇总），
  但最终状态必须满足上表第 9、10 条。
