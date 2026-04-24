package context

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gitcode.com/dscli/dscli/internal/config"
)

var (
	HistSizeKey           = ContextKeyType[int]{"HistSize"}
	StartTimeKey          = ContextKeyType[time.Time]{"StartTime"}
	StartBalanceKey       = ContextKeyType[map[string]string]{"StartBalance"}
	CurrentModelIDKey     = ContextKeyType[int64]{"CurrentModelID"}
	CurrentModelNameKey   = ContextKeyType[string]{"CurrentModelName"}
	CurrentDomainIDKey    = ContextKeyType[int64]{"CurrentDomainID"}
	ToolCallIDKey         = ContextKeyType[string]{"ToolCallID"}
	ShellNameKey          = ContextKeyType[string]{"ShellName"}
	ShellArgsKey          = ContextKeyType[[]string]{"ShellArgs"}
	ShellStdinKey         = ContextKeyType[io.Reader]{"ShellStdin"}
	InputContentKey       = ContextKeyType[string]{"InputContent"}
	InsideShellExecKey    = ContextKeyType[bool]{"InsideShellExec"}
	StreamKey             = ContextKeyType[bool]{"Stream"}
	LeftTokensKey         = ContextKeyType[int]{"LeftTokens"}
	WechatFormatKey       = ContextKeyType[string]{"WechatFormat"}
	ToolDisplayNameKey    = ContextKeyType[string]{"ToolDisplayName"}
	FinishReasonLengthKey = ContextKeyType[bool]{"FinishReasonLength"}
	GitWorkingDirKey      = ContextKeyType[string]{"GitWorkingDir"}
)

var (
	WithTimeout      = context.WithTimeout
	WithValue        = context.WithValue
	WithCancel       = context.WithCancel
	DeadlineExceeded = context.DeadlineExceeded
	Background       = context.Background
)

var (
	ProjectRoot           = GetProjectRoot()
	ModelDeepseekChat     = config.Get("model-deepseek-chat", "deepseek-v4-pro")
	ModelDeepseekReasoner = config.Get("model-deepseek-reasoner", "deepseek-v4-flash")
)

const (
	DeepseekChat     = int64(0)
	DeepseekReasoner = int64(1)
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

	if cwd != projectRoot {
		err = os.Chdir(projectRoot)
		if err != nil {
			panic(err)
		}
	}

	cwd, err = os.Getwd()
	if err != nil {
		panic(err)
	}

	if cwd != projectRoot {
		err = fmt.Errorf("cwd(%s) != ProjectRoot(%s)", cwd, projectRoot)
		panic(err)
	}
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

func ReasonerModelOK() bool {
	// 只要推理模型与聊天模型不同，即认为推理模型可用
	// V4: deepseek-v4-pro vs deepseek-v4-flash
	// V3: deepseek-chat vs deepseek-reasoner
	return ModelDeepseekReasoner != ModelDeepseekChat
}
