package toolcall

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// ToolDef 工具定义
type ToolDef struct {
	Name        string
	DisplayName string
	Description string
	Strict      bool
	Parameters  map[string]any
	Category    string
	Timeout     time.Duration // 工具执行超时时间
	Handler     func(ctx context.Context, args ToolArgs) (string, string, error)
}

// ToolArgs 参数定义
type ToolArgs map[string]any

// Primitive types tool arguments support:
// 1. string   - string
// 2. float64  - number
// 3. int64    - integer
// 4. bool     - boolean
// 5. []string - array
type Primitive interface {
	~string | ~float64 | ~int64 | ~bool | ~[]string
}

func Error(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

type ToolContent struct {
	Result     string `json:"result,omitzero"`
	Error      string `json:"error,omitzero"`
	Suggestion string `json:"suggestion,omitzero"`
}

func (tc *ToolContent) String() (content string) {
	b, err := json.MarshalIndent(tc, "", " ")
	if err != nil {
		return
	}
	content = string(b)
	return
}

// ToolArgsValue 安全获取类型化参数值
func ToolArgsValue[T Primitive](args ToolArgs, key string, defaultValue T) T {
	if value, ok := args[key]; ok {
		if typedValue, ok := value.(T); ok {
			return typedValue
		}
	}
	return defaultValue
}

func TitleLikePattern(maxLength int) string {
	return fmt.Sprintf("^[^\\n\\r]{1,%d}$", maxLength)
}

func ContentLikePattern(maxLength int) string {
	return fmt.Sprintf("^[\\s\\S]{1,%d}$", maxLength)
}
