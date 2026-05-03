// Package prompt 负责 prompt 构建、消息持久化、历史回忆与对话笔记。
//
// 本包提供两类功能：
//  - 底层实现（SearchMessages, SaveNote 等）：供上层调用
//  - 工具处理函数（HandleRecall, HandleNote）：供 toolcall/history 直接注册为 LLM 工具
//
// recall 和 note 归入 history 类别（toolcall/history），与 memory 系列
// （toolcall/memory）区分：前者限定当前 session，后者是跨 session 的全局持久记忆。
//
// # Prompt 构建 (prompt.go)
//
// 按优先级加载 chat.md / reasoner.md：
//  1. ${PROJECT_ROOT}/.dscli/prompt/  — 项目级（可编辑）
//  2. ~/.dscli/prompt/                 — 系统级（可编辑）
//  3. 内嵌源文件                         — 只读兜底
//
// # 消息持久化 (message.go)
//
// Message 结构体对应 messages 表，SaveMessages 在同一事务中
// 写入 messages 表并同步 FTS5 索引（messages_fts），支持中日韩
// 全文搜索。
//
// # 历史回忆 (recall.go)
//
// SearchMessages：底层 FTS5 + 中文分词搜索，仅匹配 user 消息
// 和助手总结（无 tool_calls 的 assistant），限定当前 session，
// 按相关性排序。
//
// HandleRecall：工具处理函数，解析关键词、调用 SearchMessages、
// 格式化结果并做截断保护（单条 ≤2000 字，总条数 ≤10），防止撑爆
// LLM 上下文。
//
// # 对话笔记 (note.go)
//
// SaveNote / LoadNotes：基于 notes 表存储/加载简短摘要（≤40字），
// 记录跨对话的关键信息。
//
// HandleNote：工具处理函数，在 SaveNote 基础上增加超长截断警告提示。
// BuildNotePrompt：将近期笔记注入 system prompt 作为回忆线索。
//
// # 会话历史 (history.go)
//
// LoadHistory 加载完整消息历史，JudgeHistory 管理历史累积与截断，
// 为 LLM 调用构建上下文。同时提供 UpdateHistory 等维护方法。

package prompt
