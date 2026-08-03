// wakeup dispatches a message (optional) to another AI maintainer at a given
// project, waking them up if they are not already running.  The tool is
// IDE-agnostic: it writes the message to the target project's chimeins queue
// and dispatches via a configurable display command when no dscli process is
// running for that project.
//
// The call is fire-and-forget: the message is queued and control returns
// to the calling AI immediately.  The recipient AI processes the message
// independently in its own session context.
//
// Renamed from send_message (v0.1.x) to wakeup (v0.2+).
package ai

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dscli/dscli/internal/chimein"
	"github.com/dscli/dscli/internal/config"
	"github.com/dscli/dscli/internal/emacsutil"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/processutil"
	"github.com/dscli/dscli/internal/session"
	"github.com/dscli/dscli/internal/toolcall"
	"github.com/nanjj/clog"
)

//go:embed wakeup.md
var wakeupMd string

// wakeupTool tool definition
var wakeupTool = toolcall.ToolDef{
	Name:        "wakeup",
	DisplayName: "Wake Up AI",
	Description: wakeupMd,
	Strict:      true,
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"project": map[string]any{
				"type":        "string",
				"description": "Project root directory path — must be an existing, absolute directory",
			},
			"input": map[string]any{
				"type":        "string",
				"description": "The message content to send (optional — may be empty if you have already communicated via mail)",
			},
			"ainame": map[string]any{
				"type":        "string",
				"description": "The AI maintainer name at the target project (optional — validated against project assignment if provided)",
			},
		},
		"required":             []string{"project"},
		"additionalProperties": false,
	},
	Category: "ai",
	Handler:  handleWakeup,
}

func init() {
	if err := toolcall.RegisterTool(wakeupTool); err != nil {
		panic(fmt.Sprintf("wakeup: register tool: %v", err))
	}
}

// handleWakeup handles the wakeup tool call.
func handleWakeup(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	span, ctx := clog.StartSpanFromContext(ctx, "handleWakeup")
	defer span.Finish()

	project := toolcall.ToolArgsValue(args, "project", "")
	input := toolcall.ToolArgsValue(args, "input", "")
	ainame := toolcall.ToolArgsValue(args, "ainame", "")

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

	// Look up target project's maintainer information
	targetInfo := session.GetProjectInfo(ctx, project)
	targetName := targetInfo.MaintainerCN
	if targetName == "" {
		targetName = targetInfo.MaintainerEN
	}
	if targetName == "" {
		targetName = projectName // fallback
	}

	// Validate ainame if provided — catches LLM hallucination early.
	if ainame != "" {
		if targetInfo.MaintainerCN == "" && targetInfo.MaintainerEN == "" {
			err = fmt.Errorf("project %q has no maintainer assigned — cannot validate ainame %q",
				project, ainame)
			return result, warning, err
		}
		if ainame != targetInfo.MaintainerCN && ainame != targetInfo.MaintainerEN {
			err = fmt.Errorf("ainame %q does not match project %q's maintainer (CN: %q, EN: %q)",
				ainame, project, targetInfo.MaintainerCN, targetInfo.MaintainerEN)
			return result, warning, err
		}
	}

	// Step 1: Write message to target project's chimeins queue (if input provided).
	// All projects share the global sqlite.db (~/.dscli/sqlite.db), keyed by
	// project_path in the sessions table.  The chimein is the sole content
	// delivery path — the display command (Step 3) only wakes the session.
	if input != "" {
		marked := fmt.Sprintf("[wakeup at %s]\n%s",
			time.Now().Format(time.RFC3339), input)
		if chimeinErr := chimein.AppendToProject(ctx, project, marked); chimeinErr != nil {
			// Non-fatal: a failed chimein write means the message is lost, but
			// returning an error to the caller is worse — the display command
			// still starts a session where the user can see context.
			outfmt.Debug("wakeup: append chimein: %v\n", chimeinErr)
		}
	}

	// Step 2: Check if a dscli chat process is already running for the
	// target project.  If so, the existing session will pick up the
	// chimein in its next round — no further action needed.
	if processutil.IsProcessRunning(project) {
		outfmt.Printf("📨 已送达 %s (已有运行中的会话)\n", targetName)
		result = fmt.Sprintf("已送达 %s 在项目 %s（运行中会话）", targetName, projectName)
		return result, warning, nil
	}

	// Step 3: No running process — dispatch via configured display command.
	// The display command carries ONLY the project path.  The message content
	// is already in the chimeins queue — the started session reads it on boot.
	dispatchCmd := displayCommandFromConfig()
	if len(dispatchCmd) == 0 {
		dispatchCmd = detectDisplayCommand()
	}
	if len(dispatchCmd) > 0 {
		// Fire-and-forget: the command launches a visible dscli session
		// in the user's IDE (Emacs frame, terminal window, etc.).  The
		// project path travels as the command's working directory
		// (RunCommandBackground sets cmd.Dir): emacsclient -e evaluates in
		// the daemon, but default-directory there follows the client's
		// cwd, so the Lisp side reads the target project from
		// default-directory.  No shared handoff state — concurrent
		// wakeups of different projects cannot interfere.
		go func() {
			if err := processutil.RunCommandBackground(project, dispatchCmd[0], dispatchCmd[1:]...); err != nil {
				outfmt.Debug("wakeup: display command start: %v\n", err)
			}
		}()
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

// detectDisplayCommand auto-detects the best display command based on
// tools available on the system.  The mode decision is centralized in
// emacsutil.Detect so editor and flycheck behave identically.
//
// The returned argv is passed verbatim to exec.Command — no shell layer,
// no string interpolation.  The project path never appears here: it
// travels as the command's working directory (RunCommandBackground sets
// cmd.Dir), so a crafted path can never become a shell metacharacter or
// a Lisp form.  Returns nil when no display command is available.
func detectDisplayCommand() []string {
	switch emacsutil.Detect() {
	case emacsutil.ModeClientServer:
		// Emacs server (daemon) is up: attach a new frame via emacsclient.
		// The command carries no project data at all — RunCommandBackground
		// sets cmd.Dir to the target project, and emacsclient -e evaluates
		// with default-directory following the client's cwd, so
		// dscli--send-message-raw reads the project from default-directory.
		// The Emacs Lisp function starts a dscli chat that reads the
		// message from the chimeins queue on boot.
		return []string{"emacsclient", "-n", "-c", "-e", "(dscli--send-message-raw)"}
	case emacsutil.ModeStandalone:
		// Standalone Emacs: start a fresh instance for this wakeup.
		// Most users run Emacs without server-mode, where emacsclient
		// cannot connect and the wakeup is silently lost; a plain
		// `emacs` invocation always works and gives each chat its own
		// frame instead of crowding a shared daemon.  The new instance's
		// default-directory is cmd.Dir — the target project.
		return []string{"emacs", "--eval", "(dscli--send-message-raw)"}
	case emacsutil.ModeClientOnly:
		// No emacs binary, but a client exists - last resort: it may
		// still reach a server started by another installation.
		return []string{"emacsclient", "-n", "-c", "-e", "(dscli--send-message-raw)"}
	}
	// Future detectors (the project path still arrives as the command's
	// working directory; keep it out of argv so it is never interpreted):
	//   - VSCode:   []string{"code", "--command", "dscli.startChat"} — reads cwd
	//   - Vim/nvim: terminal-based launch
	//   - Terminal: []string{"x-terminal-emulator", "-e", "dscli", "chat"} — inherits cwd
	return nil
}

// displayCommandFromConfig resolves the user-configured display command:
//
//	wakeup-command = ["emacs", "--eval", "(dscli--send-message-raw)"]
//
// The array is passed verbatim to exec.Command — no shell layer, no
// template expansion.  Any other value type (or nothing) means "not
// configured" and returns nil, so the caller falls back to
// detectDisplayCommand.
func displayCommandFromConfig() []string {
	return config.GetStrings("wakeup-command")
}
