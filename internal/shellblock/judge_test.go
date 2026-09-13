package shellblock

import (
	"strings"
	"testing"
	"time"
)

// wellFormed is the canonical block used across the judge tests.
var wellFormed = strings.Join([]string{
	"<shell>",
	"<script>",
	"#!/bin/bash",
	"echo hi",
	"</script>",
	"<summary>say hi</summary>",
	"<timeout>100</timeout>",
	"</shell>",
}, "\n")

// longFinal is a block-less reply long enough to read as a final report.
var longFinal = strings.Repeat("The task is complete. ", 8)

func TestJudge(t *testing.T) {
	tests := []struct {
		name        string
		reasoning   string
		content     string
		want        Action
		wantScript  string
		wantSummary string
		wantTimeout time.Duration
		wantIssue   string
	}{
		{
			name: "well-formed block", content: wellFormed,
			want: ActionExecute, wantScript: "#!/bin/bash\necho hi\n", wantSummary: "say hi", wantTimeout: 100 * time.Second,
		},
		{
			name:    "minimal block uses defaults",
			content: strings.Join([]string{"<shell>", "<script>", "ls", "</script>", "</shell>"}, "\n"),
			want:    ActionExecute, wantScript: "ls\n", wantSummary: "", wantTimeout: DefaultTimeout,
		},
		{
			name: "prose around the block", content: "Here you go:\n" + wellFormed + "\nAll done later.",
			want: ActionExecute, wantScript: "#!/bin/bash\necho hi\n", wantSummary: "say hi", wantTimeout: 100 * time.Second,
		},
		{
			name:    "trailing whitespace on tags tolerated",
			content: strings.Replace(wellFormed, "<shell>", "<shell>   ", 1),
			want:    ActionExecute, wantScript: "#!/bin/bash\necho hi\n", wantSummary: "say hi", wantTimeout: 100 * time.Second,
		},
		{
			name:    "invalid timeout falls back to default",
			content: strings.Replace(wellFormed, "<timeout>100</timeout>", "<timeout>abc</timeout>", 1),
			want:    ActionExecute, wantScript: "#!/bin/bash\necho hi\n", wantSummary: "say hi", wantTimeout: DefaultTimeout,
		},
		{
			name:    "timeout over cap is clamped",
			content: strings.Replace(wellFormed, "<timeout>100</timeout>", "<timeout>99999</timeout>", 1),
			want:    ActionExecute, wantScript: "#!/bin/bash\necho hi\n", wantSummary: "say hi", wantTimeout: MaxTimeout,
		},
		{
			name: "fence and heredoc inside the body are verbatim",
			content: strings.Join([]string{
				"<shell>", "<script>",
				"cat <<EOF", "hello", "EOF", "```", "code", "```", "# <script.sh>",
				"</script>", "<summary>body</summary>", "</shell>",
			}, "\n"),
			want:        ActionExecute,
			wantScript:  "cat <<EOF\nhello\nEOF\n```\ncode\n```\n# <script.sh>\n",
			wantSummary: "body", wantTimeout: DefaultTimeout,
		},
		{
			name: "missing </script>", content: "<shell>\n<script>\necho x\n</shell>",
			want: ActionWarn, wantIssue: "the `</script>` line is missing",
		},
		{
			name: "missing </shell>", content: "<shell>\n<script>\necho x\n</script>",
			want: ActionWarn, wantIssue: "the `</shell>` line is missing",
		},
		{
			name: "missing <shell>", content: "<script>\necho x\n</script>\n</shell>",
			want: ActionWarn, wantIssue: "the `<shell>` line is missing",
		},
		{
			name: "missing <script>", content: "<shell>\necho x\n</script>\n</shell>",
			want: ActionWarn, wantIssue: "the `<script>` line is missing",
		},
		{
			name: "stacked open tags are tolerated", content: "<shell>\n<shell>\n<script>\necho x\n</script>\n</shell>",
			want: ActionExecute, wantScript: "echo x\n", wantSummary: "", wantTimeout: DefaultTimeout,
		},
		{
			name:    "second full block refused",
			content: "<shell>\n<script>\necho a\n</script>\n</shell>\n<shell>\n<script>\necho b\n</script>\n</shell>",
			want:    ActionWarn, wantIssue: "more than once",
		},
		{
			name: "body may contain literal tag lines",
			content: strings.Join([]string{
				"<shell>", "<script>",
				"cat > example.txt <<EOF", "<shell>", "<script>", "EOF",
				"</script>", "<summary>write example</summary>", "</shell>",
			}, "\n"),
			want:        ActionExecute,
			wantScript:  "cat > example.txt <<EOF\n<shell>\n<script>\nEOF\n",
			wantSummary: "write example", wantTimeout: DefaultTimeout,
		},
		{
			name: "literal </script> in the body ends it",
			content: strings.Join([]string{
				"<shell>", "<script>",
				"cat > x <<EOF", "</script>", "EOF",
				"</script>", "<summary>cut</summary>", "</shell>",
			}, "\n"),
			// The protocol's one hard boundary: the first whole-line
			// </script> terminates the body, so the prefix is what runs.
			want:        ActionExecute,
			wantScript:  "cat > x <<EOF\n",
			wantSummary: "cut", wantTimeout: DefaultTimeout,
		},
		{
			name: "out of order", content: "<script>\n<shell>\necho x\n</script>\n</shell>",
			want: ActionWarn, wantIssue: "out of order",
		},
		{
			name: "empty body", content: "<shell>\n<script>\n\n</script>\n</shell>",
			want: ActionWarn, wantIssue: "body is empty",
		},
		{
			name:    "indented tag lines only",
			content: "  <shell>\n  <script>\n  echo x\n  </script>\n  </shell>",
			want:    ActionWarn, wantIssue: "indented",
		},
		{
			name:    "badge-rendered tag lines only",
			content: "\uff5c<shell>\uff5c\n\uff5c<script>\uff5c\necho x\n\uff5c</script>\uff5c\n\uff5c</shell>\uff5c",
			want:    ActionWarn, wantIssue: "badge",
		},
		{
			name: "fenced example alone is not a call", content: "The format is:\n```\n" + wellFormed + "\n```\n" + longFinal,
			want: ActionFinal,
		},
		{
			name: "real block after a fenced example", content: "Example:\n```\n" + wellFormed + "\n```\nNow the real one:\n" + wellFormed,
			want: ActionExecute, wantScript: "#!/bin/bash\necho hi\n", wantSummary: "say hi", wantTimeout: 100 * time.Second,
		},
		{
			name: "unclosed fence swallows the block", content: "```\n" + wellFormed,
			want: ActionFinal, // quoted + long: the transport's truncation check owns the cut case
		},
		{
			name: "dsml invoke shape, long", content: longFinal + "\n<invoke name=\"shell\">",
			want: ActionWarn, wantIssue: "dsml-shape",
		},
		{
			name: "fullwidth bars badge shape, long", content: longFinal + " \uff5c\uff5c broken badges",
			want: ActionWarn, wantIssue: "dsml-shape",
		},
		{
			name: "tool_calls prose mention stays final", content: longFinal + " I avoided <tool_calls> markup.",
			want: ActionFinal,
		},
		{
			name: "short reply without a block", content: "Sure, starting now.",
			want: ActionWarn, wantIssue: "no-block-short",
		},
		{
			name: "100 runes is still short", content: strings.Repeat("a", 100),
			want: ActionWarn, wantIssue: "no-block-short",
		},
		{
			name: "101 runes is a final report", content: strings.Repeat("a", 101),
			want: ActionFinal,
		},
		{
			name: "reasoning is the fallback", reasoning: wellFormed,
			want: ActionExecute, wantScript: "#!/bin/bash\necho hi\n", wantSummary: "say hi", wantTimeout: 100 * time.Second,
		},
		{
			name: "content wins over reasoning", reasoning: wellFormed, content: "Sure, starting now.",
			want: ActionWarn, wantIssue: "no-block-short",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verdict := Judge(tt.reasoning, tt.content)
			if verdict.Action != tt.want {
				t.Fatalf("Judge() action = %v, want %v (issue %q)", verdict.Action, tt.want, verdict.Issue)
			}
			if tt.wantIssue != "" && !strings.Contains(verdict.Issue, tt.wantIssue) {
				t.Errorf("issue = %q, want substring %q", verdict.Issue, tt.wantIssue)
			}
			if tt.want == ActionExecute {
				block := verdict.Block
				if block == nil {
					t.Fatal("execute verdict without a block")
				}
				if block.Script != tt.wantScript {
					t.Errorf("script = %q, want %q", block.Script, tt.wantScript)
				}
				if block.Summary != tt.wantSummary {
					t.Errorf("summary = %q, want %q", block.Summary, tt.wantSummary)
				}
				if block.Timeout != tt.wantTimeout {
					t.Errorf("timeout = %v, want %v", block.Timeout, tt.wantTimeout)
				}
			}
			if tt.want == ActionWarn && verdict.Warning == "" {
				t.Error("warn verdict without a warning message")
			}
		})
	}
}

func TestShouldEnter(t *testing.T) {
	if !ShouldEnter("", wellFormed) {
		t.Error("a block must enter the shell loop")
	}
	if !ShouldEnter("", "Sure, starting now.") {
		t.Error("a short stalled reply must enter the loop for its warning")
	}
	if ShouldEnter("", longFinal) {
		t.Error("a long final report must not enter the loop")
	}
}

// TestJudgeDSMLNote verifies the "do not use DSML" note rides along when a
// malformed attempt also carries DSML shapes.
func TestJudgeDSMLNote(t *testing.T) {
	verdict := Judge("", "<shell>\n<script>\necho x\n</shell>\n<invoke name=\"shell\">")
	if verdict.Action != ActionWarn {
		t.Fatalf("action = %v, want warn (missing </script>)", verdict.Action)
	}
	if !strings.Contains(verdict.Warning, "Do not use the DSML shape") {
		t.Errorf("warning misses the DSML note:\n%s", verdict.Warning)
	}
}

func TestBlocked(t *testing.T) {
	tests := []struct {
		name    string
		script  string
		blocked bool
		detail  string
	}{
		{name: "sudo", script: "sudo apt-get install x", blocked: true, detail: "sudo"},
		{name: "git stash", script: "git stash", blocked: true},
		{name: "git reset --hard", script: "git reset --hard HEAD~1", blocked: true},
		{name: "curl", script: "curl -s https://example.com", blocked: true, detail: "curl"},
		{name: "wget", script: "wget http://example.com/x", blocked: true},
		{name: "nc", script: "nc -l 1234", blocked: true},
		{name: "mkfs", script: "mkfs.ext4 /dev/sda1", blocked: true},
		{name: "shutdown", script: "shutdown -h now", blocked: true},
		{name: "rm -rf /", script: "rm -rf /", blocked: true},
		{name: "rm -rf ~", script: "rm -rf ~", blocked: true},
		{name: "rm -rf absolute path", script: "rm -rf /tmp/build", blocked: true},
		{name: "echo", script: "echo ok", blocked: false},
		{name: "go test", script: "go test ./...", blocked: false},
		{name: "git commit", script: "git commit -m 'feat: x'", blocked: false},
		{name: "rm -rf relative", script: "rm -rf ./dist", blocked: false},
		{name: "git checkout branch", script: "git checkout -b feature", blocked: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detail, blocked := Blocked(tt.script)
			if blocked != tt.blocked {
				t.Fatalf("Blocked(%q) = (%q, %v), want blocked=%v", tt.script, detail, blocked, tt.blocked)
			}
			if tt.blocked && detail == "" {
				t.Error("blocked=true with an empty detail")
			}
			if tt.detail != "" && detail != tt.detail {
				t.Errorf("detail = %q, want %q", detail, tt.detail)
			}
		})
	}
}
