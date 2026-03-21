package toolcall

import (
	"context"
	"fmt"
	"strconv"
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
	Handler     func(ctx context.Context, args ToolArgs) (string, error)
}

// ToolArgs 参数定义
type ToolArgs map[string]any

// ToolArgsValue 安全获取类型化参数值
func ToolArgsValue[T any](args ToolArgs, key string, defaultValue T) T {
	value, exists := args[key]
	if !exists {
		return defaultValue
	}

	// 根据目标类型进行安全转换
	return convertValue(value, defaultValue)
}

// convertValue 安全类型转换
func convertValue[T any](value any, defaultValue T) T {
	var result T

	// 使用类型开关进行安全转换
	switch target := any(&result).(type) {
	case *int:
		switch v := value.(type) {
		case int:
			*target = v
		case float64:
			*target = int(v)
		case string:
			if i, err := strconv.Atoi(v); err == nil {
				*target = i
			} else {
				*target = *any(&defaultValue).(*int)
			}
		default:
			*target = *any(&defaultValue).(*int)
		}
	case *bool:
		switch v := value.(type) {
		case bool:
			*target = v
		case string:
			*target = v == "true" || v == "1"
		default:
			*target = *any(&defaultValue).(*bool)
		}
	case *string:
		switch v := value.(type) {
		case string:
			*target = v
		default:
			*target = *any(&defaultValue).(*string)
		}
	case *float64:
		switch v := value.(type) {
		case float64:
			*target = v
		case int:
			*target = float64(v)
		case string:
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				*target = f
			} else {
				*target = *any(&defaultValue).(*float64)
			}
		default:
			*target = *any(&defaultValue).(*float64)
		}
	default:
		// 尝试直接断言
		if tv, ok := value.(T); ok {
			return tv
		}
		return defaultValue
	}

	return result
}

func TitleLikePattern(maxLength int) string {
	return fmt.Sprintf("^[^\\n\\r]{1,%d}$", maxLength)
}

func ContentLikePattern(maxLength int) string {
	return fmt.Sprintf("^[\\s\\S]{1,%d}$", maxLength)
}
