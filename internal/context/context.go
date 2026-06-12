package context

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/dscli/dscli/internal/config"
)

var (
	HistSizeKey           = ContextKeyType[int]{"HistSize"}
	StartTimeKey          = ContextKeyType[time.Time]{"StartTime"}
	StartBalanceKey       = ContextKeyType[map[string]string]{"StartBalance"}
	CurrentModelIDKey     = ContextKeyType[int64]{"CurrentModelID"}
	CurrentModelNameKey   = ContextKeyType[string]{"CurrentModelName"}
	CurrentDomainIDKey    = ContextKeyType[int64]{"CurrentDomainID"}
	CurrentRoleKey        = ContextKeyType[string]{"CurrentRole"}
	ShellSummaryKey       = ContextKeyType[string]{"ShellSummary"}
	ShellArgsKey          = ContextKeyType[[]string]{"ShellArgs"}
	StreamKey             = ContextKeyType[bool]{"Stream"}
	LeftTokensKey         = ContextKeyType[int]{"LeftTokens"}
	FinishReasonLengthKey = ContextKeyType[bool]{"FinishReasonLength"}
	IsChildProcessKey     = ContextKeyType[bool]{"IsChildProcess"}
	AINameCNKey           = ContextKeyType[string]{"AINameCN"}
	AINameENKey           = ContextKeyType[string]{"AINameEN"}
	AINameEmailKey        = ContextKeyType[string]{"AINameEmail"}
	UserIDKey             = ContextKeyType[string]{"UserID"}
	AINameBirdFrogKey     = ContextKeyType[string]{"AINameBirdFrog"}
	GitUserNameKey        = ContextKeyType[string]{"GitUserName"}
	GitUserEmailKey       = ContextKeyType[string]{"GitUserEmail"}
	KeepKey               = ContextKeyType[bool]{"Keep"}
)

var (
	WithTimeout      = context.WithTimeout
	WithValue        = context.WithValue
	WithCancel       = context.WithCancel
	DeadlineExceeded = context.DeadlineExceeded
	Canceled         = context.Canceled
	Background       = context.Background
	TODO             = context.TODO
)

var (
	ProjectRoot       = GetProjectRoot()
	ModelDeepseekChat = config.Get("model-deepseek-chat", "deepseek-v4-flash")
)

const (
	DeepseekChat = int64(0)
)

// Role constants for --role flag
const (
	RoleDev    = "dev"
	RoleExpert = "expert"
	RoleReview = "review"
	RoleTest   = "test"
)

type (
	Context    = context.Context
	CancelFunc = context.CancelFunc
)

type ContextKeyType[T any] struct {
	name string
}

func ContextValue[T any](ctx context.Context, k ContextKeyType[T], d T) (v T) {
	v, ok := ctx.Value(k).(T)
	if ok {
		return v
	}
	return d
}

func GetProjectRoot() (projectRoot string) {
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	gitRoot, err := findGitRoot(cwd)
	if err != nil {
		gitRoot = cwd
	}
	projectRoot, err = filepath.Abs(gitRoot)
	if err != nil {
		panic(err)
	}
	// 注意：不在此处切换 CWD。
	// Shell 相关工具通过 interp.Dir 设置工作目录，
	// 其他工具通过 context.ProjectRoot 解析相对路径。
	// 保持进程 CWD 不变，避免影响用户指定的相对路径（如 skill add）。
	return projectRoot
}

func findGitRoot(dir string) (string, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		gitPath := filepath.Join(absDir, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			return absDir, nil
		}
		parent := filepath.Dir(absDir)
		if parent == absDir {
			break
		}
		absDir = parent
	}
	return "", fmt.Errorf("未找到 Git 仓库根目录")
}

func IsTesting() bool {
	return strings.HasSuffix(os.Args[0], ".test")
}

// GitUserName returns git config user.name, or "未知" on error.
func GitUserName() string {
	output, err := exec.Command("git", "config", "user.name").Output()
	if err != nil {
		return "未知"
	}
	return strings.TrimSpace(string(output))
}

// GitUserEmail returns git config user.email, or "未知" on error.
func GitUserEmail() string {
	output, err := exec.Command("git", "config", "user.email").Output()
	if err != nil {
		return "未知"
	}
	return strings.TrimSpace(string(output))
}
