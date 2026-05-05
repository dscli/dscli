package shell

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

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

// ============================================================
// 命令验证相关
// ============================================================

// CommandInfo 系统命令验证信息
type CommandInfo struct {
	Name     string // 命令名
	Path     string // 命令路径（空表示未找到）
	Version  string // 版本信息首行（通过 --version/version 等获取）
	Exists   bool   // 系统中是否存在（exec.LookPath 成功）
	Verified bool   // 是否经过版本验证（确认为真实命令，非同名包装脚本）
	Error    string // 验证过程中的错误信息
}

// VerifySystemCommand 验证命令在系统中真实存在且为期望的命令
//
// 使用 exec.LookPath 查找命令路径，然后通过尝试多种版本查询方式
// （--version、-version、version 子命令等）获取版本信息来验证命令身份。
//
// 某些命令（如 rg）可能被用户同名脚本覆盖，此方法通过版本输出检测此类情况。
// 版本输出非空即认为命令身份已经过验证——真实的命令行工具都会响应版本查询，
// 而简单的包装脚本通常不会正确处理 --version 参数。
//
// 验证流程：
//  1. exec.LookPath 查找命令路径（2s 超时）
//  2. 依次尝试 --version、-version、version、-V、--help 获取版本信息
//  3. 版本信息非空 → Verified=true
//
// 参数:
//
//	ctx: 上下文（用于超时控制）
//	cmd: 命令名
//
// 返回值:
//
//	*CommandInfo: 命令验证信息（始终非 nil）
//	error: 系统级错误（如超时），命令不存在不会返回 error
func VerifySystemCommand(ctx context.Context, cmd string) (*CommandInfo, error) {
	info := &CommandInfo{Name: cmd}

	// 1. 查找命令路径
	path, err := exec.LookPath(cmd)
	if err != nil {
		info.Error = fmt.Sprintf("命令未找到: %v", err)
		return info, nil
	}
	info.Path = path
	info.Exists = true

	// 2. 尝试获取版本信息（2秒超时，防止挂起）
	versionCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	version := tryGetVersion(versionCtx, path)
	if version != "" {
		info.Version = version
		info.Verified = true
	}

	return info, nil
}

// tryGetVersion 尝试获取命令的版本信息
//
// 按优先级依次尝试常见版本查询参数，返回首行输出。
// 所有尝试都失败则返回空字符串。
func tryGetVersion(ctx context.Context, path string) string {
	// 按优先级排列：--version 是 GNU 标准，-version 次之，version 子命令再次
	versionFlags := [][]string{
		{"--version"},
		{"-version"},
		{"version"},
		{"-V"},
		{"--help"}, // 部分命令（如 pandoc）在 help 首行显示版本
	}

	for _, flags := range versionFlags {
		cmd := exec.CommandContext(ctx, path, flags...)
		output, err := cmd.Output()
		if err == nil && len(output) > 0 {
			lines := strings.SplitN(string(output), "\n", 2)
			firstLine := strings.TrimSpace(lines[0])
			if firstLine != "" {
				return firstLine
			}
		}
	}

	return ""
}

// IsCommandAvailable 判断命令是否可用（在允许列表中且系统中真实存在）
//
// 安全策略：系统中存在但不在允许列表中的命令，统一判为不可用。
// 此设计确保：
//  1. 沙箱执行时不会意外放行未授权的命令
//  2. 不泄露系统已安装命令的信息（不区分"不存在"与"存在但不允许"）
//
// 参数:
//
//	ctx: 上下文
//	cmd: 命令名
//	allowedCommands: 允许的命令列表（如 getAllowedCommands() 的返回值）
//
// 返回值:
//
//	bool: 命令可用返回 true
func IsCommandAvailable(ctx context.Context, cmd string, allowedCommands []string) bool {
	// 检查是否在允许列表中（快速路径：O(n)，n 为允许命令数 ≈ 55）
	for _, allowed := range allowedCommands {
		if allowed == cmd {
			goto checkSystem
		}
	}
	// 不在允许列表中，无论系统是否存在，统一返回 false
	return false

checkSystem:
	// 在允许列表中，验证系统存在且为真实命令
	info, err := VerifySystemCommand(ctx, cmd)
	if err != nil || !info.Exists {
		return false
	}
	return true
}

// GetAvailableCommandsDescription 返回系统可用命令的 Markdown 描述
//
// 仅包含允许列表中存在且系统中真实存在的命令，按分类组织。
// 返回格式适合嵌入 Shell 工具的系统提示词（tool call description）：
//
//	### 命令分类1
//	命令名1 - 版本信息1
//	命令名2 - 版本信息2
//	...
//
//	### 命令分类2
//	...
//
// 参数:
//
//	ctx: 上下文（用于命令验证的超时控制，整体可能耗时数秒）
//
// 返回值:
//
//	string: Markdown 格式的命令描述（无可用命令时返回提示信息）
func GetAvailableCommandsDescription(ctx context.Context) string {
	categories := getCommandCategories()
	var b strings.Builder

	for _, cat := range categories {
		var available []string
		for _, cmd := range cat.Commands {
			if _, err := exec.LookPath(cmd); err != nil {
				continue
			}
			available = append(available, cmd)
		}
		if len(available) == 0 {
			continue
		}
		fmt.Fprintf(&b, "### %s\n", cat.Name)
		b.WriteString(strings.Join(available, ", "))
		b.WriteString("\n\n")
	}

	result := b.String()
	if result == "" {
		return "（系统中未找到任何允许的命令）\n"
	}
	return result
}
