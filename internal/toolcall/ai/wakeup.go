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
	"strings"
	"time"

	"github.com/dscli/dscli/internal/chimein"
	"github.com/dscli/dscli/internal/config"
	"github.com/dscli/dscli/internal/emacsutil"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/processutil"
	"github.com/dscli/dscli/internal/roles"
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
			"role": map[string]any{
				"type": "string",
				"description": "Role of the woken session (optional). Use architect when the task is still an idea whose design needs work (the CLI default when omitted); " +
					"dev when both the idea and its implementation are clear and no architect analysis is needed; review, expert and test are also accepted. " +
					"Applied only when no session is running for the target project.",
				"enum": roles.Names(),
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
	role, err := normalizeWakeupRole(toolcall.ToolArgsValue(args, "role", ""))
	if err != nil {
		return result, warning, err
	}

	projectName, targetName, err := resolveWakeupTarget(ctx, project, ainame)
	if err != nil {
		return result, warning, err
	}

	// Step 1: queue the message in the target project's chimeins queue.
	// All projects share the global sqlite.db (~/.dscli/sqlite.db), keyed by
	// project_path in the sessions table.  The chimein is the sole content
	// delivery path — the display command (Step 3) only wakes the session.
	queueWakeupMessage(ctx, project, input, role)

	// Step 2: a running session picks the chimein up in its next round, so
	// no further action is needed.
	if processutil.IsProcessRunning(project) {
		return deliverToRunningSession(targetName, projectName, role)
	}

	// Step 3: no running process - dispatch the display command.
	return dispatchWakeup(project, projectName, targetName, role)
}

// resolveWakeupTarget validates the target project and resolves the names
// used in output: the project's base name and its maintainer's display
// name.  ainame, when provided, is validated against the project's
// assignment to catch a hallucinated name early.
func resolveWakeupTarget(ctx context.Context, project, ainame string) (projectName, targetName string, err error) {
	if project == "" {
		return "", "", fmt.Errorf("project is required")
	}
	info, statErr := os.Stat(project)
	if statErr != nil {
		return "", "", fmt.Errorf("project directory %q does not exist: %w", project, statErr)
	}
	if !info.IsDir() {
		return "", "", fmt.Errorf("project path %q is not a directory", project)
	}
	projectName = filepath.Base(project)

	targetInfo := session.GetProjectInfo(ctx, project)
	targetName = targetInfo.MaintainerCN
	if targetName == "" {
		targetName = targetInfo.MaintainerEN
	}
	if targetName == "" {
		targetName = projectName // fallback
	}

	if ainame != "" {
		if targetInfo.MaintainerCN == "" && targetInfo.MaintainerEN == "" {
			return "", "", fmt.Errorf("project %q has no maintainer assigned — cannot validate ainame %q",
				project, ainame)
		}
		if ainame != targetInfo.MaintainerCN && ainame != targetInfo.MaintainerEN {
			return "", "", fmt.Errorf("ainame %q does not match project %q's maintainer (CN: %q, EN: %q)",
				ainame, project, targetInfo.MaintainerCN, targetInfo.MaintainerEN)
		}
	}
	return projectName, targetName, nil
}

// queueWakeupMessage appends the message to the target project's chimein
// queue.  The role rides along in the tag so a session that is already
// running (and therefore cannot switch roles) still sees the sender's
// intent; a newly started session has it applied anyway.
func queueWakeupMessage(ctx context.Context, project, input, role string) {
	if input == "" {
		return
	}
	tag := fmt.Sprintf("[wakeup at %s]", time.Now().Format(time.RFC3339))
	if role != "" {
		tag = fmt.Sprintf("[wakeup at %s role=%s]", time.Now().Format(time.RFC3339), role)
	}
	// Non-fatal: a failed chimein write means the message is lost, but
	// returning an error to the caller is worse — the display command
	// still starts a session where the user can see context.
	if err := chimein.AppendToProject(ctx, project, tag+"\n"+input); err != nil {
		outfmt.Debug("wakeup: append chimein: %v\n", err)
	}
}

// deliverToRunningSession reports delivery to an already running session.
// A session's role is fixed at startup: the chimein still reaches it, but
// a requested role cannot be applied.  Say so instead of silently
// claiming success.
func deliverToRunningSession(targetName, projectName, role string) (result, warning string, err error) {
	outfmt.Printf("📨 已送达 %s (已有运行中的会话)\n", targetName)
	result = fmt.Sprintf("已送达 %s 在项目 %s（运行中会话）", targetName, projectName)
	if role != "" {
		outfmt.Printf("⚠️ 运行中的会话角色不变，role=%s 未生效\n", role)
		warning = fmt.Sprintf("role=%s 未生效：项目 %s 已有运行中的会话（角色在会话启动时固定），消息已投递", role, projectName)
	}
	return result, warning, nil
}

// dispatchWakeup starts a session via the display command: the configured
// wakeup-command when present, otherwise the auto-detected Emacs command.
// The command carries no message content: the project travels as its
// working directory and, when requested, the role as a Lisp argument; the
// started session reads the queued chimein on boot.
func dispatchWakeup(project, projectName, targetName, role string) (result, warning string, err error) {
	dispatchCmd := displayCommandFromConfig()
	configured := len(dispatchCmd) > 0
	if configured {
		// A configured command runs verbatim (no shell layer, no template
		// expansion), so there is no channel to pass the role through; the
		// session starts with the CLI default.  Report it rather than
		// splicing a role the command does not expect.
		if role != "" {
			warning = fmt.Sprintf("role=%s 未生效：wakeup-command 由配置提供，按原样执行、不接收 role", role)
		}
	} else {
		dispatchCmd = detectDisplayCommand(role)
	}
	if len(dispatchCmd) == 0 {
		// No display command available — message is queued; user must
		// manually start dscli chat to see it.
		outfmt.Printf("📨 消息已写入项目 %s 的待处理队列\n", projectName)
		return fmt.Sprintf("消息已写入项目 %s，请运行 dscli chat 查看", projectName), warning, nil
	}

	// Fire-and-forget: the command launches a visible dscli session in the
	// user's IDE (Emacs frame, terminal window, etc.).  The project path
	// travels as the command's working directory (RunCommandBackground sets
	// cmd.Dir): emacsclient -e evaluates in the daemon, but default-directory
	// there follows the client's cwd, so the Lisp side reads the target
	// project from default-directory.  No shared handoff state — concurrent
	// wakeups of different projects cannot interfere.
	go func() {
		if err := processutil.RunCommandBackground(project, dispatchCmd[0], dispatchCmd[1:]...); err != nil {
			outfmt.Debug("wakeup: display command start: %v\n", err)
		}
	}()
	roleNote := ""
	if role != "" && !configured {
		roleNote = fmt.Sprintf("（role=%s）", role)
	}
	outfmt.Printf("📨 已唤醒 %s 处理项目 %s 的任务%s\n", targetName, projectName, roleNote)
	return fmt.Sprintf("已唤醒 %s 处理项目 %s 的任务%s", targetName, projectName, roleNote), warning, nil
}

// detectDisplayCommand auto-detects the best display command based on
// tools available on the system.  The mode decision is centralized in
// emacsutil.Detect so editor and flycheck behave identically.
//
// The returned argv is passed verbatim to exec.Command — no shell layer,
// no string interpolation.  The project path never appears here: it
// travels as the command's working directory (RunCommandBackground sets
// cmd.Dir), so a crafted path can never become a shell metacharacter or
// a Lisp form.  The optional role is the one exception: it is validated
// against the built-in role names before dispatch and embedded as a Lisp
// string argument (see wakeupLispForm).  Returns nil when no display
// command is available.
func detectDisplayCommand(role string) []string {
	form := wakeupLispForm(role)
	switch emacsutil.Detect() {
	case emacsutil.ModeClientServer:
		// Emacs server (daemon) is up: attach a new frame via emacsclient.
		// The command carries no project data at all — RunCommandBackground
		// sets cmd.Dir to the target project, and emacsclient -e evaluates
		// with default-directory following the client's cwd, so
		// dscli--send-message-raw reads the project from default-directory.
		// The Emacs Lisp function starts a dscli chat that reads the
		// message from the chimeins queue on boot.
		return []string{"emacsclient", "-n", "-c", "-e", form}
	case emacsutil.ModeStandalone:
		// Standalone Emacs: start a fresh instance for this wakeup.
		// Most users run Emacs without server-mode, where emacsclient
		// cannot connect and the wakeup is silently lost; a plain
		// `emacs` invocation always works and gives each chat its own
		// frame instead of crowding a shared daemon.  The new instance's
		// default-directory is cmd.Dir — the target project.
		// --no-splash suppresses the startup splash screen: the wakeup
		// opens a working frame, not an advertisement.
		return []string{"emacs", "--no-splash", "--eval", form}
	case emacsutil.ModeClientOnly:
		// No emacs binary, but a client exists - last resort: it may
		// still reach a server started by another installation.
		return []string{"emacsclient", "-n", "-c", "-e", form}
	}
	// Future detectors (the project path still arrives as the command's
	// working directory; keep it out of argv so it is never interpreted):
	//   - VSCode:   []string{"code", "--command", "dscli.startChat"} — reads cwd
	//   - Vim/nvim: terminal-based launch
	//   - Terminal: []string{"x-terminal-emulator", "-e", "dscli", "chat"} — inherits cwd
	return nil
}

// wakeupLispForm builds the Emacs Lisp form the display command evaluates.
//
// role is validated against the built-in role names before it reaches here
// (normalizeWakeupRole), so embedding it as a Lisp string literal needs no
// escaping.  The role must travel inside the form, not via the environment:
// environment variables do not cross the emacsclient boundary into a
// running server's -e evaluation environment.
func wakeupLispForm(role string) string {
	if role == "" {
		return "(dscli--send-message-raw)"
	}
	return fmt.Sprintf("(dscli--send-message-raw nil %q)", role)
}

// normalizeWakeupRole canonicalizes the optional role argument and rejects
// unknown names.  Strict validation is deliberate: `dscli chat --role`
// falls back to the dev profile and template for an unknown role, so a
// typo would otherwise start the wrong persona instead of failing loudly.
func normalizeWakeupRole(raw string) (string, error) {
	role := strings.ToLower(strings.TrimSpace(raw))
	if role == "" {
		return "", nil
	}
	if !roles.IsValid(role) {
		return "", fmt.Errorf("invalid role %q: valid roles are %s", raw, strings.Join(roles.Names(), ", "))
	}
	return role, nil
}

// displayCommandFromConfig resolves the user-configured display command:
//
//	wakeup-command = ["emacs", "--no-splash", "--eval", "(dscli--send-message-raw)"]
//
// The array is passed verbatim to exec.Command — no shell layer, no
// template expansion.  Any other value type (or nothing) means "not
// configured" and returns nil, so the caller falls back to
// detectDisplayCommand.
//
// A configured command receives no role argument: the verbatim contract
// leaves no channel for one, so a requested role is reported as not
// applied instead of being spliced into argv.
func displayCommandFromConfig() []string {
	return config.GetStrings("wakeup-command")
}
