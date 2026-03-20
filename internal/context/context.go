package context

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

var (
	HistSizeKey         = ContextKeyType[int]{"HistSize"}
	StartTimeKey        = ContextKeyType[time.Time]{"StartTime"}
	StartBalanceKey     = ContextKeyType[BalanceInfo]{"StartBalance"}
	CurrentModelIDKey   = ContextKeyType[int64]{"CurrentModelID"}
	CurrentModelNameKey = ContextKeyType[string]{"CurrentModelName"}
	CurrentDomainIDKey  = ContextKeyType[int64]{"CurrentDomainID"}
	CurrentSessionIDKey = ContextKeyType[int64]{"CurrentSessionID"}
	ToolCallIDKey       = ContextKeyType[string]{"ToolCallID"}
	ShellNameKey        = ContextKeyType[string]{"ShellName"}
	ShellArgsKey        = ContextKeyType[[]string]{"ShellArgs"}
	ShellStdinKey       = ContextKeyType[io.Reader]{"ShellStdin"}
	InputContentKey     = ContextKeyType[string]{"InputContent"}
	VerboseKey          = ContextKeyType[bool]{"Verbose"}
	InsideShellExecKey  = ContextKeyType[bool]{"InsideShellExec"}
	IsTestingKey        = ContextKeyType[bool]{"IsTesting"}
	StreamKey           = ContextKeyType[bool]{"Stream"}
	LeftTokensKey       = ContextKeyType[int]{"LeftTokens"}
	CodeFormatKey       = ContextKeyType[string]{"CodeFormat"}
	MakeTestKey         = ContextKeyType[string]{"MakeTest"}
	MakeBuildKey        = ContextKeyType[string]{"MakeBuild"}
	WechatFormatKey     = ContextKeyType[string]{"WechatFormat"}
	ProjectRootKey      = ContextKeyType[string]{"ProjectRoot"}
)

var (
	WithTimeout      = context.WithTimeout
	WithValue        = context.WithValue
	WithCancel       = context.WithCancel
	DeadlineExceeded = context.DeadlineExceeded
)

type (
	Context    = context.Context
	CancelFunc = context.CancelFunc
)

type BalanceInfo struct {
	Currency        string `json:"currency"`
	TotalBalance    string `json:"total_balance"`
	GrantedBalance  string `json:"granted_balance"`
	ToppedUpBalance string `json:"topped_up_balance"`
}

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

func GetConfigDir() (configDir string) {
	configDir = filepath.Join(os.Getenv("HOME"), ".dscli")
	err := os.MkdirAll(configDir, 0o755)
	if err != nil {
		panic(err)
	}
	return
}

func Getenv(key, dvalue string) (value string) {
	value = os.Getenv(key)
	if value == "" {
		value = dvalue
	}
	return
}
