package ask

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	_ "embed"

	"github.com/dscli/dscli/internal/lp"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/prompt"
	"github.com/dscli/dscli/internal/shell"
	"github.com/dscli/dscli/internal/toolcall"
	"github.com/nanjj/clog"
)

//go:embed code_review.md
var code_review_md string

var codeReviewTool = toolcall.ToolDef{
	Name:        "code_review",
	DisplayName: "Code Review",
	Description: code_review_md,
	Strict:      true,
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"summary": map[string]any{
				"type":        "string",
				"description": "Required, background and focus of this commit, 1-1024 chars",
			},
			"test_command": map[string]any{
				"type":        "string",
				"description": "Optional test command, default empty skips tests, 1-128 chars",
			},
			"since": map[string]any{
				"type":        "string",
				"description": "Number of commits to review, e.g. '-1' (last), '-2' (last 2), default '-1'",
			},
			"timeout": map[string]any{
				"type":        "integer",
				"description": "Timeout in seconds for the expert phase (default 0 = no extra bound; the tool-level budget is 30 minutes). Set longer for very large projects with many tests.",
			},
		},
		"required":             []string{"summary"},
		"additionalProperties": false,
	},
	Category: "check",
	// One browser session plus model generation: a large review can take
	// several minutes to generate, and 15 minutes proved too short.
	Timeout: 30 * time.Minute,
	Handler: handleCodeReview,
}

func init() {
	// WebChat is always available (free DeepSeek Web) — no API key needed.
	toolcall.RegisterTool(codeReviewTool)
}

// handleCodeReview 处理代码审查工具调用
func handleCodeReview(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	span, ctx := clog.StartSpanFromContext(ctx, "handleCodeReview")
	defer span.Finish()
	summary := toolcall.ToolArgsValue(args, "summary", "")
	testCommand := toolcall.ToolArgsValue(args, "test_command", "")
	since := toolcall.ToolArgsValue(args, "since", "-1")
	// timeout bounds the expert phase (default 0 = no extra bound; the
	// tool-level Timeout is the ceiling). Wrapping the whole handler keeps
	// git/test steps fast anyway — they finish in seconds.
	if secs := toolcall.ToolArgsValue(args, "timeout", 0); secs > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(secs)*time.Second)
		defer cancel()
	}

	if summary == "" {
		outfmt.Println("❌ 必须提供提交摘要")
		err = fmt.Errorf("必须提供提交摘要")
		return result, warning, err
	}

	// 校验 since 格式（并取出提交数，用于列出变更文件）
	commits, err := parseSince(since)
	if err != nil {
		outfmt.Printf("❌ since 参数格式错误: %v\n", err)
		return result, warning, err
	}

	// 检查是否有未提交的更改（staged + unstaged，忽略 untracked）
	fmt.Println("🔍 检查是否有未提交的更改...")
	statusScript := `git status --porcelain | grep -v '^??'`
	status, shellErr := shell.SimpleExecute(ctx, statusScript)
	if shellErr != nil {
		// grep 返回非零退出码表示没有匹配，这是正常情况
		status = ""
	}

	if status != "" {
		outfmt.Println("❌ 检测到未提交的更改")
		outfmt.Println("当前状态：")
		outfmt.Println(status)
		err = fmt.Errorf("请使用 'git status' 查看详情，并使用 'git add' 和 'git commit' 提交所有更改后再进行审查")
		return result, warning, err
	}

	outfmt.Println("✅ 没有未提交的更改")

	if testCommand != "" {
		outfmt.Println("🔍 运行单元测试:", testCommand)
		testOutput := ""
		testOutput, err = shell.SimpleExecute(ctx, testCommand)
		if err != nil {
			outfmt.Println("❌ 单元测试未通过")
			errorMsg := fmt.Sprintf("单元测试未通过，请修复测试后再审查。\n测试命令：%s\n", testCommand)
			if testOutput != "" {
				// 截断过长的输出
				outputLines := strings.Split(testOutput, "\n")
				if len(outputLines) > 20 {
					errorMsg += "测试输出（前20行）：\n" + strings.Join(outputLines[:20], "\n")
					errorMsg += fmt.Sprintf("\n... 还有%d行输出", len(outputLines)-20)
				} else {
					errorMsg += "测试输出：\n" + testOutput
				}
			}
			outfmt.Println("❌ 单元测试失败")
			err = fmt.Errorf("%s: %w", errorMsg, err)
			return result, warning, err
		}
		if testOutput != "" {
			outfmt.Println(testOutput)
		}
		outfmt.Println("✅ 单元测试通过")
	}

	// 获取最新的提交信息
	logScript := fmt.Sprintf(`git log --oneline %s`, since)
	log, err := shell.SimpleExecute(ctx, logScript)
	if err != nil {
		outfmt.Println("❌ 获取提交历史失败")
		err = fmt.Errorf("获取提交历史失败: %w", err)
		return result, warning, err
	}

	if strings.TrimSpace(log) == "" {
		outfmt.Println("❌ 没有找到提交记录")
		err = fmt.Errorf("没有找到提交记录，请先提交代码")
		return result, warning, err
	}

	outfmt.Println("📝 提交信息:")
	outfmt.Println(log)

	// 获取完整的提交信息用于构建请求
	fullLogScript := fmt.Sprintf(`git log --format="%%B" %s`, since)
	fullLog, err := shell.SimpleExecute(ctx, fullLogScript)
	if err != nil {
		fullLog = log // 如果失败，使用简短的log
	}

	// 生成patch
	patchScript := fmt.Sprintf(`git --no-pager format-patch --stdout %s`, since)
	patch, err := shell.SimpleExecute(ctx, patchScript)
	if err != nil {
		fmt.Println("❌ 生成patch失败")
		err = fmt.Errorf("生成patch失败: %w", err)
		return result, warning, err
	}

	// 审查输入全部作为附件上传：review-guide.md（渲染后的 review 提示词）、
	// changes.patch（完整 diff）、变更文件全文（附件名编码仓库路径）、
	// AGENTS.md（存在时）和 gocyclo.txt（变更 Go 文件的圈复杂度报告）。
	// review 角色默认没有可执行工具（role_configs / roles.DefaultFor），
	// 专家静态地基于这些输入完成审查；消息正文只承载摘要、提交信息和覆盖
	// 清单（哪些文件没有附上），让盲区显式可见。
	repoRoot, err := gitRepoRoot(ctx)
	if err != nil {
		return result, warning, err
	}
	dir, err := os.MkdirTemp("", "dscli-code-review-*")
	if err != nil {
		err = fmt.Errorf("创建审查附件目录失败: %w", err)
		return result, warning, err
	}
	defer os.RemoveAll(dir)

	attachments, plan, err := assembleReviewAttachments(ctx, dir, repoRoot, patch, commits)
	if err != nil {
		return result, warning, err
	}

	message := buildReviewMessage(summary, fullLog, plan)
	outfmt.Printf("📤 发送代码审查请求到 DeepSeek Web（免费）...\n%s\n", message)
	outfmt.Printf("📎 附件 %d 个（%d 个变更文件全文，%d 个文件未附）\n", len(attachments), len(plan.Attached), len(plan.NotAttached))
	warning = reviewAttachmentWarning(plan)
	result, err = AskExpertWithRoleFiles(ctx, message, "review", attachments)
	if err != nil {
		err = fmt.Errorf("代码审查失败: %w", err)
		return result, warning, err
	}

	outfmt.Printf("✅ 代码审查结果\n%s\n", result)
	return result, warning, err
}

// parseSince 校验并解析 since 参数：必须是 "-N" 格式（如 "-1", "-2", "-3"），
// 返回提交数 N（用于列出变更文件）。
func parseSince(since string) (int, error) {
	if !strings.HasPrefix(since, "-") {
		return 0, fmt.Errorf("格式必须为 '-N'（如 '-1', '-2', '-3'），当前值: %q", since)
	}
	n, err := strconv.Atoi(since[1:])
	if err != nil || n < 1 {
		return 0, fmt.Errorf("格式必须为 '-N'（如 '-1', '-2', '-3'），当前值: %q", since)
	}
	return n, nil
}

// ---------- 审查附件 ----------

// Review attachment names: the fixed inputs always uploaded (when available)
// under these names. Changed files are uploaded under path-encoded names (see
// encodeAttachmentName).
const (
	reviewGuideName   = "review-guide.md"
	reviewPatchName   = "changes.patch"
	reviewAgentsName  = "AGENTS.md"
	reviewGocycloName = "gocyclo.txt"
)

// maxReviewCommitLogRunes caps the commit-message section of the request
// message: a very long multi-commit log must not push the coverage note past
// a site-truncated send, and the head of the log carries the subject lines a
// reviewer needs first.
const maxReviewCommitLogRunes = 40000

// gocycloThreshold is the project standard for cyclomatic complexity; the
// report lists functions above it (values 21+).
const gocycloThreshold = 20

// runGocyclo is the gocyclo runner used by handleCodeReview. A package
// variable so tests can inject a fake (no external binary needed there).
var runGocyclo = gocycloCmd

// isGocycloAcceptableExit reports whether err is nil or gocyclo's normal
// non-zero exit for "functions above the threshold were found" (exit status
// 1). Any other error means the report may be incomplete.
func isGocycloAcceptableExit(err error) bool {
	if err == nil {
		return true
	}
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr) && exitErr.ExitCode() == 1
}

// gocycloCmd runs `gocyclo -over <threshold>` over the changed Go files (dir =
// repo root) and returns the report attached as gocyclo.txt. Failures (missing
// binary, unparsable file) are reported IN the report instead of failing the
// review: complexity is a secondary signal, not a gate.
func gocycloCmd(ctx context.Context, dir string, files []string) string {
	if len(files) == 0 {
		return "No changed Go files in this review.\n"
	}
	if _, err := exec.LookPath("gocyclo"); err != nil {
		return fmt.Sprintf("gocyclo not found on PATH; cyclomatic complexity was not measured. Changed Go files: %s\n", strings.Join(files, ", "))
	}
	args := append([]string{"-over", strconv.Itoa(gocycloThreshold)}, files...)
	cmd := exec.CommandContext(ctx, "gocyclo", args...)
	cmd.Dir = dir
	// stdout and stderr are captured separately: findings go to stdout,
	// fatal errors (log.Fatal) to stderr, and gocyclo exits 1 for BOTH - a
	// merged stream could present an aborted run's error text as findings.
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	report := fmt.Sprintf("gocyclo -over %d (project threshold), %d changed Go file(s):\n", gocycloThreshold, len(files))
	findings := strings.TrimSpace(stdout.String())
	diagnostics := strings.TrimSpace(stderr.String())
	switch {
	case findings == "" && diagnostics == "" && err == nil:
		report += fmt.Sprintf("No function above the cyclomatic threshold (%d).\n", gocycloThreshold)
	case findings != "" && diagnostics == "" && isGocycloAcceptableExit(err):
		// Clean findings: nil or the normal "found something" exit (1).
		report += findings + "\n"
	case findings != "" || diagnostics != "":
		// Anything else may be a partial listing (an abort after some
		// findings, or diagnostics on stderr): show it, but say so.
		if findings != "" {
			report += findings + "\n"
		}
		if diagnostics != "" {
			report += diagnostics + "\n"
		}
		if diagnostics != "" {
			report += "(gocyclo wrote to stderr; the report may be partial)\n"
		} else {
			report += fmt.Sprintf("(gocyclo exited with %v; the report may be partial)\n", err)
		}
	default:
		report += fmt.Sprintf("gocyclo failed: %v\n", err)
	}
	return report
}

// gitRepoRoot returns the toplevel directory of the git repository containing
// the working directory. Git reports repository-relative paths even when run
// from a subdirectory, so every file lookup (changed files, AGENTS.md,
// gocyclo) resolves against this root.
func gitRepoRoot(ctx context.Context) (string, error) {
	out, err := shell.SimpleExecute(ctx, "git rev-parse --show-toplevel")
	if err != nil {
		return "", fmt.Errorf("无法定位 git 仓库根目录: %w", err)
	}
	root := strings.TrimSpace(out)
	if root == "" {
		return "", fmt.Errorf("无法定位 git 仓库根目录: 输出为空")
	}
	return root, nil
}

// numstatEntry is one changed-file entry from `git diff --numstat`: a path
// plus whether git considers the change binary (both counters "-"). Binary
// entries are reported (not silently dropped) so the coverage note can list
// them as skipped.
type numstatEntry struct {
	path   string
	binary bool
}

// parseNumstat parses `git diff --numstat` output into changed-file entries.
// With --no-renames a rename shows as delete + add; the deleted path is
// filtered later by the on-disk existence check.
func parseNumstat(out string) []numstatEntry {
	var entries []numstatEntry
	for _, line := range strings.Split(out, "\n") {
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) != 3 {
			continue
		}
		p := strings.TrimSpace(fields[2])
		if p == "" {
			continue
		}
		entries = append(entries, numstatEntry{path: p, binary: fields[0] == "-" && fields[1] == "-"})
	}
	return entries
}

// parseNameOnly parses `git log --name-only --pretty=format:` output into a
// deduplicated path list. Used when the diff range is unavailable (e.g. a
// shallow clone where HEAD~N does not resolve); the list can include deleted
// and binary files, which the caller filters against the filesystem.
func parseNameOnly(out string) []string {
	seen := map[string]bool{}
	var paths []string
	for _, line := range strings.Split(out, "\n") {
		p := strings.TrimSpace(line)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		paths = append(paths, p)
	}
	return paths
}

// listChangedFiles returns the files touched by the last n commits. The
// primary path is `git diff --numstat` over HEAD~n..HEAD (binary entries
// flagged); when the range does not resolve (shallow history), it falls back
// to `git log --name-only` (binary detection deferred to the NUL sniff).
// Returns nil when both fail - the review then proceeds with the patch alone.
func listChangedFiles(ctx context.Context, n int) []numstatEntry {
	out, err := shell.SimpleExecute(ctx, fmt.Sprintf("git diff --numstat --no-renames HEAD~%d..HEAD", n))
	if err == nil {
		return parseNumstat(out)
	}
	out, err = shell.SimpleExecute(ctx, fmt.Sprintf("git log --name-only --pretty=format: -n %d", n))
	if err != nil {
		return nil
	}
	var entries []numstatEntry
	for _, p := range parseNameOnly(out) {
		entries = append(entries, numstatEntry{path: p})
	}
	return entries
}

// isBinaryFile reports whether the file looks binary: a NUL byte within the
// first 8000 bytes, matching git's FIRST_FEW_BYTES heuristic. Binary files
// cannot be uploaded as text attachments, so they are skipped and listed in
// the coverage note. An unreadable file returns false; it is caught later as
// a copy failure and lands in the not-attached list instead.
func isBinaryFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 8000)
	n, _ := io.ReadFull(f, buf)
	return bytes.IndexByte(buf[:n], 0) >= 0
}

// reviewCandidates splits the changed files of the last n commits into
// attachable candidates and skipped entries. A file is skipped when numstat
// flags it binary, when it no longer exists (deleted), when it is not a
// regular file (submodule), or when the NUL sniff calls it binary on the
// git log fallback path; the coverage note lists every skipped path.
func reviewCandidates(ctx context.Context, repoRoot string, commits int) (candidates []candidateFile, skipped []string) {
	for _, e := range listChangedFiles(ctx, commits) {
		full := filepath.Join(repoRoot, e.path)
		info, statErr := os.Stat(full)
		if e.binary || statErr != nil || !info.Mode().IsRegular() || isBinaryFile(full) {
			skipped = append(skipped, e.path)
			continue
		}
		candidates = append(candidates, candidateFile{path: e.path, size: info.Size()})
	}
	return candidates, skipped
}

// assembleReviewAttachments collects the changed files, renders the fixed
// inputs and writes every review attachment into dir (owned by the caller).
// It returns the upload paths and the coverage plan; a write failure aborts
// the review because an incomplete attachment set must not be sent silently.
func assembleReviewAttachments(ctx context.Context, dir, repoRoot, patch string, commits int) ([]string, reviewPlan, error) {
	candidates, skipped := reviewCandidates(ctx, repoRoot, commits)

	changed := make([]string, 0, len(candidates))
	var goFiles []string
	for _, c := range candidates {
		changed = append(changed, c.path)
		if strings.HasSuffix(c.path, ".go") {
			goFiles = append(goFiles, c.path)
		}
	}
	sort.Strings(changed)

	guide := prompt.RenderPromptForRole(ctx, "review")
	agents := ""
	if b, readErr := os.ReadFile(filepath.Join(repoRoot, reviewAgentsName)); readErr == nil {
		agents = string(b)
	}
	gocyclo := runGocyclo(ctx, repoRoot, goFiles)

	plan := reviewPlan{CommitCount: commits, Changed: changed, Skipped: skipped, Agents: agents != ""}
	// 固定附件优先：guide/patch/AGENTS.md/gocyclo 先占预算，patch 只在自身
	// 超预算时才按文件区段丢弃（越小越优先保留，覆盖面最大）。
	patchBody := patch
	othersBytes := int64(len(guide)) + int64(len(gocyclo))
	if agents != "" {
		othersBytes += int64(len(agents))
	}
	if othersBytes+int64(len(patchBody)) > int64(lp.WebUploadMaxTotal) {
		budget := int64(lp.WebUploadMaxTotal) - othersBytes
		if budget < 0 {
			budget = 0
		}
		cut, dropped := truncatePatchToBudget(patchBody, int(budget))
		plan.PatchDropped = dropped
		// Flag from the actual reduction, not from the dropped list: a
		// degenerate patch (no parseable sections) can shrink without naming
		// any file, and the coverage note must never claim completeness.
		plan.PatchTruncated = len(cut) < len(patchBody)
		patchBody = cut
	}
	fixedBytes := othersBytes + int64(len(patchBody))
	fixedCount := 3 // guide + patch + gocyclo
	if agents != "" {
		fixedCount++
	}
	kept, dropped := selectReviewFiles(candidates, lp.WebUploadMaxFiles-fixedCount, int64(lp.WebUploadMaxTotal)-fixedBytes)

	attachments := make([]string, 0, fixedCount+len(kept))
	for _, f := range []struct{ name, content string }{
		{reviewGuideName, guide},
		{reviewPatchName, patchBody},
		{reviewGocycloName, gocyclo},
	} {
		p, werr := writeReviewFile(dir, f.name, f.content)
		if werr != nil {
			return nil, reviewPlan{}, werr
		}
		attachments = append(attachments, p)
	}
	if agents != "" {
		p, werr := writeReviewFile(dir, reviewAgentsName, agents)
		if werr != nil {
			return nil, reviewPlan{}, werr
		}
		attachments = append(attachments, p)
	}

	usedNames := map[string]bool{
		reviewGuideName:   true,
		reviewPatchName:   true,
		reviewGocycloName: true,
	}
	if agents != "" {
		usedNames[reviewAgentsName] = true
	}
	attached := make([]string, 0, len(kept))
	notAttached := make([]string, 0, len(dropped))
	for _, d := range dropped {
		notAttached = append(notAttached, d.path)
	}
	for _, c := range kept {
		name := uniqueAttachmentName(usedNames, encodeAttachmentName(c.path))
		p, cerr := copyReviewFile(dir, name, filepath.Join(repoRoot, c.path))
		if cerr != nil {
			fmt.Fprintf(os.Stderr, "⚠️ 附件复制失败: %v\n", cerr)
			notAttached = append(notAttached, c.path)
			continue
		}
		attachments = append(attachments, p)
		attached = append(attached, c.path)
	}
	sort.Strings(attached)
	sort.Strings(notAttached)
	plan.Attached = attached
	plan.NotAttached = notAttached
	return attachments, plan, nil
}

// candidateFile is a changed file that can be attached, with its size on disk.
type candidateFile struct {
	path string
	size int64
}

// selectReviewFiles greedily keeps the smallest files first while both the
// attachment count and byte budgets allow (the site caps uploads at 50 files
// and 100MB; code_review pre-drops inputs instead of failing the whole call).
// Smallest-first maximizes the number of files with full-content context; the
// complete diff is in changes.patch either way. Kept and dropped are returned
// sorted by path for stable output.
func selectReviewFiles(files []candidateFile, maxCount int, maxBytes int64) (kept, dropped []candidateFile) {
	sorted := make([]candidateFile, len(files))
	copy(sorted, files)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].size != sorted[j].size {
			return sorted[i].size < sorted[j].size
		}
		return sorted[i].path < sorted[j].path
	})
	var used int64
	for _, f := range sorted {
		if len(kept) < maxCount && used+f.size <= maxBytes {
			kept = append(kept, f)
			used += f.size
			continue
		}
		dropped = append(dropped, f)
	}
	sortByPath(kept)
	sortByPath(dropped)
	return kept, dropped
}

// sortByPath sorts candidate files by path.
func sortByPath(files []candidateFile) {
	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })
}

// encodeAttachmentName maps a repo-relative path to a flat attachment file
// name: path separators become "__" (internal/lp/doc.go ->
// internal__lp__doc.go). Uploads keep only the base name, so the encoded form
// is what the expert sees; the request message explains the encoding and
// changes.patch carries the real paths.
func encodeAttachmentName(path string) string {
	return strings.ReplaceAll(path, "/", "__")
}

// uniqueAttachmentName appends a numeric suffix when an earlier file already
// mapped to the same encoded name (a/b__c vs a__b/c both encode to a__b__c),
// so no attachment silently overwrites another.
func uniqueAttachmentName(used map[string]bool, name string) string {
	if !used[name] {
		used[name] = true
		return name
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s_%d", name, i)
		if !used[candidate] {
			used[candidate] = true
			return candidate
		}
	}
}

// writeReviewFile writes one attachment file into dir and returns its path.
func writeReviewFile(dir, name, content string) (string, error) {
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("写入审查附件 %s 失败: %w", name, err)
	}
	return path, nil
}

// copyReviewFile copies a changed file into dir under the given attachment
// name and returns the new path.
func copyReviewFile(dir, name, source string) (string, error) {
	in, err := os.Open(source)
	if err != nil {
		return "", fmt.Errorf("打开 %s 失败: %w", source, err)
	}
	defer in.Close()
	path := filepath.Join(dir, name)
	out, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("创建附件 %s 失败: %w", name, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return "", fmt.Errorf("复制 %s 失败: %w", source, err)
	}
	if err := out.Close(); err != nil {
		return "", fmt.Errorf("关闭附件 %s 失败: %w", name, err)
	}
	return path, nil
}

// dropSectionsToBytes greedily keeps the smallest patch sections until the
// byte budget is exhausted (smallest-first: maximal coverage count).
func dropSectionsToBytes(sections []namedSection, maxBytes int) (kept, dropped []namedSection) {
	sort.Slice(sections, func(i, j int) bool { return len(sections[i].text) < len(sections[j].text) })
	used := 0
	for _, s := range sections {
		if used+len(s.text) <= maxBytes {
			kept = append(kept, s)
			used += len(s.text)
		} else {
			dropped = append(dropped, s)
		}
	}
	return kept, dropped
}

// sectionNames returns the file names of the patch sections.
func sectionNames(secs []namedSection) []string {
	names := make([]string, 0, len(secs))
	for _, s := range secs {
		names = append(names, s.name)
	}
	return names
}

// truncatePatchToBudget drops whole file sections (smallest first) until the
// patch fits maxBytes (attachment limits are file sizes, so this budget is in
// bytes), and reports the dropped section names. A defensive path: the diff
// is the primary review input and is only cut when it alone blows the upload
// budget.
func truncatePatchToBudget(patch string, maxBytes int) (string, []string) {
	secs := splitPatchByFile(patch)
	if maxBytes <= 0 {
		return "", sectionNames(secs)
	}
	kept, dropped := dropSectionsToBytes(secs, maxBytes)
	if len(dropped) == 0 {
		return patch, nil
	}
	return joinNamed(kept), sectionNames(dropped)
}

// reviewPlan describes what the review request carries; it drives the
// message's coverage note (the expert must know what it cannot see) and the
// local warning.
type reviewPlan struct {
	CommitCount    int      // commits under review
	Changed        []string // reviewable changed files (existing, non-binary)
	Skipped        []string // changed entries skipped (deleted, binary, unreadable)
	Attached       []string // repo paths attached with full content
	NotAttached    []string // repo paths not attached (attachment budget / read error)
	Agents         bool     // AGENTS.md was attached
	PatchDropped   []string // files whose patch section was dropped
	PatchTruncated bool     // the patch was cut to fit the upload budget
}

// capCommitLog head-caps the commit-message section at
// maxReviewCommitLogRunes with an explicit marker.
func capCommitLog(log string) string {
	if countRunes(log) <= maxReviewCommitLogRunes {
		return log
	}
	return cutToRuneLen(log, maxReviewCommitLogRunes) + "\n[commit message truncated for length]"
}

// buildReviewMessage assembles the first message: commit background, the
// capped commit message, one line describing the attached inputs, and the
// coverage note. The coverage note replaces the old in-body truncation note:
// with every input uploaded as an attachment, the message is the only place
// that can name the blind spots (files not attached, patch sections dropped).
func buildReviewMessage(summary, commitLog string, plan reviewPlan) string {
	var sb strings.Builder
	sb.WriteString("## Commit Background\n")
	sb.WriteString(summary)
	sb.WriteString("\n\n## Commit Message\n")
	sb.WriteString(capCommitLog(commitLog))
	sb.WriteString("\n\n## Review Inputs\n")
	inputs := []string{
		"review-guide.md (the review instructions - follow them)",
		"changes.patch (the complete diff)",
	}
	switch {
	case len(plan.NotAttached) > 0:
		inputs = append(inputs, "the full content of the changed files that fit the upload budget")
	case len(plan.Skipped) > 0:
		inputs = append(inputs, "the full content of every changed text file (deleted and binary files are listed as skipped)")
	default:
		inputs = append(inputs, "the full content of every changed file")
	}
	if plan.Agents {
		inputs = append(inputs, "AGENTS.md (project guide)")
	}
	inputs = append(inputs, "gocyclo.txt (cyclomatic complexity of the changed Go files, project threshold 20)")
	sb.WriteString("All review inputs are attached to this message: " + strings.Join(inputs, ", ") + ". ")
	sb.WriteString("Attachment file names encode repo paths: \"internal__lp__webchat.go\" is \"internal/lp/webchat.go\".\n")
	sb.WriteString("\n## Coverage\n")
	fmt.Fprintf(&sb, "- Commits under review: %d.\n", plan.CommitCount)
	fmt.Fprintf(&sb, "- Changed files: %d (%d skipped as deleted/binary/unreadable).\n", len(plan.Changed)+len(plan.Skipped), len(plan.Skipped))
	fmt.Fprintf(&sb, "- Full content attached: %d file(s).\n", len(plan.Attached))
	if len(plan.NotAttached) == 0 {
		sb.WriteString("- Not attached: none.\n")
	} else {
		fmt.Fprintf(&sb, "- NOT attached (attachment budget): %s\n", strings.Join(plan.NotAttached, ", "))
	}
	if !plan.Agents {
		sb.WriteString("- AGENTS.md not attached (absent, empty, or unreadable).\n")
	}
	if plan.PatchTruncated {
		if len(plan.PatchDropped) > 0 {
			fmt.Fprintf(&sb, "- changes.patch was truncated: patch sections missing for %s\n", strings.Join(plan.PatchDropped, ", "))
		} else {
			sb.WriteString("- changes.patch was truncated (some content omitted).\n")
		}
	}
	return sb.String()
}

// reviewAttachmentWarning summarizes the coverage gaps for the local caller,
// capped like the old truncation warning. The same information rides in the
// request message for the expert.
func reviewAttachmentWarning(plan reviewPlan) string {
	var parts []string
	if n := len(plan.NotAttached); n > 0 {
		parts = append(parts, fmt.Sprintf("附件预算不足，未附上 %d 个文件的全文: %s", n, cappedList(plan.NotAttached)))
	}
	if plan.PatchTruncated {
		if len(plan.PatchDropped) > 0 {
			parts = append(parts, fmt.Sprintf("patch 超预算已截断，缺少区段: %s", cappedList(plan.PatchDropped)))
		} else {
			parts = append(parts, "patch 超预算已截断（部分内容已省略）")
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "⚠️ " + strings.Join(parts, "；") + "。"
}

// cappedList joins names for terminal output, capping the listed count at
// maxWarnList with an explicit remainder.
func cappedList(names []string) string {
	if len(names) <= maxWarnList {
		return strings.Join(names, ", ")
	}
	return strings.Join(names[:maxWarnList], ", ") + fmt.Sprintf(" …等共 %d 个", len(names))
}

// ---------- 请求构建 ----------

// buildCodeReviewRequest 组装审查请求。patch 为空时省略对应区段。
func buildCodeReviewRequest(summary, commitLog, patch string) string {
	req := "## Commit Background\n" + summary + "\n\n## Commit Message\n" + commitLog
	if patch != "" {
		req += "\n\n## Code Changes\n" + patch
	}
	return req
}

// ---------- patch 文件拆分 ----------

// namedSection 是带文件名的文本区段（用于逐文件保留/丢弃）。
type namedSection struct {
	name string
	text string
}

// splitPatchByFile 将 patch 按文件拆分为独立区段（用于逐文件丢弃）。
func splitPatchByFile(patch string) []namedSection {
	var secs []namedSection
	var cur *namedSection
	for _, line := range strings.Split(patch, "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			if cur != nil {
				secs = append(secs, *cur)
			}
			cur = &namedSection{text: line + "\n"}
			continue
		}
		if cur != nil {
			cur.text += line + "\n"
			// 先记录 --- a/ 的旧路径作为回退名：删除文件时 +++ 为
			// /dev/null（新增文件则 --- 为 /dev/null、+++ 正常覆盖）。
			if strings.HasPrefix(line, "--- a/") {
				cur.name = strings.TrimPrefix(line, "--- a/")
			}
			if strings.HasPrefix(line, "+++ b/") {
				cur.name = strings.TrimPrefix(line, "+++ b/")
			}
		}
	}
	if cur != nil {
		secs = append(secs, *cur)
	}
	return secs
}

// ---------- 截断管线 ----------

// maxUserInputLen is the maximum RUNE count for the user portion of a code
// review request. The site limit is a character count (~158k runes measured
// 2026-08-29: ASCII 162k pass, ~165k reject; byte count is NOT the metric —
// see lp.webChatMaxInputRunes). The role prompt (~7KB ≈ 2-7k runes) is
// prepended by HandleWebChat, so the user portion keeps a margin below the
// site cap: 140k runes ≈ 158k cap minus role/system prompt and the DSML
// feedback that follow-up rounds append. (26000 was the old "~30k chars"
// era value; the site grew, and a too-small cap silently dropped diff
// sections the expert could not see.)
const maxUserInputLen = 140000

// truncNoteBudget 为请求正文末尾的截断提示（被丢弃文件清单）预留的字符空间：
// 清单随请求发送，专家才能感知覆盖盲区并主动 read_file 补读；该预算从 diff
// 保留中扣除（牺牲少量 diff 换取盲区可见）。
const truncNoteBudget = 4096

// maxWarnList 限制 warning 中明细列表的条数，避免极端场景（数百文件）时
// 本地告警把终端撑爆；buildTruncNote 中用同一上限保证提示本身不超预算。
const maxWarnList = 20

// truncateReviewRequest 检查请求总长并在超限时按文件丢弃 diff 区段（最小优先）。
// 被丢弃文件的 diff 不再可见；为保证覆盖盲区可见，截断提示（含被丢弃文件清单）
// 会拼进请求正文随请求发送，专家据此用 read_file 补读；同一提示也作为 warning
// 返回给本地用户。
func truncateReviewRequest(summary, commitLog, patch string) (string, string) {
	req := buildCodeReviewRequest(summary, commitLog, patch)
	if countRunes(req) <= maxUserInputLen {
		return req, ""
	}
	origLen := countRunes(req)
	var warns []string

	// 按文件丢弃 diff 区段（小文件优先保留，保证覆盖面），并为截断提示预留预算
	diffSecs := splitPatchByFile(patch)
	base := buildCodeReviewRequest(summary, commitLog, "")
	kept, dropped := dropUntilFits(base, diffSecs, maxUserInputLen-truncNoteBudget)
	for _, d := range dropped {
		warns = append(warns, fmt.Sprintf("diff 过大，已丢弃 %s 的 diff（%d 字符），专家可用 read_file 读取该文件", d.name, countRunes(d.text)))
	}

	req = buildCodeReviewRequest(summary, commitLog, joinNamed(kept)) + buildTruncNote(dropped, false)
	if countRunes(req) <= maxUserInputLen {
		return req, truncateWarning(origLen, req, warns)
	}

	// 兜底：summary/commitLog 本身超限（或清单超预算）的极端情况，按 rune 边界
	// 硬截断。先为截断提示预留预算再切正文，提示承载"已硬截断"信号并列出
	// 被丢弃文件，保证专家在最需要提示的场景仍能看到盲区。
	hardNote := buildTruncNote(dropped, true)
	body := buildCodeReviewRequest(summary, commitLog, joinNamed(kept))
	req = cutToRuneLen(body, maxUserInputLen-countRunes(hardNote)) + hardNote
	warns = append(warns, "输入仍超限，已硬截断")
	return req, truncateWarning(origLen, req, warns)
}

// countRunes returns the rune count of s (the site limit is a character
// count — see lp.webChatMaxInputRunes).
func countRunes(s string) int {
	return utf8.RuneCountInString(s)
}

// buildTruncNote 生成随请求发送的截断提示：列出被丢弃的文件（覆盖盲区），
// 让专家知道哪些文件需要 read_file 补读。清单最多列 maxWarnList 条，
// 超出时压缩为"前 N 条 + …等共 M 个"，保证提示本身不超预算。
// hardTrunc 为 true 表示走到了硬截断兜底（提示更强调信号）。
func buildTruncNote(dropped []namedSection, hardTrunc bool) string {
	var sb strings.Builder
	sb.WriteString("\n\n## ⚠️ 审查输入截断\n")
	if hardTrunc {
		sb.WriteString("输入超限，已硬截断；以下文件的 diff 未包含在本请求中，请用 read_file 读取补全：\n")
	} else {
		sb.WriteString("以下文件因输入长度限制未包含 diff，请用 read_file 读取补全：\n")
	}
	if len(dropped) == 0 {
		sb.WriteString("（无 diff 区段可列出）\n")
		return sb.String()
	}
	shown := dropped
	if len(shown) > maxWarnList {
		shown = shown[:maxWarnList]
	}
	for _, d := range shown {
		sb.WriteString("- ")
		sb.WriteString(d.name)
		sb.WriteString("\n")
	}
	if len(dropped) > maxWarnList {
		fmt.Fprintf(&sb, "…等共 %d 个文件\n", len(dropped))
	}
	return sb.String()
}

// cutToRuneLen 返回 s 的前缀，runes 数不超过 maxRunes（字节预算对 rune
// 计数不适用：站点限制按字符数，见 maxUserInputLen）。负预算（硬截断时
// 截断提示本身超预算的防御场景）返回空，绝不 panic。
func cutToRuneLen(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	if countRunes(s) <= maxRunes {
		return s
	}
	return string([]rune(s)[:maxRunes])
}

// dropUntilFits 按区段 rune 数升序贪心保留（小文件优先，覆盖面最大），
// 返回保留与丢弃的区段。base 为必保内容，limit 为总预算（rune 数）。
func dropUntilFits(base string, sections []namedSection, limit int) (kept, dropped []namedSection) {
	sort.Slice(sections, func(i, j int) bool { return countRunes(sections[i].text) < countRunes(sections[j].text) })
	budget := limit - countRunes(base)
	for _, s := range sections {
		if countRunes(s.text) <= budget {
			kept = append(kept, s)
			budget -= countRunes(s.text)
		} else {
			dropped = append(dropped, s)
		}
	}
	return kept, dropped
}

// joinNamed 拼接区段文本。
func joinNamed(secs []namedSection) string {
	var sb strings.Builder
	for _, s := range secs {
		sb.WriteString(s.text)
	}
	return sb.String()
}

// truncateWarning 生成截断告警：超限比例 + 截断动作列表（超出 maxWarnList
// 条时压缩为前几条 + 总数）。比例与截断后长度均按 rune 数（站点限制语义）。
func truncateWarning(origLen int, req string, warns []string) string {
	overLen := origLen - maxUserInputLen
	if overLen < 0 {
		overLen = 0
	}
	warning := fmt.Sprintf("⚠️ 审查输入过长（超出约 %d%%），已自动截断至 %d 字符。", overLen*100/maxUserInputLen, countRunes(req))
	if len(warns) > 0 {
		warning += " "
		if len(warns) > maxWarnList {
			warning += strings.Join(warns[:maxWarnList], "；") + fmt.Sprintf("；…等共 %d 个", len(warns))
		} else {
			warning += strings.Join(warns, "；")
		}
		warning += "。"
	}
	return warning
}
