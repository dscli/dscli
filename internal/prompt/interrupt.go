package prompt

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/sqlite"
	"github.com/nanjj/clog"
)

// InterruptedToolContent 生成占位 tool 消息内容。
// 用户中断时用它替代未执行的工具调用结果，维持 assistant↔tool 配对。
func InterruptedToolContent(name string) string {
	return fmt.Sprintf("⚠️ 工具调用 %s 被用户中断（Ctrl+C），未执行", name)
}

// MarkInterruptedToolCalls 修正用户主动中断（Ctrl+C / kill）后遗留的消息不变量。
//
// 背景：ChatRound 在工具执行前先保存 assistant(tool_calls) 消息。若进程在
// 工具执行中被终止，数据库最后一条是带 tool_calls 的 assistant，重启后
// ChatRunE 会自动重放未完成的工具调用——对用户主动取消而言这是错误行为
// （上次已执行/已取消的操作会被再次执行）。
//
// 本函数在可捕获信号（SIGINT/SIGTERM）到达时调用：
//  1. 找到最后一条带 tool_calls 的 assistant 消息；
//  2. 将 tool_calls 裁剪为已完成的前 completed 个（结果已落库）；
//  3. 为每个未执行的调用插入占位 tool 消息，保持 API 协议配对。
//
// 完成后重启，历史最后一条是占位 tool 消息，ChatRunE 不会重放。
// 返回插入的占位消息数；无可处理残留时返回 0。
func MarkInterruptedToolCalls(ctx context.Context, completed int) (n int, err error) {
	msg, err := lastToolCallMessage(ctx)
	if err != nil {
		return 0, err
	}
	if msg.ID == 0 {
		return 0, nil // 无残留调用
	}
	if completed >= len(msg.ToolCalls) {
		return 0, nil // 全部完成，无需处理
	}

	// 裁剪 assistant 消息：只保留已完成（结果已保存）的调用。
	// completed=0 时存 "[]"，重启后同样不会重放。
	if err := UpdateToolCalls(ctx, msg.ID, msg.ToolCalls[:completed]); err != nil {
		return 0, fmt.Errorf("裁剪 tool_calls 失败: %w", err)
	}

	// 为未执行的调用插入占位 tool 消息。
	placeholders := make([]Message, 0, len(msg.ToolCalls)-completed)
	for _, tc := range msg.ToolCalls[completed:] {
		name := tc.Function.Name
		if name == "" {
			name = tc.ID
		}
		placeholders = append(placeholders, Message{
			Role:       "tool",
			ToolCallID: tc.ID,
			Content:    InterruptedToolContent(name),
		})
	}
	if err := SaveMessages(ctx, placeholders...); err != nil {
		return 0, fmt.Errorf("插入占位 tool 消息失败: %w", err)
	}
	return len(placeholders), nil
}

// lastToolCallMessage 返回当前会话中最后一条带 tool_calls 的 assistant 消息。
// 无匹配消息时返回 ID 为 0 的空 Message。
func lastToolCallMessage(ctx context.Context) (Message, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "lastToolCallMessage")
	defer span.Finish()

	sessionID := GetCurrentSessionID(ctx)
	modelID := context.ContextValue(ctx, context.CurrentModelIDKey, context.DeepseekChat)

	db, err := sqlite.OpenDB(ctx)
	if err != nil {
		return Message{}, err
	}
	defer db.Close(ctx)

	var m Message
	var toolCalls sql.NullString
	err = db.QueryRowContext(ctx, `
		SELECT id, role, content, tool_calls
		FROM messages
		WHERE session_id = ? AND model_id = ? AND role = 'assistant'
		  AND tool_calls IS NOT NULL AND tool_calls != ''
		ORDER BY id DESC LIMIT 1`,
		sessionID, modelID).Scan(&m.ID, &m.Role, &m.Content, &toolCalls)
	if err != nil {
		if err == sql.ErrNoRows {
			return Message{}, nil
		}
		return Message{}, fmt.Errorf("查询最后一条工具调用消息失败: %w", err)
	}
	if toolCalls.Valid {
		if err := json.Unmarshal([]byte(toolCalls.String), &m.ToolCalls); err != nil {
			return Message{}, fmt.Errorf("解析 tool_calls 失败: %w", err)
		}
	}
	return m, nil
}
