package file

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	// 项目惯例：只导入 internal/context（它别名了标准库 context 并提供
	// Background/Context)，避免双导入陷阱（见 fix-context-import 技能）。
	"github.com/dscli/dscli/internal/context"
)

// newApplyPatchRepo 创建临时 git 仓库：初始化 + 提交初始文件，返回仓库根。
// git apply 需要仓库上下文（非 git 目录会失败），测试统一从此基线出发。
func newApplyPatchRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	repo := t.TempDir()
	git(t, repo, "init", "-q")
	for name, content := range files {
		path := filepath.Join(repo, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	git(t, repo, "add", "-A")
	git(t, repo, "-c", "user.name=test", "-c", "user.email=test@test", "commit", "-qm", "init")
	return repo
}

// git 运行 git 命令，失败时输出命令与 stderr（t.Helper 定位调用行）。
func git(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmdArgs := append([]string{"-C", repo}, args...)
	cmd := exec.Command("git", cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// mustReadFile 读取仓库内文件（错误即 fail）。
func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// makePatch 在 repo 中执行 mutate 修改工作区，生成补丁文本（git diff），
// 然后恢复工作区到提交状态（reset 清除 intent 标记 + checkout + clean 未
// 跟踪文件），返回补丁——即"补丁应用前的现场"与"补丁应用后的现场"一致。
// 注意：git diff 不显示未跟踪文件，新增文件的 mutate 需自行 git add -N。
func makePatch(t *testing.T, repo string, mutate func()) string {
	t.Helper()
	mutate()
	patch := git(t, repo, "diff")
	git(t, repo, "reset", "-q")
	git(t, repo, "checkout", "--", ".")
	git(t, repo, "clean", "-fdq")
	return patch
}

// setProjectRoot 临时覆盖 context.ProjectRoot 并在结束时恢复。
func setProjectRoot(t *testing.T, root string) {
	t.Helper()
	old := context.ProjectRoot
	context.ProjectRoot = root
	t.Cleanup(func() { context.ProjectRoot = old })
}

func TestHandleApplyPatchModify(t *testing.T) {
	repo := newApplyPatchRepo(t, map[string]string{"a.txt": "hello\n"})
	setProjectRoot(t, repo)

	patch := makePatch(t, repo, func() {
		if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("hello\nworld\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	})

	result, _, err := handleApplyPatch(context.Background(), ToolArgs{"patch": patch})
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if !strings.Contains(result, `"applied":true`) {
		t.Errorf("result missing applied=true: %s", result)
	}
	if !strings.Contains(result, `"check_only":false`) {
		t.Errorf("result missing check_only=false: %s", result)
	}
	if !strings.Contains(result, "a.txt") {
		t.Errorf("changed_files should list a.txt: %s", result)
	}
	if !strings.Contains(result, "1 file changed") {
		t.Errorf("summary missing: %s", result)
	}
	if got := mustReadFile(t, filepath.Join(repo, "a.txt")); got != "hello\nworld\n" {
		t.Errorf("file not modified: %q", got)
	}
}

func TestHandleApplyPatchCreateAndDelete(t *testing.T) {
	repo := newApplyPatchRepo(t, map[string]string{"a.txt": "hello\n", "gone.txt": "bye\n"})
	setProjectRoot(t, repo)

	// 新增 b.txt、删除 gone.txt（新增文件需 add -N 才进入 git diff）。
	patch := makePatch(t, repo, func() {
		if err := os.WriteFile(filepath.Join(repo, "b.txt"), []byte("new\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.Join(repo, "gone.txt")); err != nil {
			t.Fatal(err)
		}
		git(t, repo, "add", "-N", "b.txt")
	})

	result, _, err := handleApplyPatch(context.Background(), ToolArgs{"patch": patch})
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if !strings.Contains(result, `"applied":true`) {
		t.Errorf("result: %s", result)
	}
	if !strings.Contains(result, "b.txt") {
		t.Errorf("changed_files should include b.txt: %s", result)
	}
	if mustReadFile(t, filepath.Join(repo, "b.txt")) != "new\n" {
		t.Error("b.txt not created")
	}
	if _, err := os.Stat(filepath.Join(repo, "gone.txt")); os.IsNotExist(err) == false {
		t.Error("gone.txt not deleted")
	}
}

func TestHandleApplyPatchCheckOnly(t *testing.T) {
	repo := newApplyPatchRepo(t, map[string]string{"a.txt": "hello\n"})
	setProjectRoot(t, repo)

	patch := makePatch(t, repo, func() {
		if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("hello\nworld\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	})

	result, _, err := handleApplyPatch(context.Background(), ToolArgs{"patch": patch, "check": true})
	if err != nil {
		t.Fatalf("check failed: %v", err)
	}
	if !strings.Contains(result, `"check_only":true`) {
		t.Errorf("result missing check_only=true: %s", result)
	}
	if !strings.Contains(result, "a.txt") {
		t.Errorf("check result should list would-be files: %s", result)
	}
	// check 模式：工作区保持提交基线（patch 未应用）。
	if got := mustReadFile(t, filepath.Join(repo, "a.txt")); got != "hello\n" {
		t.Errorf("check mode must not write: %q", got)
	}
}

func TestHandleApplyPatchReverse(t *testing.T) {
	repo := newApplyPatchRepo(t, map[string]string{"a.txt": "hello\n"})
	setProjectRoot(t, repo)

	patch := makePatch(t, repo, func() {
		if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("hello\nworld\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	})

	if _, _, err := handleApplyPatch(context.Background(), ToolArgs{"patch": patch}); err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	// 反向应用撤销。
	result, _, err := handleApplyPatch(context.Background(), ToolArgs{"patch": patch, "reverse": true})
	if err != nil {
		t.Fatalf("reverse failed: %v", err)
	}
	if !strings.Contains(result, `"applied":true`) {
		t.Errorf("result: %s", result)
	}
	if got := mustReadFile(t, filepath.Join(repo, "a.txt")); got != "hello\n" {
		t.Errorf("reverse should restore original: %q", got)
	}
}

func TestHandleApplyPatchAtomicConflict(t *testing.T) {
	repo := newApplyPatchRepo(t, map[string]string{"a.txt": "a1\n", "b.txt": "b1\n"})
	setProjectRoot(t, repo)

	// 补丁修改 a.txt（追加 a2）与 b.txt（追加 b2）。
	patch := makePatch(t, repo, func() {
		if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a1\na2\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repo, "b.txt"), []byte("b1\nb2\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	})

	// 让 b.txt 与补丁上下文冲突：工作区改成 b3（补丁期望 b1\nb2\n）。
	if err := os.WriteFile(filepath.Join(repo, "b.txt"), []byte("b1\nb2\nb3\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err := handleApplyPatch(context.Background(), ToolArgs{"patch": patch})
	if err == nil {
		t.Fatal("conflicting patch should fail")
	}
	// 原子性：a.txt 必须未被修改（patch 未部分应用）。
	if got := mustReadFile(t, filepath.Join(repo, "a.txt")); got != "a1\n" {
		t.Errorf("atomicity broken: a.txt = %q", got)
	}
	if !strings.Contains(err.Error(), "b.txt") {
		t.Errorf("conflict error should mention b.txt: %v", err)
	}
}

func TestHandleApplyPatchPatchFileArg(t *testing.T) {
	repo := newApplyPatchRepo(t, map[string]string{"a.txt": "hello\n"})
	setProjectRoot(t, repo)

	patch := makePatch(t, repo, func() {
		if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("hello\nworld\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	patchPath := filepath.Join(repo, "change.patch")
	if err := os.WriteFile(patchPath, []byte(patch), 0o644); err != nil {
		t.Fatal(err)
	}

	result, _, err := handleApplyPatch(context.Background(), ToolArgs{"patch": patchPath})
	if err != nil {
		t.Fatalf("patch-file apply failed: %v", err)
	}
	if !strings.Contains(result, `"applied":true`) {
		t.Errorf("result: %s", result)
	}
	if mustReadFile(t, filepath.Join(repo, "a.txt")) != "hello\nworld\n" {
		t.Error("patch file content not applied")
	}
}

func TestHandleApplyPatchCwdEscapesProjectRoot(t *testing.T) {
	repo := newApplyPatchRepo(t, map[string]string{"a.txt": "hello\n"})
	setProjectRoot(t, repo)

	patch := makePatch(t, repo, func() {
		if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("hello\nworld\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	})

	// cwd 逃逸到项目外（/tmp 下的其他目录）。
	_, _, err := handleApplyPatch(context.Background(), ToolArgs{"patch": patch, "cwd": "/tmp"})
	if err == nil || !strings.Contains(err.Error(), "outside the project root") {
		t.Fatalf("cwd escape must be rejected, got: %v", err)
	}
}

func TestHandleApplyPatchCwdOtherRepoOutside(t *testing.T) {
	repo := newApplyPatchRepo(t, map[string]string{"a.txt": "hello\n"})
	setProjectRoot(t, repo)

	patch := makePatch(t, repo, func() {
		if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("hello\nworld\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	})

	// 项目外的另一个 git 仓库：即使在项目根外是合法仓库也必须被拒绝。
	other := newApplyPatchRepo(t, map[string]string{"x.txt": "x\n"})
	if other == repo {
		t.Fatal("sanity: repos must differ")
	}
	_, _, err := handleApplyPatch(context.Background(), ToolArgs{"patch": patch, "cwd": other})
	if err == nil || !strings.Contains(err.Error(), "outside the project root") {
		t.Fatalf("external repo cwd must be rejected, got: %v", err)
	}
}

func TestHandleApplyPatchNotGitRepo(t *testing.T) {
	// 项目根本身不是 git 仓库：git apply 无仓库上下文，必须拒绝。
	repo := t.TempDir()
	setProjectRoot(t, repo)

	// 非 git 目录无法 git diff，直接手写合法 unified diff。
	patch := "--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-x\n+y\n"

	_, _, err := handleApplyPatch(context.Background(), ToolArgs{"patch": patch})
	if err == nil || !strings.Contains(err.Error(), "not inside a git repository") {
		t.Fatalf("non-git root must be rejected, got: %v", err)
	}
}

func TestHandleApplyPatchProtectedFile(t *testing.T) {
	repo := newApplyPatchRepo(t, map[string]string{"a.txt": "hello\n"})
	setProjectRoot(t, repo)

	// 手写合法 unified diff（针对未跟踪的敏感文件）。
	patch := "--- a/sqlite.db\n+++ b/sqlite.db\n@@ -1 +1 @@\n-old\n+new\n"
	_, _, err := handleApplyPatch(context.Background(), ToolArgs{"patch": patch})
	if err == nil || !strings.Contains(err.Error(), "protected file") {
		t.Fatalf("sqlite.db patch must be refused, got: %v", err)
	}

	patch2 := "--- a/dscli.env\n+++ b/dscli.env\n@@ -1 +1 @@\n-old\n+new\n"
	_, _, err = handleApplyPatch(context.Background(), ToolArgs{"patch": patch2})
	if err == nil || !strings.Contains(err.Error(), "protected file") {
		t.Fatalf("dscli.env patch must be refused, got: %v", err)
	}
}

func TestHandleApplyPatchMissingPatch(t *testing.T) {
	_, _, err := handleApplyPatch(context.Background(), ToolArgs{})
	if err == nil {
		t.Fatal("missing patch must fail")
	}
}

func TestHandleApplyPatchPathTraversal(t *testing.T) {
	repo := newApplyPatchRepo(t, map[string]string{"a.txt": "hello\n"})
	setProjectRoot(t, repo)

	// ../ 逃逸 patch 必须在工具层被拒绝（不依赖 git apply）。
	patch := "--- a/../outside.txt\n+++ b/../outside.txt\n@@ -1 +1 @@\n-x\n+y\n"
	_, _, err := handleApplyPatch(context.Background(), ToolArgs{"patch": patch})
	if err == nil || !strings.Contains(err.Error(), "escapes the working tree") {
		t.Fatalf("path traversal must be rejected, got: %v", err)
	}
}

func TestParseApplyStat(t *testing.T) {
	out := " a.txt | 2 +-\n 1 file changed, 1 insertion(+), 1 deletion(-)\n"
	summary, files := parseApplyStat(out)
	if summary != "1 file changed, 1 insertion(+), 1 deletion(-)" {
		t.Errorf("summary: %q", summary)
	}
	if len(files) != 1 || files[0] != "a.txt" {
		t.Errorf("files: %v", files)
	}

	multi := " dir/a.go | 10 +++---\n b.go | 2 +-\n 2 files changed, 11 insertions(+), 1 deletion(-)\n"
	summary, files = parseApplyStat(multi)
	if !strings.Contains(summary, "2 files changed") {
		t.Errorf("summary: %q", summary)
	}
	if len(files) != 2 || files[0] != "dir/a.go" || files[1] != "b.go" {
		t.Errorf("files: %v", files)
	}
}
