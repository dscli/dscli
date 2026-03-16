package shell

import (
	"context"
	"fmt"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// IsShellScript 判断给定的脚本是否是有效的Shell脚本
//
// 该函数使用mvdan.cc/sh/v3/syntax包来解析脚本，如果能够成功解析为有效的Shell语法，
// 则返回true。这比简单的shebang检测更准确，因为：
// 1. 不依赖shebang（shebang可能缺失或错误）
// 2. 能够检测语法错误
// 3. 能够处理复杂的Shell脚本结构
//
// 参数:
//
//	ctx: 上下文，目前未使用，为未来扩展保留
//	script: 要检查的脚本内容
//
// 返回值:
//
//	bool: 如果是有效的Shell脚本返回true，否则返回false
//	error: 解析过程中的错误信息（如果脚本不是有效的Shell脚本，会返回具体的语法错误）
func IsShellScript(ctx context.Context, script string) (bool, error) {
	// 创建语法解析器
	parser := syntax.NewParser()

	// 尝试解析脚本
	_, err := parser.Parse(strings.NewReader(script), "")
	if err != nil {
		// 如果解析失败，说明不是有效的Shell脚本
		// 返回false和具体的错误信息
		return false, fmt.Errorf("不是有效的Shell脚本: %w", err)
	}

	// 解析成功，是有效的Shell脚本
	return true, nil
}
