package toolcall

import (
	"encoding/json"

	"github.com/dscli/dscli/internal/prompt"
)

// DualMessage 是工具结果的双消息协议。
//
// 部分工具的语义是"读内容给自己看"（如 vision_file_read：上传本地图片
// 并立即注入对话），此时仅返回 tool 消息不够——图片块只允许出现在
// user 消息中，模型不能在工具轮次后自行插入。因此 handler 可以让
// result 返回 DualMessage 的 JSON：
//
//	{"dual":true,"tool_result":"<tool 消息内容>","user_message":<prompt.Message>}
//
// HandleToolCalls 在构造 tool 消息时检测该格式：Dual 标记避免与普通
// JSON 结果（如文件对象）混淆；命中后拆分为一条 tool 消息（ToolResult，
// 保留 file_id 元数据）和一条附加 user 消息（UserMessage，携带 file 块），
// 使任何走 HandleToolCalls 的会话（chat 主路径、中断恢复、未来运行时）
// 都能在当前轮看到图片，无需等下一个 user turn。
//
// 非视觉模型由 handler 自行判断（不返回 DualMessage 即不注入），
// 与 chat 侧 cleanMessagesForModel 的双重保护配合，避免 API 400。
type DualMessage struct {
	// Dual 标记此消息为双消息格式（true）。普通 JSON 结果不含此字段，
	// SplitDualResult 仅在 Dual 为 true 时拆分。
	Dual bool `json:"dual"`
	// ToolResult 是 tool 消息的内容（如文件对象 JSON，保留 file_id 等
	// 元数据，list/info/delete 仍依赖）。
	ToolResult string `json:"tool_result"`
	// UserMessage 是附加注入的 user 消息（如"图片已加载" + file 块）。
	// 为 nil 时仅返回 tool 消息。
	UserMessage *prompt.Message `json:"user_message,omitempty"`
}

// SplitDualResult 尝试把工具结果解析为 DualMessage。
// 普通结果（含普通 JSON）原样返回，isDual 为 false。
// 三重确认（Dual 标记 + UserMessage 非 nil + role=user）避免把普通
// JSON 结果（如 `{"dual":true,...}` 巧合字段）误拆——DualMessage 是
// 内部协议，user 消息必须由注册的 handler 显式构造。
func SplitDualResult(result string) (toolResult string, userMsg *prompt.Message, isDual bool) {
	if result == "" {
		return result, nil, false
	}
	var d DualMessage
	if err := json.Unmarshal([]byte(result), &d); err != nil || !d.Dual {
		return result, nil, false
	}
	if d.UserMessage == nil || d.UserMessage.Role != "user" {
		return result, nil, false
	}
	return d.ToolResult, d.UserMessage, true
}

// NewDual 构造双消息：Dual 标记与 user 消息 role 由框架保证，
// handler 只关心 ToolResult（tool 消息内容）与 UserMessage（附加 user 消息）。
// UserMessage 必须为 user 角色（SplitDualResult 的确认条件之一）。
func NewDual(toolResult string, userMsg *prompt.Message) DualMessage {
	if userMsg != nil {
		userMsg.Role = "user"
	}
	return DualMessage{Dual: true, ToolResult: toolResult, UserMessage: userMsg}
}

// MarshalDual 将双消息序列化为 handler 的 result 字符串。
// MarshalDual 与 SplitDualResult 互为逆操作。
func MarshalDual(d DualMessage) (string, error) {
	data, err := json.Marshal(d)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
