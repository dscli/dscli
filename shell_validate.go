package main

import (
	"fmt"
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

func parseCommands(script string) (commands []string, err error) {
	parser := syntax.NewParser()
	reader := strings.NewReader(script)
	sf, err := parser.Parse(reader, "script.sh")
	if err != nil {
		return
	}
	syntax.Walk(sf, func(node syntax.Node) bool {
		// 我们关心的是 *syntax.CallExpr 节点，它代表一个命令调用
		if call, ok := node.(*syntax.CallExpr); ok {
			// 命令名通常是 Args 切片中的第一个参数
			if len(call.Args) > 0 {
				// Args[0] 是一个 *syntax.Word，我们需要获取它的字面值
				// Lit() 方法可以返回单词的字面字符串
				cmdName := call.Args[0].Lit()
				if cmdName != "" {
					commands = append(commands, cmdName)
				}
			}
		}
		// 返回 true 表示继续遍历子节点
		return true
	})
	return
}

// checkDangerousCommands 检查脚本中是否包含危险命令
func checkDangerousCommands(script string) error {
	// 转换为小写进行不区分大小写的检查
	lowerScript := strings.ToLower(script)

	for _, dangerousCmd := range dangerousCommands {
		if strings.Contains(lowerScript, strings.ToLower(dangerousCmd)) {
			return fmt.Errorf("检测到危险命令: %s", dangerousCmd)
		}
	}

	return nil
}

func validateShell(script string) (err error) {
	// 检查危险命令
	if err := checkDangerousCommands(script); err != nil {
		return err
	}

	// 检查是否运行 dscli
	commands, err := parseCommands(script)
	if err != nil {
		return err
	}

	if slices.Contains(commands, "./dscli") {
		return fmt.Errorf("do not run dscli in dscli")
	}

	return nil
}
