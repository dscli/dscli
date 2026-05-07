// Package chimein 提供用户插话功能。
//
// 在 LLM 与 ToolCall 多轮交互过程中，用户可通过 dscli climein 命令
// 向当前 session 注入纠正指令，在下一轮 ChatRound 自动生效。
//
// # 设计
//
// chimeins 表每 session 仅一行（session_id UNIQUE），Append 追加内容，
// Get 读取，Reset 清空。追加格式为 "\n" + content + "\n"，多次插话以
// 空行分隔。
//
// # 注入点
//
// ChatRound 中 HandleToolCalls 之后、递归之前插入 chimein 消息。
// 此时 tool 结果已写入 history，LLM 下一轮能看到用户纠正。
//
// # 并发
//
// 不同 session 各行其道，无行级竞争。SQLite 数据库级锁保证写入原子性。
// 所有操作通过 sqlite.OpenDB + Close 获取短暂连接，不长期占用。
//
// # 使用
//
//	# 终端 A（chat 进行中）
//	$ dscli chat "重构 main.go"
//
//	# 终端 B（插话）
//	$ dscli climein "注意向后兼容"
//	$ dscli climein --input instructions.txt
//	$ dscli climein <<'EOF'
//	别改 public API
//	单测一并更新
//	EOF
package chimein
