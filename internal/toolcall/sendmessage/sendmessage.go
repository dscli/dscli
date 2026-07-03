// Package sendmessage implements the send_message tool.
//
// send_message dispatches a message to a dscli chat session for a given
// project via Emacs daemon (emacsclient --eval).  The call is fire-and-forget:
// the message is queued and control returns immediately.
package sendmessage

import (
	_ "embed"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/session"
	"github.com/dscli/dscli/internal/toolcall"
	"github.com/nanjj/clog"
)

//go:embed sendmessage.md
var sendmessageMd string

// sendMessageTool tool definition
var sendMessageTool = toolcall.ToolDef{
	Name:        "send_message",
	DisplayName: "Send Message",
	Description: sendmessageMd,
	Strict:      true,
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"input": map[string]any{
				"type":        "string",
				"description": "The message content to send to dscli",
			},
			"project": map[string]any{
				"type":        "string",
				"description": "Project root directory path — must be an existing, absolute directory",
			},
		},
		"required":             []string{"input", "project"},
		"additionalProperties": false,
	},
	Category: "communication",
	Handler:  handleSendMessage,
}

func init() {
	if err := toolcall.RegisterTool(sendMessageTool); err != nil {
		panic(fmt.Sprintf("sendmessage: register tool: %v", err))
	}
}

// handleSendMessage handles the send_message tool call.
func handleSendMessage(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	span, ctx := clog.StartSpanFromContext(ctx, "handleSendMessage")
	defer span.Finish()

	input := toolcall.ToolArgsValue(args, "input", "")
	project := toolcall.ToolArgsValue(args, "project", "")

	if input == "" {
		err = fmt.Errorf("input is required")
		return result, warning, err
	}
	if project == "" {
		err = fmt.Errorf("project is required")
		return result, warning, err
	}

	// Verify project directory exists
	info, statErr := os.Stat(project)
	if statErr != nil {
		err = fmt.Errorf("project directory %q does not exist: %w", project, statErr)
		return result, warning, err
	}
	if !info.IsDir() {
		err = fmt.Errorf("project path %q is not a directory", project)
		return result, warning, err
	}

	// Get project basename for the return message
	projectName := filepath.Base(project)

	// Look up target project's maintainer name for display
	targetInfo := session.GetProjectInfo(ctx, project)
	targetName := targetInfo.MaintainerCN
	if targetName == "" {
		targetName = targetInfo.MaintainerEN
	}
	if targetName == "" {
		targetName = projectName // fallback
	}

	// Escape input for Emacs Lisp string safety.
	// The expression is: (dscli--send-message-raw "<input>" "<project>")
	// We need to escape: \ → \\, " → \", newline → \n
	input = strings.ReplaceAll(input, "\\", "\\\\")
	input = strings.ReplaceAll(input, "\"", "\\\"")
	input = strings.ReplaceAll(input, "\n", "\\n")

	// Escape project path too (defensive — paths rarely contain special chars)
	project = strings.ReplaceAll(project, "\\", "\\\\")
	project = strings.ReplaceAll(project, "\"", "\\\"")

	expr := fmt.Sprintf(`(dscli--send-message-raw "%s" "%s")`, input, project)

	outfmt.Printf("📨 发送消息至 %s ...\n", targetName)

	cmd := exec.Command("emacsclient", "--eval", expr)
	stdout, cmdErr := cmd.CombinedOutput()
	if cmdErr != nil {
		err = fmt.Errorf("emacsclient failed: %w\n%s", cmdErr, strings.TrimSpace(string(stdout)))
		outfmt.Println("❌ 发送失败:", err)
		return result, warning, err
	}

	outfmt.Printf("✅ 消息已送达 %s (项目: %s)\n", targetName, projectName)

	result = fmt.Sprintf("消息已送达 %s 在项目 %s", targetName, projectName)
	return result, warning, nil
}
