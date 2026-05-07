// Package chimein 提供用户插话功能。
//
// # 背景
//
// 当 LLM 与 ToolCall 进行多轮交互时（如代码修改→编译→报错→再修改），
// 用户可能希望在中途插入纠正指令（"换个方向"、"别用那个库"），
// 而不中断当前会话。chimein 包实现了这一能力。
//
// # 架构设计
//
// chimein 采用"信号量"模式：用户通过 dscli climein 命令写入消息，
// ChatRound 在每一轮结束后检查是否有新内容，有则注入到 LLM 对话流中。
//
// 数据流：
//
//	用户终端 A (dscli climein)      用户终端 B (dscli chat)
//	─────────────────────────       ─────────────────────────
//	$ echo "修正方向" | dscli climein
//	        │                               │
//	        ▼                               │
//	  chimein.Append(ctx, "修正方向")        │
//	        │                               │
//	  INSERT/UPDATE chimeins 表 ◄───────────┤
//	        │                               │
//	        │                         ChatRound 循环:
//	        │                           HandleToolCalls()
//	        │                               │
//	        │                           chimein.Get() → "修正方向"
//	        │                               │
//	        │                           注入 user message 到 history
//	        │                               │
//	        │                           chimein.Reset()
//	        │                               │
//	        │                           下一轮 ChatRound (带修正)
//
// # 数据模型
//
// chimeins 表设计（一个 session 仅一行）：
//
//	CREATE TABLE chimeins (
//	    id          INTEGER PRIMARY KEY AUTOINCREMENT,
//	    session_id  INTEGER UNIQUE NOT NULL,  -- 与 sessions 表关联
//	    content     TEXT    NOT NULL DEFAULT '',
//	    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
//	)
//
// 设计决策：
//   - session_id UNIQUE —— 保证每个 session 最多一行，避免并发混乱
//   - 追加格式 "\n" + content + "\n" —— 多次插话间以空行分隔，可读性好
//   - ON DELETE CASCADE —— session 结束时自动清理
//   - Reset 仅将 content 设为空串，不删行 —— 避免频繁 INSERT/DELETE
//
// # API
//
//   - Append(ctx, content) —— 追加内容（首次创建行，后续追加）
//   - Get(ctx) → (content, error) —— 获取当前内容（无内容返回 ""）
//   - Reset(ctx) —— 清空内容（设为空串，不删行）
//
// # 注入点
//
// 选在 ChatRound 中 HandleToolCalls 之后、递归 ChatRound 之前：
//
//	toolInputs := toolcall.HandleToolCalls(ctx, tcs)
//	history = append(history, toolInputs...)
//
//	// chimein 检查
//	if content, err := chimein.Get(ctx); err == nil && content != "" {
//	    msg := prompt.Message{Role: "user", Content: content}
//	    history = append(history, msg)
//	    outfmt.PrintUserContent(ctx, content)
//	    prompt.SaveMessages(ctx, msg)
//	    chimein.Reset(ctx)
//	}
//
//	return ChatRound(ctx, prompts, history)
//
// 选在此处的理由：tool 结果已写入 history，用户可基于最新结果进行纠正；
// 注入发生在 LLM 下一轮之前，确保 LLM 能看到用户的纠正指令。
//
// # 并发安全
//
// 每个 session 唯一一行，Append 和 Get/Reset 操作不同 session，
// 因此不存在行级竞争。SQLite 的数据库级锁保证了写入原子性。
// dscli climein 和 dscli chat 通过 sqlite.OpenDB() + Close() 各自
// 获取短暂连接，不会长期占用。
//
// # 使用示例
//
// 终端 A（chat 会话进行中）：
//
//	$ dscli chat "重构 main.go"
//	[LLM 开始修改代码...]
//	[ToolCall: write_file main.go...]
//
// 终端 B（用户插话）：
//
//	$ dscli climein "注意保持向后兼容"
//
//	# 或从文件读取
//	$ dscli climein --input fix-instructions.txt
//
//	# 或 heredoc 多行
//	$ dscli climein <<'EOF'
//	别改 public API 签名
//	单测也必须一并更新
//	EOF
//
// 终端 A 下一轮 ChatRound 自动读取并注入该消息。
package chimein
