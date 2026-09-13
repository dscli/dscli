package lp

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dscli/dscli/internal/shellblock"
)

// shellBlockTest is a well-formed <shell> block for the loop tests.
var shellBlockTest = strings.Join([]string{
	"<shell>",
	"<script>",
	"echo hi",
	"</script>",
	"<summary>say hi</summary>",
	"<timeout>100</timeout>",
	"</shell>",
}, "\n")

// shellBlockedTest carries a script that matches the shared
// destructive-command interception (sudo).
var shellBlockedTest = strings.Join([]string{
	"<shell>",
	"<script>",
	"sudo rm -rf /",
	"</script>",
	"<summary>bad idea</summary>",
	"</shell>",
}, "\n")

// shellFinalTest reads as a final report: long prose, no block.
var shellFinalTest = strings.Repeat("Final report. ", 12) + "Done."

func TestHandleWebChatShellLoopExecutesAndFeedsBack(t *testing.T) {
	origSend, origExec := handleWebChatSend, handleWebChatExecShell
	t.Cleanup(func() { handleWebChatSend, handleWebChatExecShell = origSend, origExec })

	const convURL = "https://chat.deepseek.com/a/chat/s/convSHELL"
	var messages []string
	var sentOpts []WebChatOptions
	calls := 0
	handleWebChatSend = func(_ context.Context, msg string, opts WebChatOptions) (WebChatResult, error) {
		calls++
		messages = append(messages, msg)
		sentOpts = append(sentOpts, opts)
		return WebChatResult{Content: shellFinalTest, URL: convURL}, nil
	}
	var gotScript string
	var gotTimeout time.Duration
	handleWebChatExecShell = func(_ context.Context, _ string, script string, timeout time.Duration) (*shellblock.RunResult, error) {
		gotScript, gotTimeout = script, timeout
		return &shellblock.RunResult{Number: 65, OutputPath: "/tmp/script65.txt", ExitCode: 0, Timeout: timeout}, nil
	}

	res, err := handleWebChatShellLoop(context.Background(),
		WebChatResult{Content: shellBlockTest, URL: convURL},
		WebChatOptions{Role: "dev", ShellTool: true})
	if err != nil {
		t.Fatalf("shell loop: %v", err)
	}
	if res.Content != shellFinalTest {
		t.Errorf("content = %q, want the final report", res.Content)
	}
	if !res.Printed {
		t.Error("loop result must be marked Printed")
	}
	if gotScript != "echo hi\n" {
		t.Errorf("executed script = %q, want the verbatim body", gotScript)
	}
	if gotTimeout != 100*time.Second {
		t.Errorf("timeout = %v, want 100s from <timeout>", gotTimeout)
	}
	if calls != 1 {
		t.Fatalf("sends = %d, want 1 (feedback only; the block arrived as the first reply)", calls)
	}
	if messages[0] != "output of script65.sh (attached as script65.txt):" {
		t.Errorf("feedback message = %q", messages[0])
	}
	if len(sentOpts[0].Attachments) != 1 || sentOpts[0].Attachments[0] != "/tmp/script65.txt" {
		t.Errorf("feedback attachments = %v, want [/tmp/script65.txt]", sentOpts[0].Attachments)
	}
	if sentOpts[0].Keep != convURL {
		t.Errorf("feedback Keep = %q, want the conversation URL", sentOpts[0].Keep)
	}
	if sentOpts[0].ShellTool {
		t.Error("transport options must not carry ShellTool")
	}
}

func TestHandleWebChatShellLoopWarnsThenExecutes(t *testing.T) {
	origSend, origExec := handleWebChatSend, handleWebChatExecShell
	t.Cleanup(func() { handleWebChatSend, handleWebChatExecShell = origSend, origExec })

	const convURL = "https://chat.deepseek.com/a/chat/s/convSHELL"
	var messages []string
	calls := 0
	handleWebChatSend = func(_ context.Context, msg string, _ WebChatOptions) (WebChatResult, error) {
		calls++
		messages = append(messages, msg)
		switch calls {
		case 1:
			// The warning's answer: the model re-sends properly this time.
			return WebChatResult{Content: shellBlockTest, URL: convURL}, nil
		default:
			return WebChatResult{Content: shellFinalTest, URL: convURL}, nil
		}
	}
	executed := false
	handleWebChatExecShell = func(_ context.Context, _ string, _ string, _ time.Duration) (*shellblock.RunResult, error) {
		executed = true
		return &shellblock.RunResult{Number: 1, OutputPath: "/tmp/script1.txt", ExitCode: 0}, nil
	}

	res, err := handleWebChatShellLoop(context.Background(),
		WebChatResult{Content: "Sure, starting now.", URL: convURL},
		WebChatOptions{Role: "dev", ShellTool: true})
	if err != nil {
		t.Fatalf("shell loop: %v", err)
	}
	if !strings.Contains(messages[0], "neither contains a `<shell>` block") {
		t.Errorf("warning message = %q, want the stalled-round warning", messages[0])
	}
	if !executed {
		t.Error("the block after the warning must be executed")
	}
	if calls != 2 {
		t.Errorf("sends = %d, want 2 (warning + feedback)", calls)
	}
	if res.Content != shellFinalTest {
		t.Errorf("content = %q, want the final report", res.Content)
	}
}

func TestHandleWebChatShellLoopConsecutiveWarnsAbort(t *testing.T) {
	origSend, origWarns := handleWebChatSend, handleWebChatMaxShellWarns
	t.Cleanup(func() { handleWebChatSend, handleWebChatMaxShellWarns = origSend, origWarns })
	handleWebChatMaxShellWarns = 2

	const convURL = "https://chat.deepseek.com/a/chat/s/convSHELL"
	calls := 0
	handleWebChatSend = func(_ context.Context, _ string, _ WebChatOptions) (WebChatResult, error) {
		calls++
		return WebChatResult{Content: "Sure, starting now.", URL: convURL}, nil
	}

	_, err := handleWebChatShellLoop(context.Background(),
		WebChatResult{Content: "Sure, starting now.", URL: convURL},
		WebChatOptions{Role: "dev", ShellTool: true})
	if err == nil || !strings.Contains(err.Error(), "consecutive warnings") {
		t.Fatalf("err = %v, want the consecutive-warnings abort", err)
	}
	if calls != 2 {
		t.Errorf("sends = %d, want 2 (the third warning would exceed the cap)", calls)
	}
}

func TestHandleWebChatShellLoopBlockedRefusal(t *testing.T) {
	origSend, origExec := handleWebChatSend, handleWebChatExecShell
	t.Cleanup(func() { handleWebChatSend, handleWebChatExecShell = origSend, origExec })

	const convURL = "https://chat.deepseek.com/a/chat/s/convSHELL"
	var messages []string
	calls := 0
	handleWebChatSend = func(_ context.Context, msg string, _ WebChatOptions) (WebChatResult, error) {
		calls++
		messages = append(messages, msg)
		return WebChatResult{Content: shellFinalTest, URL: convURL}, nil
	}
	executed := false
	handleWebChatExecShell = func(_ context.Context, _ string, _ string, _ time.Duration) (*shellblock.RunResult, error) {
		executed = true
		return &shellblock.RunResult{}, nil
	}

	res, err := handleWebChatShellLoop(context.Background(),
		WebChatResult{Content: shellBlockedTest, URL: convURL},
		WebChatOptions{Role: "dev", ShellTool: true})
	if err != nil {
		t.Fatalf("shell loop: %v", err)
	}
	if executed {
		t.Error("a blocked script must never be executed")
	}
	if !strings.Contains(messages[0], "NOT executed") {
		t.Errorf("refusal message = %q, want the blocked-command refusal", messages[0])
	}
	if calls != 1 {
		t.Errorf("sends = %d, want 1 (refusal only)", calls)
	}
	if res.Content != shellFinalTest {
		t.Errorf("content = %q, want the final report", res.Content)
	}
}

func TestHandleWebChatShellToolProseNotRouted(t *testing.T) {
	origSend, origExec := handleWebChatSend, handleWebChatExecShell
	t.Cleanup(func() { handleWebChatSend, handleWebChatExecShell = origSend, origExec })

	calls := 0
	handleWebChatSend = func(_ context.Context, _ string, _ WebChatOptions) (WebChatResult, error) {
		calls++
		return WebChatResult{Content: shellFinalTest, URL: "https://chat.deepseek.com/a/chat/s/convX"}, nil
	}
	handleWebChatExecShell = func(_ context.Context, _ string, _ string, _ time.Duration) (*shellblock.RunResult, error) {
		t.Error("a prose reply must not execute anything")
		return &shellblock.RunResult{}, nil
	}

	res, err := HandleWebChat(context.Background(), "do it", WebChatOptions{Role: "dev", ShellTool: true, SkipPromptInjection: true})
	if err != nil {
		t.Fatalf("HandleWebChat: %v", err)
	}
	if res.Content != shellFinalTest {
		t.Errorf("content = %q, want the prose verbatim", res.Content)
	}
	if res.Printed {
		t.Error("a prose reply never enters the loop, so Printed must be false")
	}
	if calls != 1 {
		t.Errorf("sends = %d, want 1", calls)
	}
}

func TestHandleWebChatShellToolRoutesBlockIntoLoop(t *testing.T) {
	origSend, origExec := handleWebChatSend, handleWebChatExecShell
	t.Cleanup(func() { handleWebChatSend, handleWebChatExecShell = origSend, origExec })

	const convURL = "https://chat.deepseek.com/a/chat/s/convSHELL"
	calls := 0
	handleWebChatSend = func(_ context.Context, _ string, _ WebChatOptions) (WebChatResult, error) {
		calls++
		if calls == 1 {
			return WebChatResult{Content: shellBlockTest, URL: convURL}, nil
		}
		return WebChatResult{Content: shellFinalTest, URL: convURL}, nil
	}
	executed := false
	handleWebChatExecShell = func(_ context.Context, _ string, _ string, _ time.Duration) (*shellblock.RunResult, error) {
		executed = true
		return &shellblock.RunResult{Number: 1, OutputPath: "/tmp/script1.txt", ExitCode: 0}, nil
	}

	res, err := HandleWebChat(context.Background(), "do it", WebChatOptions{Role: "dev", ShellTool: true, SkipPromptInjection: true})
	if err != nil {
		t.Fatalf("HandleWebChat: %v", err)
	}
	if !executed {
		t.Error("the first-round block must route into the shell loop and execute")
	}
	if !res.Printed || res.Content != shellFinalTest {
		t.Errorf("res = %+v, want the printed final report", res)
	}
	if calls != 2 {
		t.Errorf("sends = %d, want 2 (initial + feedback)", calls)
	}
}

func TestHandleWebChatShellLoopContinueWarningVariant(t *testing.T) {
	origSend, origExec, origDelays := handleWebChatSend, handleWebChatExecShell, handleWebChatRetryDelays
	t.Cleanup(func() {
		handleWebChatSend, handleWebChatExecShell, handleWebChatRetryDelays = origSend, origExec, origDelays
	})
	handleWebChatRetryDelays = []time.Duration{time.Millisecond}

	const convURL = "https://chat.deepseek.com/a/chat/s/convSHELL"
	var messages []string
	calls := 0
	handleWebChatSend = func(_ context.Context, msg string, _ WebChatOptions) (WebChatResult, error) {
		calls++
		messages = append(messages, msg)
		switch calls {
		case 1:
			return WebChatResult{Content: shellBlockTest, URL: convURL}, nil
		case 2:
			// The feedback send's reply comes back truncated.
			return WebChatResult{}, ErrTruncated
		default:
			if msg != webChatContinueWarningShell {
				t.Errorf("retry message = %q, want the shell continue warning", msg)
			}
			return WebChatResult{Content: shellFinalTest, URL: convURL}, nil
		}
	}
	handleWebChatExecShell = func(_ context.Context, _ string, _ string, _ time.Duration) (*shellblock.RunResult, error) {
		return &shellblock.RunResult{Number: 1, OutputPath: "/tmp/script1.txt", ExitCode: 0}, nil
	}

	res, err := handleWebChatShellLoop(context.Background(),
		WebChatResult{Content: shellBlockTest, URL: convURL},
		WebChatOptions{Role: "dev", ShellTool: true})
	if err != nil {
		t.Fatalf("shell loop: %v", err)
	}
	if calls != 3 {
		t.Fatalf("sends = %d, want 3 (feedback, truncated, retry nudge)", calls)
	}
	if !strings.Contains(messages[2], "`<shell>` block") {
		t.Errorf("continue warning = %q, want the <shell> variant", messages[2])
	}
	if res.Content != shellFinalTest {
		t.Errorf("content = %q, want the final report", res.Content)
	}
}

func TestHandleWebChatResumeShellPendingBlock(t *testing.T) {
	origSend, origExec := handleWebChatSend, handleWebChatExecShell
	t.Cleanup(func() { handleWebChatSend, handleWebChatExecShell = origSend, origExec })

	const convURL = "https://chat.deepseek.com/a/chat/s/convSHELL"
	calls := 0
	handleWebChatSend = func(_ context.Context, _ string, _ WebChatOptions) (WebChatResult, error) {
		calls++
		return WebChatResult{Content: shellFinalTest, URL: convURL}, nil
	}
	executed := false
	handleWebChatExecShell = func(_ context.Context, _ string, _ string, _ time.Duration) (*shellblock.RunResult, error) {
		executed = true
		return &shellblock.RunResult{Number: 1, OutputPath: "/tmp/script1.txt", ExitCode: 0}, nil
	}
	mockResumeResolve(t, convURL)
	mockResumeRead(t, shellBlockTest, "FINISHED", true)

	res, err := HandleWebChatResume(context.Background(), WebChatOptions{Keep: "convSHELL", Role: "dev", ShellTool: true})
	if err != nil {
		t.Fatalf("HandleWebChatResume: %v", err)
	}
	if !executed {
		t.Error("the pending block must execute on resume")
	}
	if !res.Printed || res.Content != shellFinalTest {
		t.Errorf("res = %+v, want the printed final report", res)
	}
	if calls != 1 {
		t.Errorf("sends = %d, want 1 (feedback only)", calls)
	}
}

func TestHandleWebChatResumeShellMultiTurn(t *testing.T) {
	origSend := handleWebChatSend
	t.Cleanup(func() { handleWebChatSend = origSend })
	handleWebChatSend = func(_ context.Context, _ string, _ WebChatOptions) (WebChatResult, error) {
		t.Fatal("a multi-turn shell resume must not send anything")
		return WebChatResult{}, nil
	}
	mockResumeResolve(t, "https://chat.deepseek.com/a/chat/s/convSHELL")
	mockResumeRead(t, shellFinalTest, "FINISHED", true)

	res, err := HandleWebChatResume(context.Background(), WebChatOptions{Keep: "convSHELL", Role: "dev", ShellTool: true})
	if err != nil {
		t.Fatalf("HandleWebChatResume: %v", err)
	}
	if res.Content != shellFinalTest {
		t.Errorf("content = %q, want the last message verbatim", res.Content)
	}
	if res.Printed {
		t.Error("a multi-turn resume must not be marked Printed")
	}
}

// residueAfterBlock is a well-formed block followed by badge-rendered DSML
// marker residue: the shape the site stores after it badges the markup.
func residueAfterBlock() string {
	lt := string(rune(60))
	gt := string(rune(62))
	bar := string(rune(0xFF5C))
	residue := lt + "/" + bar + bar + "DSML" + bar + bar + "invoke" + gt
	return shellBlockTest + "\n" + residue
}

// TestHandleWebChatShellLoopResidueReminder pins the residue feedback: an
// executed block whose reply carries DSML marker residue OUTSIDE the block
// still runs, and the round's feedback message gets ResidueNote appended; a
// clean round's feedback must NOT carry it.
func TestHandleWebChatShellLoopResidueReminder(t *testing.T) {
	tests := []struct {
		name        string
		first       string
		wantResidue bool
	}{
		{name: "residue after the block", first: residueAfterBlock(), wantResidue: true},
		{name: "clean block", first: shellBlockTest, wantResidue: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origSend, origExec := handleWebChatSend, handleWebChatExecShell
			t.Cleanup(func() { handleWebChatSend, handleWebChatExecShell = origSend, origExec })

			const convURL = "https://chat.deepseek.com/a/chat/s/convSHELL"
			var messages []string
			handleWebChatSend = func(_ context.Context, msg string, _ WebChatOptions) (WebChatResult, error) {
				messages = append(messages, msg)
				return WebChatResult{Content: shellFinalTest, URL: convURL}, nil
			}
			handleWebChatExecShell = func(_ context.Context, _ string, _ string, timeout time.Duration) (*shellblock.RunResult, error) {
				return &shellblock.RunResult{Number: 7, OutputPath: "/tmp/script7.txt", ExitCode: 0, Timeout: timeout}, nil
			}

			if _, err := handleWebChatShellLoop(context.Background(),
				WebChatResult{Content: tt.first, URL: convURL},
				WebChatOptions{Role: "dev", ShellTool: true}); err != nil {
				t.Fatalf("shell loop: %v", err)
			}
			if len(messages) != 1 {
				t.Fatalf("sends = %d, want 1 (the feedback)", len(messages))
			}
			feedback := messages[0]
			// The existing prefix assertion must keep holding: the note rides
			// after a blank line.
			if !strings.HasPrefix(feedback, "output of script7.sh (attached as script7.txt):") {
				t.Errorf("feedback = %q, want the script prefix preserved", feedback)
			}
			hasNote := strings.Contains(feedback, shellblock.ResidueNote())
			if hasNote != tt.wantResidue {
				t.Errorf("feedback carries ResidueNote = %v, want %v:\n%s", hasNote, tt.wantResidue, feedback)
			}
			if tt.wantResidue && !strings.Contains(feedback, "\n\n") {
				t.Errorf("residue note must ride after a blank line:\n%s", feedback)
			}
		})
	}
}

// captureStderr swaps os.Stderr for a pipe and returns a func that restores
// it and yields everything written. The writes in the shell loop are tiny,
// so a single ReadAll after closing the writer cannot deadlock.
func captureStderr(t *testing.T) func() string {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	return func() string {
		os.Stderr = orig
		_ = w.Close()
		b, _ := io.ReadAll(r)
		_ = r.Close()
		return string(b)
	}
}

// finalResidueReport builds a long block-less reply (an ActionFinal report)
// followed by a plain ASCII DSML close tag. The close tag is built from rune
// helpers, and it is deliberately a PLAIN close (no fullwidth bars, no
// <invoke): hasDSMLCallShape stays false, so the route really is ActionFinal
// and only the residue stderr note may fire.
func finalResidueReport() string {
	lt := string(rune(60))
	gt := string(rune(62))
	return shellFinalTest + "\n" + lt + "/invoke" + gt
}

// TestHandleWebChatShellLoopFinalResidueNote pins the ActionFinal residue
// stderr branch: a final report that carries DSML marker residue prints the
// note but is NOT routed into another round.
func TestHandleWebChatShellLoopFinalResidueNote(t *testing.T) {
	tests := []struct {
		name        string
		first       string
		wantResidue bool
	}{
		{name: "final report with residue", first: finalResidueReport(), wantResidue: true},
		{name: "clean final report", first: shellFinalTest, wantResidue: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origSend := handleWebChatSend
			t.Cleanup(func() { handleWebChatSend = origSend })

			sends := 0
			handleWebChatSend = func(_ context.Context, _ string, _ WebChatOptions) (WebChatResult, error) {
				sends++
				return WebChatResult{Content: shellFinalTest}, nil
			}

			drain := captureStderr(t)
			res, err := handleWebChatShellLoop(context.Background(),
				WebChatResult{Content: tt.first},
				WebChatOptions{Role: "dev", ShellTool: true})
			stderr := drain()
			if err != nil {
				t.Fatalf("shell loop: %v", err)
			}
			// The final route must NOT send a follow-up round.
			if sends != 0 {
				t.Errorf("sends = %d, want 0 (a final report must not be re-sent)", sends)
			}
			if !res.Printed {
				t.Error("the final result must be marked Printed")
			}
			// The loop always writes the "will run locally" line first, so
			// assert substrings, never equality.
			hasNote := strings.Contains(stderr, "最终回复携带 DSML 标记残留")
			if hasNote != tt.wantResidue {
				t.Errorf("stderr carries the final-residue note = %v, want %v:\n%s", hasNote, tt.wantResidue, stderr)
			}
		})
	}
}
