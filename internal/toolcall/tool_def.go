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
// 5. []string  - array with item type string
// 6. []float64 - array with item type float64
// 7. []int64   - array with item type int64
// 8. []bool    - array with item type bool

type Primitive interface {
	PrimitiveType | ArrayType
}

type PrimitiveType interface {
	~string | ~float64 | ~int64 | ~bool
}

type ArrayType interface {
	~[]string | ~[]float64 | ~[]int64 | ~[]bool
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

func ToArrayType[T PrimitiveType](anyValues []any, anyValuesLen int) []T {
	sa := make([]T, anyValuesLen)
	for i, v := range anyValues {
		sa[i] = v.(T)
	}
	return sa
}

// ToolArgsValue 安全获取类型化参数值
func ToolArgsValue[T Primitive](args ToolArgs, key string, defaultValue T) T {
	var (
		value     any
		ok        bool
		anyValues []any
	)

	if value, ok = args[key]; ok {
		if anyValues, ok = value.([]any); ok {
			anyValuesLen := len(anyValues)
			if anyValuesLen != 0 {
				anyValue := anyValues[0]
				switch anyValue.(type) {
				case string:
					value = ToArrayType[string](anyValues, anyValuesLen)
				case float64:
					value = ToArrayType[float64](anyValues, anyValuesLen)
				case int64:
					value = ToArrayType[int64](anyValues, anyValuesLen)
				case bool:
					value = ToArrayType[bool](anyValues, anyValuesLen)
				}
			}
		}
	}

	if typedValue, ok := value.(T); ok {
		return typedValue
	}
	return defaultValue
}

func TitleLikePattern(maxLength int) string {
	return fmt.Sprintf("^[^\\n\\r]{1,%d}$", maxLength)
}

func ContentLikePattern(maxLength int) string {
	return fmt.Sprintf("^[\\s\\S]{1,%d}$", maxLength)
}
