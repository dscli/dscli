package issue

import (
	"context"
	"fmt"
	"strings"

	"gitcode.com/dscli/dscli/internal/toolcall"
)

// handleIssueList 处理issue列表查询（Tool Calling）
func handleIssueList(ctx context.Context, args toolcall.ToolArgs) (result string, warning string, err error) {
	state := toolcall.ToolArgsValue(args, "state", "open")

	// 验证状态参数
	if state != "open" && state != "closed" && state != "all" {
		err = fmt.Errorf("状态必须是 'open'、'closed' 或 'all'，收到: %s", state)
		return
	}

	issues, err := ListIssues(ctx, state)
	if err != nil {
		return
	}

	if len(issues) == 0 {
		result = fmt.Sprintf("没有找到状态为 '%s' 的issues", state)
		return
	}

	// 构建结果
	var b strings.Builder
	fmt.Fprintf(&b, "📋 Issues (状态: %s, 总数: %d):\n\n", state, len(issues))

	for _, issue := range issues {
		assigneeInfo := "-"
		if issue.Assignee != nil {
			if issue.Assignee.Name != "" {
				assigneeInfo = fmt.Sprintf("%s (%s)", issue.Assignee.Name, issue.Assignee.Login)
			} else {
				assigneeInfo = issue.Assignee.Login
			}
		}

		labelsInfo := "-"
		if len(issue.Labels) > 0 {
			var labelNames []string
			for _, label := range issue.Labels {
				labelNames = append(labelNames, label.Name)
			}
			labelsInfo = strings.Join(labelNames, ", ")
		}

		fmt.Fprintf(&b, "## #%s [%s] %s\n", issue.Number, issue.State, issue.Title)
		fmt.Fprintf(&b, "  ID: %d | 作者: %s | 负责人: %s\n",
			issue.ID, issue.User.Login, assigneeInfo)
		fmt.Fprintf(&b, "  创建时间: %s | 更新时间: %s\n",
			formatTime(issue.CreatedAt), formatTime(issue.UpdatedAt))
		fmt.Fprintf(&b, "  标签: %s\n", labelsInfo)

		if issue.Body != "" {
			preview := issue.Body
			if len(preview) > 100 {
				preview = preview[:100] + "..."
			}
			fmt.Fprintf(&b, "  预览: %s\n", preview)
		}
		b.WriteString("\n")
	}
	result = b.String()
	return
}

func init() {
	// 注册issue相关工具
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "issue_list",
		Description: "列出项目中的issues，支持按状态过滤",
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"state": map[string]any{
					"type":        "string",
					"description": "issue状态：open（打开）、closed（关闭）、all（全部），默认为open",
					"enum":        []string{"open", "closed", "all"},
				},
			},
			"required":             []string{},
			"additionalProperties": false,
		},
		Category: "issue",
		Handler:  handleIssueList,
	})
}
