package context

import (
	"context"
	"io"
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
