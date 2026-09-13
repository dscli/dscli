package shellblock

import (
	"fmt"
	"strings"

	"github.com/dscli/dscli/internal/dsml"
)

// formatRules is the four-rule format contract, verbatim from the protocol
// contract (docs/task-shell-block.md); warnings and the tool doc both carry
// it word for word.
const formatRules = "1. `<shell>` must be the leading line of the shell block and it fills the whole line,\n" +
	"2. `</shell>` must be the ending line of the shell block and it fills the whole line,\n" +
	"3. `<script>` must be the leading line of the script block and it fills the whole line,\n" +
	"4. `</script>` must be the ending line of the script block and it fills the whole line."

// formatExample is a minimal well-formed block; warnings show it so the
// model can mirror it exactly.
const formatExample = `<shell>
<script>
#!/bin/bash
set -euo pipefail
echo hello
</script>
<summary>say hello</summary>
<timeout>120</timeout>
</shell>`

// dsmlNote is appended to warnings when the reply carries DSML shapes: the
// site badges and mangles them, so the fix is a plain <shell> block.
const dsmlNote = "Do not use the DSML shape (`<tool_calls>`/`<invoke>`/fullwidth bars): this channel badges and mangles it - use a `<shell>` block instead."

// MalformedWarning is the re-send contract for a malformed block attempt:
// the issue line, the four format rules, and a minimal example.
func MalformedWarning(issue string, dsmlShape bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Your reply contains a malformed `<shell>` block (%s).\n\n", issue)
	b.WriteString("The format rules you MUST follow are:\n\n")
	b.WriteString(formatRules)
	b.WriteString("\n\nExample:\n\n")
	b.WriteString(formatExample)
	b.WriteString("\n\nRe-send the whole block exactly in this shape.\n")
	appendDSMLNote(&b, dsmlShape)
	return b.String()
}

// NoBlockWarning is sent for a short reply that neither contains a block nor
// reads as a final report (a stalled round).
func NoBlockWarning(dsmlShape bool) string {
	var b strings.Builder
	b.WriteString("Your reply neither contains a `<shell>` block nor reads as a final report. Send your next step as ONE `<shell>` block in the same format, or finish the task and write the final report as plain prose.\n")
	appendDSMLNote(&b, dsmlShape)
	return b.String()
}

// DSMLShapeWarning is sent when the reply uses the DSML tool-call shape
// instead of a <shell> block. Nothing was executed.
func DSMLShapeWarning() string {
	var b strings.Builder
	b.WriteString("Your reply uses the DSML tool-call shape (`<tool_calls>`/`<invoke>`/fullwidth bars). This channel badges and mangles DSML markup - it was NOT executed. Use ONE `<shell>` block instead:\n\n")
	b.WriteString(formatRules)
	b.WriteString("\n\nExample:\n\n")
	b.WriteString(formatExample)
	b.WriteString("\n")
	return b.String()
}

// BlockedWarning is the refusal for a script that matched the shared
// destructive-command interception. The script was neither written nor run.
func BlockedWarning(detail string) string {
	return fmt.Sprintf("Your `<shell>` block was NOT executed: it contains a blocked destructive command pattern (`%s`). Destructive commands (`rm -rf /` or `~`, `mkfs`, `dd of=/dev/`, `sudo`, `shutdown`/`reboot`), outbound-network tools (`curl`, `wget`, `nc`, `ncat`, `telnet`, `socat`), and git history-rewriting operations (`git reset --hard`, `git clean -fd`, `git stash`, `git checkout --`, forced push) are rejected outright. Rewrite the script without them and re-send the block.\n", truncateDetail(detail))
}

// appendDSMLNote appends the DSML note when the reply carried DSML shapes.
func appendDSMLNote(b *strings.Builder, dsmlShape bool) {
	if dsmlShape {
		b.WriteString("\n")
		b.WriteString(dsmlNote)
		b.WriteString("\n")
	}
}

// truncateDetail caps the matched pattern echoed in BlockedWarning so a
// verbose regex match (e.g. a long dd invocation) cannot flood the message.
func truncateDetail(detail string) string {
	const max = 60
	if len(detail) <= max {
		return detail
	}
	return detail[:max-3] + "..."
}

// Blocked reports whether script matches the shared destructive-command
// interception and returns the matched pattern for the refusal message. The
// pattern list lives in internal/dsml (dsml.BlockedCmdRe) so the two web
// channels cannot drift; the shell channel applies it to the whole script
// before anything is written or executed (fail-closed).
func Blocked(script string) (string, bool) {
	match := dsml.BlockedCmdRe.FindString(script)
	if match == "" {
		return "", false
	}
	return strings.TrimSpace(match), true
}

// BuildToolDoc renders the shell tool section injected into the role prompt
// in shell mode (prompt.RenderPromptForRoleWithShellTool). The prose follows
// the tool doc validated during the manual experiment (codereview.md), with
// the execution model and the rejected-command list added for the automated
// channel.
func BuildToolDoc() string {
	var b strings.Builder
	b.WriteString("## 🛠️ Available Tools: `shell`\n\n")
	b.WriteString("**Usage: execute bash script**\n")
	b.WriteString(formatExample)
	b.WriteString("\n\nThe format rules you MUST follow are:\n\n")
	b.WriteString(formatRules)
	b.WriteString("\n\n**Iterate inside the script, not across rounds**\n\n")
	b.WriteString("Every `<shell>` block is one remote round-trip, and the channel can drop,\n")
	b.WriteString("truncate, or garble any of them. So make each script carry the whole loop:\n\n")
	b.WriteString("- Avoid: run the test in round 1, read the failure, apply a fix in round 2,\n")
	b.WriteString("  run the test again in round 3. Three fragile round-trips for one task.\n")
	b.WriteString("- Prefer: one script that loops locally. Run the test, on failure apply the\n")
	b.WriteString("  fix, re-run, print the full transcript. One round-trip.\n\n")
	b.WriteString("The script is the control-flow language - use loops, conditionals, retries.\n")
	b.WriteString("Start with `set -euo pipefail`. Exit non-zero on failure: the exit status is\n")
	b.WriteString("a signal, not decoration. Print everything the next decision needs (command,\n")
	b.WriteString("exit code, relevant output) in one shot.\n\n")
	b.WriteString("The script runs on the local project host with the user's permissions; its\n")
	b.WriteString("merged stdout+stderr returns as an attached `scriptN.txt` file. The block is\n")
	b.WriteString("rejected outright - nothing runs - when the script contains: destructive\n")
	b.WriteString("commands (`rm -rf /` or `~`, `mkfs`, `dd of=/dev/`, `sudo`,\n")
	b.WriteString("`shutdown`/`reboot`), outbound-network tools (`curl`, `wget`, `nc`, `ncat`,\n")
	b.WriteString("`telnet`, `socat`), or git history-rewriting operations (`git reset --hard`,\n")
	b.WriteString("`git clean -fd`, `git stash`, `git checkout --`, forced push).\n\n")
	b.WriteString("Field conventions:\n")
	b.WriteString("- `<summary>`: <= 40 chars, states intent (\"Run tests, fix import errors\").\n")
	b.WriteString("  It is the human-visible audit line - an inaccurate summary hides a wrong\n")
	b.WriteString("  action.\n")
	b.WriteString("- `<timeout>`: default 120s. Raise it (e.g. 1200) when the script runs a\n")
	b.WriteString("  test suite or a build; more work needs more time.\n\n")
	b.WriteString("**Finishing**: when the task is complete, write the final report as plain\n")
	b.WriteString("prose - a reply without a `<shell>` block ends the session.\n")
	return b.String()
}
