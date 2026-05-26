package prompt

import (
	"context"
	"fmt"
)

// HandleShow 根据消息 ID 返回完整内容（跳过 reasoning_content），供 LLM 工具调用。
func HandleShow(ctx context.Context, id int64) (result, warning string, err error) {
	m, err := ShowMessage(ctx, id)
	if err != nil {
		return result, warning, fmt.Errorf("获取消息 #%d 失败: %w", id, err)
	}

	roleLabel := "用户"
	if m.Role == "assistant" {
		roleLabel = "助手"
	}

	result = fmt.Sprintf("### 消息 #%d\n- 角色: %s\n- 时间: %s\n\n%s",
		m.ID, roleLabel, FormatTime(m.CreatedAt), m.Content)
	return result, warning, err
}
