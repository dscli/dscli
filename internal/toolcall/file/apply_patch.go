package file

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/toolcall"
	"github.com/nanjj/clog"
)

//go:embed apply_patch.md
var apply_patch_md string

func init() {
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "apply_patch",
		Description: apply_patch_md,
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"patch": map[string]any{
					"type":        "string",
					"description": "Unified diff text; or a path to a .patch/.diff file to read",
				},
				"cwd": map[string]any{
					"type":        "string",
					"description": "Git repository directory, default project root; must stay inside the project root",
				},
				"check": map[string]any{
					"type":        "boolean",
					"description": "true = dry-run only (git apply --check), no writes",
				},
				"reverse": map[string]any{
					"type":        "boolean",
					"description": "true = reverse-apply (undo, git apply -R)",
				},
			},
			"required":             []string{"patch"},
			"additionalProperties": false,
		},
		Category: "file_ops",
		Timeout:  60 * time.Second,
		Handler:  handleApplyPatch,
	})
}

// applyPatchResult 是 apply_patch 的返回值载荷。失败时 error 字段描述
// 冲突文件与行号（经执行器外层 error 字段呈现，见 handleApplyPatch）。
type applyPatchResult struct {
	Applied      bool     `json:"applied"`
	CheckOnly    bool     `json:"check_only"`
	ChangedFiles []string `json:"changed_files"`
	Summary      string   `json:"summary"`
	Error        string   `json:"error"`
}

// maxPatchFileSize 是 patch 参数作为文件读取时的上限（4 MiB）。超出视为
// 非法 diff 文本而不是静默读取巨型文件占用内存。
const maxPatchFileSize = 4 << 20

// applyPatchPlusRe 提取 patch 中每个文件的目标路径（+++ b/... 行）。
// 删除文件的 patch 目标为 /dev/null，跳过。路径逃逸与敏感文件在此预检，
// 早于 git apply 给出更友好的错误。
var applyPatchPlusRe = regexp.MustCompile(`(?m)^\+\+\+ (?:b/)?(.+)$`)

// handleApplyPatch 把 unified diff 应用到 git 工作区（git apply 语义）。
//
// 设计要点：
//   - AWS 无关的原子性：git apply 默认整体成功或整体失败（不传 --reject），
//     冲突时任何文件都不会被部分修改。
//   - patch 经 stdin 传入（git apply -），完全规避 shell 转义与命令行
//     长度限制；这也是文档强调"独立工具规避 shell 转义问题"的原因。
//   - check=true 只做预检（git apply --check）并返回将要修改的文件，
//     不落盘；reverse=true 对应 git apply -R。
//   - cwd 必须在项目根内（防远程模型对项目外任意 git 仓库写入），且
//     必须是 git 仓库；patch 目标为 sqlite.db/dscli.env 时拒绝。
func handleApplyPatch(ctx context.Context, args ToolArgs) (result, warning string, err error) {
	span, ctx := clog.StartSpanFromContext(ctx, "handleApplyPatch")
	defer span.Finish()

	patch := toolcall.ToolArgsValue(args, "patch", "")
	if strings.TrimSpace(patch) == "" {
		err = fmt.Errorf("parameter error: no patch specified")
		return result, warning, err
	}

	// patch 参数若为单个已存在文件路径（.patch/.diff），读取其内容。
	// 多行文本必然是 diff 内容本身，不当作路径处理。文件路径必须位于
	// 项目根内（同 cwd 安全姿态：远端模型不能借 patch-as-path 读取
	// 项目外任意文件）。
	if !strings.Contains(patch, "\n") {
		// 相对路径基于项目根解析（与 file 包其他工具一致，ResolvePath）。
		fullPath := ResolvePath(ctx, patch)
		if fi, statErr := os.Stat(fullPath); statErr == nil && !fi.IsDir() {
			if err = requirePatchFileInsideProject(fullPath); err != nil {
				return result, warning, err
			}
			if fi.Size() > maxPatchFileSize {
				err = fmt.Errorf("patch file %q too large (%d bytes, max %d)", patch, fi.Size(), maxPatchFileSize)
				return result, warning, err
			}
			data, readErr := os.ReadFile(fullPath)
			if readErr != nil {
				err = fmt.Errorf("failed to read patch file %q: %w", patch, readErr)
				return result, warning, err
			}
			patch = string(data)
		}
	}

	cwd, err := resolveApplyPatchCWD(ctx, args)
	if err != nil {
		return result, warning, err
	}

	// 目标路径预检（逃逸/敏感文件），早于 git apply 报错，错误可读性好。
	if err = checkApplyPatchTargets(patch); err != nil {
		return result, warning, err
	}

	check := boolArg(args, "check", false)
	reverse := boolArg(args, "reverse", false)

	// 预检：固定 git apply --check（不写盘），与最终应用使用相同
	// reverse 语义。注意这里必须传 true——若误用用户的 check 参数，
	// 未传 check 时第一次调用就已经真实应用，第二次再应用必失败。
	if verifyErr := gitApplyDiff(ctx, cwd, patch, true, reverse); verifyErr != nil {
		return result, warning, verifyErr
	}

	// 摘要：git apply --stat 不修改文件，可独立于应用执行。
	summary, files, statErr := gitApplyStat(ctx, cwd, patch, reverse)
	if statErr != nil {
		// check 已通过仍失败说明 patch 语法奇异（如二进制 patch 的 stat
		// 变体），此时不阻塞应用本身，仅降级为空摘要。
		outfmt.Debug("apply_patch --stat failed: %v", statErr)
	}

	if check {
		outfmt.Notice("apply_patch 预检通过（未写入）：%s（%d 个文件）", summaryOr(summary, len(files)), len(files))
		result = marshalApplyPatchResult(applyPatchResult{
			Applied:      true,
			CheckOnly:    true,
			ChangedFiles: files,
			Summary:      summary,
		})
		return result, warning, err
	}

	// 正式应用（check 已通过；失败仅可能是并发修改，属罕见竞态）。
	if applyErr := gitApplyDiff(ctx, cwd, patch, false, reverse); applyErr != nil {
		return result, warning, applyErr
	}

	outfmt.Notice("apply_patch 已应用：%s（%d 个文件）", summaryOr(summary, len(files)), len(files))
	result = marshalApplyPatchResult(applyPatchResult{
		Applied:      true,
		CheckOnly:    false,
		ChangedFiles: files,
		Summary:      summary,
	})
	return result, warning, err
}

// marshalApplyPatchResult 序列化工具返回值（JSON 文本）。
func marshalApplyPatchResult(r applyPatchResult) string {
	b, err := json.Marshal(r)
	if err != nil { // unreachable: 字段均为 JSON 安全类型
		return fmt.Sprintf(`{"applied":false,"error":%q}`, err.Error())
	}
	return string(b)
}

// summaryOr 给出摘要，stat 解析失败时以文件数兜底。
func summaryOr(summary string, n int) string {
	if summary != "" {
		return summary
	}
	return fmt.Sprintf("%d file(s)", n)
}

// boolArg 从 args 读取布尔参数，容忍 DSML 解码器的数值/字符串变体
// （如 "1"、"yes" 或 float64(1)）。
func boolArg(args ToolArgs, key string, def bool) bool {
	v, ok := args[key]
	if !ok {
		return def
	}
	switch x := v.(type) {
	case bool:
		return x
	case float64:
		return x != 0
	case string:
		b, err := parseBoolLoose(x)
		if err == nil {
			return b
		}
	}
	return def
}

// parseBoolLoose 宽松解析布尔字符串（Go strconv.ParseBool 之外增加
// 1/0/yes/no/on/off）。
func parseBoolLoose(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on", "y":
		return true, nil
	case "0", "false", "no", "off", "n":
		return false, nil
	}
	return false, fmt.Errorf("not a boolean: %q", s)
}

// resolveApplyPatchCWD 解析 cwd 参数：默认项目根；显式 cwd 必须解析后
// 位于项目根内且是 git 仓库（向上查找），否则拒绝——工具面向不可信
// 的远程模型，不能允许对项目外的仓库写文件。
//
// 安全：项目根与 cwd 都先做 EvalSymlinks（符号链接解析）再校验相对关系，
// 否则项目根内一个指向外部仓库的软链接（如 proj/link -> /tmp/otherrepo）
// 就能绕过词法 Rel 检查——exec_command 可先 ln -s 制造该链接。gitTopLevel
// 返回的仓库顶层同样必须落在项目根内（仅打日志不够，-C 仍会落到外部）。
func resolveApplyPatchCWD(ctx context.Context, args ToolArgs) (string, error) {
	root := context.ProjectRoot
	if root == "" {
		var cwdErr error
		if root, cwdErr = os.Getwd(); cwdErr != nil {
			return "", fmt.Errorf("apply_patch: cannot determine project root: %w", cwdErr)
		}
	}
	if abs, absErr := filepath.Abs(root); absErr == nil {
		root = abs
	}

	raw := toolcall.ToolArgsValue(args, "cwd", "")
	if raw == "" {
		raw = root
	} else if strings.HasPrefix(raw, "~") {
		if home, homeErr := os.UserHomeDir(); homeErr == nil {
			raw = filepath.Join(home, raw[1:])
		}
	} else if !filepath.IsAbs(raw) {
		raw = filepath.Join(root, raw)
	}

	abs, absErr := filepath.Abs(raw)
	if absErr != nil {
		return "", fmt.Errorf("apply_patch: cannot resolve cwd %q: %w", raw, absErr)
	}

	// 符号链接解析后校验：拒绝任何指向项目根外目录的 cwd（含软链接）。
	rootReal, realErr := filepath.EvalSymlinks(root)
	if realErr != nil {
		// 项目根不存在时 filepath.Rel 也无意义；报可读错误。
		return "", fmt.Errorf("apply_patch: cannot resolve project root %q: %w", root, realErr)
	}
	absReal, realErr := filepath.EvalSymlinks(abs)
	if realErr != nil {
		return "", fmt.Errorf("apply_patch: cannot resolve cwd %q: %w", abs, realErr)
	}
	if err := requireInsideRoot(absReal, rootReal, "cwd"); err != nil {
		return "", err
	}

	// git 仓库校验：git apply 需要仓库上下文。cwd 必须是仓库根——
	// git apply 在仓库内子目录运行时 patch 路径相对子目录解析（实测
	// 子目录下 `git apply` 对相对仓库根的路径会"跳过补丁"且 exit 0，
	// 静默假成功），因此显式拒绝子目录，语义收敛为"cwd=仓库根，patch
	// 路径相对仓库根"。
	top, repoErr := gitTopLevel(ctx, abs)
	if repoErr != nil {
		return "", fmt.Errorf("apply_patch: %q is not inside a git repository (cwd must be a repo dir): %w", abs, repoErr)
	}
	topReal, realErr := filepath.EvalSymlinks(top)
	if realErr != nil {
		return "", fmt.Errorf("apply_patch: cannot resolve repo root %q: %w", top, realErr)
	}
	// 仓库顶层必须位于项目根内（防软链接把仓库根"抬"出项目根后仍以
	// 外层 cwd 写入）。
	if err := requireInsideRoot(topReal, rootReal, "repo"); err != nil {
		return "", err
	}
	if absReal != topReal {
		return "", fmt.Errorf("apply_patch: cwd %q must be the git repository root %q (patch paths resolve against the repo root)", abs, top)
	}
	outfmt.Debug("apply_patch: cwd=%s repo=%s", abs, top)
	return abs, nil
}

// requireInsideRoot 校验 path 解析后位于 root 内（含根自身），错误信息
// 携带原始路径便于定位。
func requireInsideRoot(path, root, what string) error {
	rel, relErr := filepath.Rel(root, path)
	if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("apply_patch: %s %q is outside the project root %q", what, path, root)
	}
	return nil
}

// requirePatchFileInsideProject 校验 patch 文件路径位于项目根内
// （符号链接解析后），与 cwd 的边界一致。
func requirePatchFileInsideProject(fullPath string) error {
	root := context.ProjectRoot
	if root == "" {
		var cwdErr error
		if root, cwdErr = os.Getwd(); cwdErr != nil {
			return fmt.Errorf("apply_patch: cannot determine project root: %w", cwdErr)
		}
	}
	if abs, absErr := filepath.Abs(root); absErr == nil {
		root = abs
	}
	absPath, absErr := filepath.Abs(fullPath)
	if absErr != nil {
		return fmt.Errorf("apply_patch: cannot resolve patch file %q: %w", fullPath, absErr)
	}
	rootReal, realErr := filepath.EvalSymlinks(root)
	if realErr != nil {
		return fmt.Errorf("apply_patch: cannot resolve project root %q: %w", root, realErr)
	}
	if pathReal, pathErr := filepath.EvalSymlinks(absPath); pathErr == nil {
		absPath = pathReal
	}
	return requireInsideRoot(absPath, rootReal, "patch file")
}

// gitTopLevel 返回 git 仓库顶层目录，验证 dir 位于仓库内。
func gitTopLevel(ctx context.Context, dir string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel")
	out, runErr := cmd.CombinedOutput()
	if runErr != nil {
		return "", fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// checkApplyPatchTargets 预检 patch 的目标文件：拒绝逃逸路径（绝对路径、
// 任何形式的 .. 段——git apply 本身也拒绝，这里提前给出可读错误）与敏感
// 文件（sqlite.db / dscli.env，项目红线）。
func checkApplyPatchTargets(patch string) error {
	for _, m := range applyPatchPlusRe.FindAllStringSubmatch(patch, -1) {
		p := strings.TrimSpace(m[1])
		if p == "" || p == "/dev/null" || p == "dev/null" {
			continue
		}
		// Clean 后 Rel(".", ...) 检测即覆盖 "../x"、"sub/../../x" 等全部
		// 逃逸形态；绝对路径先以 IsAbs 单独排除。
		clean := filepath.Clean(p)
		if filepath.IsAbs(clean) {
			return fmt.Errorf("apply_patch: patch target %q escapes the working tree", p)
		}
		if rel, relErr := filepath.Rel(".", clean); relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("apply_patch: patch target %q escapes the working tree", p)
		}
		if base := filepath.Base(clean); base == "sqlite.db" || base == "dscli.env" {
			return fmt.Errorf("apply_patch: patch target %q is a protected file (sqlite.db / dscli.env)", p)
		}
	}
	return nil
}

// gitApplyDiff 运行 git apply：check=true 时 --check（干跑）；reverse=true
// 时 --reverse（撤销）。patch 经 stdin 传入，避免 shell 转义。
func gitApplyDiff(ctx context.Context, cwd, patch string, check, reverse bool) error {
	args := []string{"-C", cwd, "apply"}
	if check {
		args = append(args, "--check")
	}
	if reverse {
		args = append(args, "--reverse")
	}
	args = append(args, "-")

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Stdin = strings.NewReader(patch)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if runErr := cmd.Run(); runErr != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = runErr.Error()
		}
		return fmt.Errorf("apply_patch: %s", msg)
	}
	return nil
}

// gitApplyStat 运行 git apply --stat（只解析 patch，不写盘），返回
// 摘要行与文件列表。
func gitApplyStat(ctx context.Context, cwd, patch string, reverse bool) (summary string, files []string, err error) {
	args := []string{"-C", cwd, "apply", "--stat"}
	if reverse {
		args = append(args, "--reverse")
	}
	args = append(args, "-")

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Stdin = strings.NewReader(patch)
	out, runErr := cmd.CombinedOutput()
	if runErr != nil {
		return "", nil, fmt.Errorf("git apply --stat: %s", strings.TrimSpace(string(out)))
	}
	summary, files = parseApplyStat(string(out))
	return summary, files, nil
}

// parseApplyStat 解析 git apply --stat 输出：
//
//	file1 | 2 +-
//	1 file changed, 1 insertion(+), 1 deletion(-)
//
// 文件行以 "|" 分隔（path | n +-）；其余含 "changed" 的行是摘要。
func parseApplyStat(out string) (summary string, files []string) {
	for _, ln := range strings.Split(strings.TrimSpace(out), "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		if i := strings.Index(ln, "|"); i > 0 {
			files = append(files, strings.TrimSpace(ln[:i]))
			continue
		}
		if summary == "" && strings.Contains(ln, "changed") {
			summary = ln
		}
	}
	return summary, files
}
