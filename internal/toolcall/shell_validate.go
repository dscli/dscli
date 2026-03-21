package toolcall

import (
	"fmt"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// DangerousCommand - 危险命令配置
type DangerousCommand struct {
	Command    string   // 命令名（如 "rm"）
	Args       []string // 危险参数模式（如 ["-rf", "/"]）
	ExactMatch bool     // 是否需要精确匹配
	Reason     string   // 危险原因描述
}

// 危险命令规则
var dangerousCommandRules = []DangerousCommand{
	// 系统破坏性命令
	{
		Command:    "rm",
		Args:       []string{"-rf", "/"},
		ExactMatch: true,
		Reason:     "删除根目录会导致系统崩溃",
	},
	{
		Command:    "rm",
		Args:       []string{"-rf", "~"},
		ExactMatch: true,
		Reason:     "删除家目录会导致数据丢失",
	},
	{
		Command:    "rm",
		Args:       []string{"-rf", "$HOME"},
		ExactMatch: true,
		Reason:     "删除家目录会导致数据丢失",
	},
	// 磁盘操作命令
	{
		Command: "dd",
		Args:    []string{"if=/dev/zero", "of=/dev/sd"},
		Reason:  "可能破坏磁盘数据",
	},
	{
		Command: "mkfs",
		Reason:  "格式化磁盘会导致数据丢失",
	},
	{
		Command: "fdisk",
		Reason:  "磁盘分区操作可能导致数据丢失",
	},
	// 系统关键文件操作
	{
		Command:    "chmod",
		Args:       []string{"000", "/etc"},
		ExactMatch: true,
		Reason:     "修改系统目录权限可能导致系统无法使用",
	},
	// 进程管理
	{
		Command:    "kill",
		Args:       []string{"-9", "-1"},
		ExactMatch: true,
		Reason:     "杀死所有进程会导致系统崩溃",
	},
	// 服务管理
	{
		Command:    "shutdown",
		Args:       []string{"-h", "now"},
		ExactMatch: true,
		Reason:     "立即关机可能导致数据丢失",
	},
	{
		Command: "reboot",
		Reason:  "重启系统可能中断正在运行的服务",
	},
}

// CommandInfo 表示解析出的命令信息
type CommandInfo struct {
	Name       string   // 命令名
	Args       []string // 参数列表
	FullCmd    string   // 完整命令字符串
	Line       int      // 行号
	IsExecuted bool     // 是否会被执行（排除注释、字符串中的命令）
}

// parseCommands 解析脚本中的所有命令
func parseCommands(script string) ([]CommandInfo, error) {
	parser := syntax.NewParser()
	reader := strings.NewReader(script)
	sf, err := parser.Parse(reader, "script.sh")
	if err != nil {
		return nil, err
	}

	var commands []CommandInfo
	syntax.Walk(sf, func(node syntax.Node) bool {
		switch n := node.(type) {
		case *syntax.CallExpr:
			// 提取命令信息
			cmdInfo := extractCommandInfo(n)
			if cmdInfo.Name != "" {
				commands = append(commands, cmdInfo)
			}
		}
		return true
	})

	return commands, nil
}

// extractCommandInfo 从语法节点提取命令信息
func extractCommandInfo(call *syntax.CallExpr) CommandInfo {
	var cmdInfo CommandInfo

	// 提取命令名
	if len(call.Args) > 0 {
		cmdInfo.Name = call.Args[0].Lit()

		// 提取参数
		for i := 1; i < len(call.Args); i++ {
			arg := call.Args[i].Lit()
			if arg != "" {
				cmdInfo.Args = append(cmdInfo.Args, arg)
			}
		}

		// 构建完整命令字符串
		var parts []string
		parts = append(parts, cmdInfo.Name)
		parts = append(parts, cmdInfo.Args...)
		cmdInfo.FullCmd = strings.Join(parts, " ")

		// 标记为实际执行的命令
		cmdInfo.IsExecuted = true
	}

	return cmdInfo
}

// checkDangerousCommands 基于语法解析检查危险命令
func checkDangerousCommands(script string) error {
	commands, err := parseCommands(script)
	if err != nil {
		// 如果解析失败，回退到简单的字符串检查（安全第一）
		return checkDangerousCommandsFallback(script)
	}

	// 检查每个实际执行的命令
	for _, cmd := range commands {
		if !cmd.IsExecuted {
			continue // 跳过注释、字符串中的命令
		}

		// 检查是否匹配危险命令规则
		for _, rule := range dangerousCommandRules {
			if isDangerousCommand(cmd, rule) {
				return fmt.Errorf("检测到危险命令: %s (%s)", cmd.FullCmd, rule.Reason)
			}
		}
	}

	return nil
}

// isDangerousCommand 检查命令是否匹配危险规则
func isDangerousCommand(cmd CommandInfo, rule DangerousCommand) bool {
	// 检查命令名
	if cmd.Name != rule.Command {
		return false
	}

	// 如果没有参数要求，只要命令名匹配就认为是危险的
	if len(rule.Args) == 0 {
		return true
	}

	// 检查参数是否匹配
	for i, requiredArg := range rule.Args {
		if i >= len(cmd.Args) {
			return false // 参数数量不足
		}

		// 根据匹配模式检查参数
		if rule.ExactMatch {
			// 精确匹配：参数必须完全相等
			if cmd.Args[i] != requiredArg {
				return false
			}
		} else {
			// 包含匹配：参数包含危险模式即可
			if !strings.Contains(cmd.Args[i], requiredArg) {
				return false
			}
		}
	}

	return true
}

// checkDangerousCommandsFallback 回退的字符串检查（用于解析失败时）
func checkDangerousCommandsFallback(script string) error {
	// 简化的危险命令列表（只检查最危险的）
	criticalCommands := []string{
		"rm -rf /",
		"rm -rf /*",
		"dd if=/dev/zero of=/dev/sd",
		":(){ :|:& };:", // fork炸弹
		"kill -9 -1",
	}

	lowerScript := strings.ToLower(script)
	for _, cmd := range criticalCommands {
		if strings.Contains(lowerScript, strings.ToLower(cmd)) {
			return fmt.Errorf("检测到危险命令（解析失败，使用回退检查）: %s", cmd)
		}
	}

	return nil
}

func validateShell(script string) (err error) {
	// 检查危险命令（基于语法解析）
	if err = checkDangerousCommands(script); err != nil {
		return
	}
	return
}
