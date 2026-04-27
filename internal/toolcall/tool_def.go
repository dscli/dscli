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
// 1. string              - string
// 2. float64, float64    - number
// 3. int64, int32, int   - integer
// 4. bool                - boolean
// 5. []string                    - array with item type string
// 6. []float64, []float32        - array with item type float64, float32
// 7. []int64, []int32, []int     - array with item type int64, int32, int
// 8. []bool                      - array with item type bool

type Primitive interface {
	PrimitiveType | ArrayType
}

type PrimitiveType interface {
	~string | ~float64 | ~float32 | ~int64 | ~int32 | ~int | ~bool
}

type ArrayType interface {
	~[]string | ~[]float64 | ~[]float32 | ~[]int64 | ~[]int32 | ~[]int | ~[]bool
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
		// 安全的类型转换，避免panic
		switch any(*new(T)).(type) {
		case string:
			if str, ok := v.(string); ok {
				sa[i] = any(str).(T)
			} else {
				// 对于非字符串类型，尝试转换为字符串
				sa[i] = any(fmt.Sprint(v)).(T)
			}
		case float64:
			switch val := v.(type) {
			case float64:
				sa[i] = any(val).(T)
			default:
				// 无法转换，使用零值
				sa[i] = *new(T)
			}
		case float32:
			switch val := v.(type) {
			case float64:
				sa[i] = any(float32(val)).(T)
			default:
				// 无法转换，使用零值
				sa[i] = *new(T)
			}

		case int64:
			switch val := v.(type) {
			case float64:
				// 注意：这里会截断小数部分
				sa[i] = any(int64(val)).(T)
			default:
				// 无法转换，使用零值
				sa[i] = *new(T)
			}
		case int32:
			switch val := v.(type) {
			case float64:
				// 注意：这里会截断小数部分
				sa[i] = any(int32(val)).(T)
			default:
				// 无法转换，使用零值
				sa[i] = *new(T)
			}
		case int:
			switch val := v.(type) {
			case float64:
				// 注意：这里会截断小数部分
				sa[i] = any(int(val)).(T)
			default:
				// 无法转换，使用零值
				sa[i] = *new(T)
			}
		case bool:
			if b, ok := v.(bool); ok {
				sa[i] = any(b).(T)
			} else {
				// 无法转换，使用零值
				sa[i] = *new(T)
			}
		}
	}
	return sa
}

// ToolArgsValue 安全获取类型化参数值
func ToolArgsValue[T Primitive](args ToolArgs, key string, defaultValue T) T {
	var (
		value       any
		ok          bool
		anyValues   []any
		floatValue  float64
		stringValue string
	)

	if value, ok = args[key]; ok {
		// 尝试将字符串解析为JSON值（处理智能体误将数组序列化为字符串的情况）
		if stringValue, ok = value.(string); ok {
			// 尝试将字符串解析为JSON
			var decoded any
			if err := json.Unmarshal([]byte(stringValue), &decoded); err == nil {
				// 只接受解析出的数组类型，避免将JSON对象/数字等误解析导致string参数丢失
				if decodedArray, ok := decoded.([]any); ok {
					value = decodedArray
				}
			}
		}

		if anyValues, ok = value.([]any); ok {
			anyValuesLen := len(anyValues)
			if anyValuesLen != 0 {
				// 检查第一个元素的类型来决定转换目标类型
				firstValue := anyValues[0]
				switch firstValue.(type) {
				case string:
					value = ToArrayType[string](anyValues, anyValuesLen)
				case float64:
					switch any(defaultValue).(type) {
					case []float64:
						value = ToArrayType[float64](anyValues, anyValuesLen)
					case []float32:
						value = ToArrayType[float32](anyValues, anyValuesLen)
					case []int64:
						value = ToArrayType[int64](anyValues, anyValuesLen)
					case []int32:
						value = ToArrayType[int32](anyValues, anyValuesLen)
					case []int:
						value = ToArrayType[int](anyValues, anyValuesLen)
					}
				case bool:
					value = ToArrayType[bool](anyValues, anyValuesLen)
				default:
					// 无法识别的类型，保持原样
				}
			}
		}

		if floatValue, ok = value.(float64); ok {
			switch any(defaultValue).(type) {
			case float32:
				value = float32(floatValue)
			case int64:
				value = int64(floatValue)
			case int32:
				value = int32(floatValue)
			case int:
				value = int(floatValue)
			}
		}
	}

	if typedValue, ok := value.(T); ok {
		return typedValue
	}

	return defaultValue
}
