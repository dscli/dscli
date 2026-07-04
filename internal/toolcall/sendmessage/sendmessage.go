// Package sendmessage implements the send_message tool.
//
// send_message dispatches a message to a dscli chat session for a given
// project.  The tool is IDE-agnostic: it writes the message to the target
// project's chimeins queue and dispatches via a configurable display
// command when no dscli process is running for that project.
//
// The call is fire-and-forget: the message is queued and control returns
// to the calling AI immediately.  The recipient AI processes the message
// independently in its own session context.
package sendmessage

import (
	_ "embed"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/dscli/dscli/internal/config"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/session"
	"github.com/dscli/dscli/internal/sqlite"
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

	// Step 1: Write message to target project's chimeins queue.
	// All projects share the global sqlite.db (~/.dscli/sqlite.db), keyed by
	// project_path in the sessions table.  The chimein is the sole content
	// delivery path — the display command (Step 3) only wakes the session.
	if err := writeChimein(ctx, project, input); err != nil {
		// Non-fatal: a failed chimein write means the message is lost, but
		// returning an error to the caller is worse — the display command
		// still starts a session where the user can see context.
		outfmt.Debug("sendmessage: write chimein: %v\n", err)
	}

	// Step 2: Check if a dscli chat process is already running for the
	// target project.  If so, the existing session will pick up the
	// chimein in its next round — no further action needed.
	if isProcessRunning(project) {
		outfmt.Printf("📨 消息已送达 %s (已有运行中的会话)\n", targetName)
		result = fmt.Sprintf("消息已送达 %s 在项目 %s（运行中会话）", targetName, projectName)
		return result, warning, nil
	}

	// Step 3: No running process — dispatch via configured display command.
	// The display command carries ONLY the project path.  The message content
	// is already in the chimeins queue — the started session reads it on boot.
	dispatchCmd := config.Get("send-message.command", "")
	if dispatchCmd == "" {
		dispatchCmd = detectDisplayCommand()
	}
	if dispatchCmd != "" {
		// Fire-and-forget: the command launches a visible dscli session
		// in the user's IDE (Emacs frame, terminal window, etc.).
		go runDisplayCommand(dispatchCmd, project)
		outfmt.Printf("📨 已唤醒 %s 处理项目 %s 的任务\n", targetName, projectName)
		result = fmt.Sprintf("已唤醒 %s 处理项目 %s 的任务", targetName, projectName)
	} else {
		// No display command available — message is queued; user must
		// manually start dscli chat to see it.
		outfmt.Printf("📨 消息已写入项目 %s 的待处理队列\n", projectName)
		result = fmt.Sprintf("消息已写入项目 %s，请运行 dscli chat 查看", projectName)
	}

	return result, warning, nil
}

// writeChimein writes the input message to the target project's chimeins
// table entry via the global database.  The target project's dscli chat
// session (existing or next) picks it up automatically.
func writeChimein(ctx context.Context, projectPath, content string) error {
	db, err := sqlite.OpenDB(ctx)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close(ctx)

	// Find or create the target project's session.
	var sessionID int64
	err = db.QueryRow(
		`SELECT id FROM sessions WHERE project_path = ?`, projectPath,
	).Scan(&sessionID)
	if err != nil {
		// Session doesn't exist yet — create one.
		res, insErr := db.Exec(
			`INSERT INTO sessions (project_path) VALUES (?)`, projectPath,
		)
		if insErr != nil {
			return fmt.Errorf("create session: %w", insErr)
		}
		sessionID, err = res.LastInsertId()
		if err != nil {
			return fmt.Errorf("last insert id: %w", err)
		}
	}

	marked := fmt.Sprintf("\n[send_message at %s]\n%s\n",
		time.Now().Format(time.RFC3339), strings.TrimSpace(content))

	// UPSERT: one chimein row per session (UNIQUE constraint on session_id),
	// content is appended so multiple messages accumulate.
	_, err = db.Exec(`
		INSERT INTO chimeins (session_id, content) VALUES (?, ?)
		ON CONFLICT(session_id) DO UPDATE SET
			content = content || ?`,
		sessionID, marked, marked)
	if err != nil {
		return fmt.Errorf("upsert chimein: %w", err)
	}

	return nil
}

// isProcessRunning checks whether a dscli chat process is currently holding
// the project-level lockfile at <project>/.dscli/locks/dscli.lock.
func isProcessRunning(projectPath string) bool {
	lockPath := filepath.Join(projectPath, ".dscli", "locks", "dscli.lock")
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return false
	}

	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil || pid == 0 {
		return false
	}

	// On Unix, sending signal 0 is a no-op that checks process existence.
	return syscall.Kill(pid, syscall.Signal(0)) == nil
}

// detectDisplayCommand auto-detects the best display command based on
// tools available on the system.
func detectDisplayCommand() string {
	if hasExecutable("emacsclient") {
		// Emacs: create frame (-c), return immediately (-n).
		// The template has one %s placeholder — the project path.
		// dscli--send-message-raw starts a dscli chat that reads
		// the message from the chimeins queue on boot.
		return `emacsclient -n -c -e '(dscli--send-message-raw "%s")'`
	}
	// Future detectors:
	//   - VSCode: `code --command "dscli.startChat" --args "..."`
	//   - Vim/nvim:  terminal-based launch
	//   - Terminal: `x-terminal-emulator -e sh -c 'cd %s && dscli chat'`
	return ""
}

// hasExecutable checks if a command is available in PATH.
func hasExecutable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// runDisplayCommand executes the display command template with the given
// project path, fire-and-forget.  The template receives a single %s
// placeholder for the project path (shell-escaped).
//
// The display command does NOT carry message content — the message is
// already in the chimeins queue.  The command's sole job is to wake up
// a dscli chat session in the user's IDE; the session reads the chimein
// on startup.
func runDisplayCommand(tmpl, project string) {
	// Escape for safe interpolation into shell command.
	project = strings.ReplaceAll(project, `\`, `\\`)
	project = strings.ReplaceAll(project, `"`, `\"`)

	cmdStr := fmt.Sprintf(tmpl, project)
	cmd := exec.Command("sh", "-c", cmdStr)

	if startErr := cmd.Start(); startErr != nil {
		outfmt.Debug("sendmessage: display command start: %v\n", startErr)
		return
	}

	// Detach: reap the child in background.
	go cmd.Wait()
}
