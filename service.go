package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/dscli/dscli/internal/userservice"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var serviceCmd *cobra.Command

func init() {
	serviceCmd = AddRootCommand(&cobra.Command{
		Use:   "service",
		Short: "内嵌服务管理 - 创建、启动、停止、删除、列出和查看服务状态",
		Long: `service 命令用于管理 dscli 内嵌的 OS 级用户服务。

这些服务作为当前用户的守护进程独立运行，不受 dscli 进程生命周期影响。

后端支持：
  Linux:   systemd --user（不可用时自动回退到 pidfile）
  macOS:   launchctl（LaunchAgent）
  其他:    直接进程 + pidfile（回退方案）

所有服务配置保存在 ~/.dscli/services/<name>.json。`,
	})

	// create 子命令
	AddCommand(serviceCmd, &cobra.Command{
		Use:   "create <name>",
		Short: "创建或更新内嵌服务",
		Long: `创建或更新一个 dscli 管理的用户服务。

从标准输入读取服务描述和命令：
  第一行：服务描述
  后续行：命令及参数（空格分隔）

Create 会解析 cmd 的绝对路径并写入平台特定的服务配置，但不会启动服务。
如需启动，请使用 'dscli service start <name>'。

Create 是幂等的：如果服务文件已存在且内容相同，不会重复写入。

示例：
  dscli service create dscli-lightpanda <<EOF
  Lightpanda Browser (dscli)
  lightpanda serve --host 127.2.2.9 --port 9227
  EOF

  dscli service create myapp <<EOF
  My Application
  /usr/local/bin/myapp --verbose --config /etc/myapp.conf
  EOF`,
		Args: cobra.ExactArgs(1),
		RunE: serviceCreateRunE,
	})

	// delete 子命令
	AddCommand(serviceCmd, &cobra.Command{
		Use:   "delete <name>",
		Short: "删除内嵌服务（停止并清理）",
		Long: `删除一个 dscli 管理的用户服务。

此操作会：
  - 停止正在运行的服务
  - 删除平台特定的服务配置文件
  - 删除 ~/.dscli/services/<name>.json 注册表条目

示例：
  dscli service delete lp`,
		Args: cobra.ExactArgs(1),
		RunE: serviceDeleteRunE,
	})

	// start 子命令
	AddCommand(serviceCmd, &cobra.Command{
		Use:   "start <name>",
		Short: "启动内嵌服务",
		Long: `启动一个 dscli 管理的用户服务。

该服务必须已经通过 'dscli service create' 创建。
启动后服务作为守护进程独立运行。

示例：
  dscli service start lp`,
		Args: cobra.ExactArgs(1),
		RunE: serviceStartRunE,
	})

	// stop 子命令
	AddCommand(serviceCmd, &cobra.Command{
		Use:   "stop <name>",
		Short: "停止内嵌服务",
		Long: `停止一个 dscli 管理的用户服务。

该服务不会从系统中删除，仍可通过 'dscli service start' 重新启动。

示例：
  dscli service stop lp`,
		Args: cobra.ExactArgs(1),
		RunE: serviceStopRunE,
	})

	// list 子命令
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "列出所有内嵌服务",
		Long: `列出所有由 dscli 管理的用户服务。

读取 ~/.dscli/services/ 下的 JSON 注册表，仅列出通过
'dscli service create' 创建的服务。

使用 --scan 扫描系统（systemd/launchd）中 dscli 管理的
但缺少 JSON 注册表的孤儿服务。

示例：
  dscli service list
  dscli service list --scan`,
		Args: cobra.NoArgs,
		RunE: serviceListRunE,
	}
	listCmd.Flags().Bool("scan", false, "扫描系统中孤儿服务（systemd/launchd 中存在但无 JSON 注册表）")
	AddCommand(serviceCmd, listCmd)

	// status 子命令
	statusCmd := &cobra.Command{
		Use:   "status [name]",
		Short: "查看服务状态",
		Long: `查看一个或所有内嵌服务的状态。

状态含义：
  running   — 服务正在运行
  stopped   — 配置存在，但服务未运行
  not_found — 未找到该服务的配置

注意：系统会自动刷新过期配置（如 dscli 更新后），无需手动干预。
仅在自动刷新失败且服务未运行时，stale 状态才会出现。

不指定名称时，列出所有服务的状态摘要。

使用 --scan 检查系统级服务（绕过 JSON 注册表），可查看
缺少注册表的孤儿服务状态。

示例：
  dscli service status lp
  dscli service status
  dscli service status dscli-lightpanda --scan`,
		Args: cobra.MaximumNArgs(1),
		RunE: serviceStatusRunE,
	}
	statusCmd.Flags().Bool("scan", false, "绕过 JSON 注册表，直接检查系统级服务状态")
	AddCommand(serviceCmd, statusCmd)
}

func serviceCreateRunE(cmd *cobra.Command, args []string) error {
	name := args[0]

	// 检测标准输入是否为终端，避免无 heredoc/管道时卡住
	if term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("请通过 heredoc 或管道提供描述和命令，例如：\n\n"+
			"  dscli service create %s <<EOF\n"+
			"  服务描述\n"+
			"  /path/to/binary --flag1 --flag2\n"+
			"  EOF", name)
	}

	// 从标准输入读取：第一行 = 描述，后续行 = 命令及参数
	stdin, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("读取标准输入失败: %w", err)
	}

	content := strings.TrimSpace(string(stdin))
	if content == "" {
		return fmt.Errorf("标准输入为空，请通过 heredoc 或管道提供描述和命令")
	}

	lines := strings.Split(content, "\n")
	if len(lines) < 2 {
		return fmt.Errorf("需要至少2行输入：第一行为服务描述，后续行为命令及参数")
	}

	desc := strings.TrimSpace(lines[0])
	if desc == "" {
		return fmt.Errorf("第一行（服务描述）不能为空")
	}

	// 将后续行合并为命令字符串，按空格拆分为命令 + 参数
	cmdLine := strings.TrimSpace(strings.Join(lines[1:], " "))
	if cmdLine == "" {
		return fmt.Errorf("命令不能为空")
	}

	parts := strings.Fields(cmdLine)
	command := parts[0]
	cmdArgs := parts[1:]

	execCmd := exec.Command(command, cmdArgs...)
	if err := userservice.Create(name, desc, execCmd); err != nil {
		return fmt.Errorf("创建服务失败: %w", err)
	}

	fmt.Printf("✅ 服务 %q 已创建（未启动）\n", name)
	fmt.Printf("   使用 'dscli service start %s' 启动服务\n", name)
	return nil
}

func serviceDeleteRunE(cmd *cobra.Command, args []string) error {
	name := args[0]

	if err := userservice.Delete(name); err != nil {
		return fmt.Errorf("删除服务失败: %w", err)
	}

	fmt.Printf("✅ 服务 %q 已删除\n", name)
	return nil
}

func serviceStartRunE(cmd *cobra.Command, args []string) error {
	name := args[0]

	if err := userservice.Start(name); err != nil {
		return fmt.Errorf("启动服务失败: %w", err)
	}

	fmt.Printf("✅ 服务 %q 已启动\n", name)
	return nil
}

func serviceStopRunE(cmd *cobra.Command, args []string) error {
	name := args[0]

	if err := userservice.Stop(name); err != nil {
		return fmt.Errorf("停止服务失败: %w", err)
	}

	fmt.Printf("✅ 服务 %q 已停止\n", name)
	return nil
}

func serviceListRunE(cmd *cobra.Command, args []string) error {
	scan, _ := cmd.Flags().GetBool("scan")

	names, err := userservice.List()
	if err != nil {
		return fmt.Errorf("列出服务失败: %w", err)
	}

	// --scan: also discover orphaned services from OS-level service manager.
	var orphaned []string
	if scan {
		orphaned, err = userservice.Scan()
		if err != nil {
			return fmt.Errorf("扫描孤儿服务失败: %w", err)
		}
	}

	if len(names) == 0 && len(orphaned) == 0 {
		fmt.Println("没有内嵌服务。使用 'dscli service create' 创建服务。")
		return nil
	}

	// Collect status for registered services.
	var maps []map[string]string
	for _, name := range names {
		status, err := userservice.Status(name)
		if err != nil {
			status = fmt.Sprintf("error: %v", err)
		}
		maps = append(maps, map[string]string{
			"name":   name,
			"status": statusLabel(status),
		})
	}

	// Add orphaned services with a distinct label.
	for _, name := range orphaned {
		status, err := userservice.ScanStatus(name)
		if err != nil {
			status = fmt.Sprintf("error: %v", err)
		}
		label := statusLabel(status)
		// Add note that this is orphaned (no JSON registry).
		label = label + "（孤儿：缺少注册表，请重新 create）"
		maps = append(maps, map[string]string{
			"name":   name,
			"status": label,
		})
	}

	headers := []string{"名称", "状态"}
	rowFunc := func(data any) []string {
		switch m := data.(type) {
		case map[string]string:
			return []string{m["name"], m["status"]}
		default:
			return []string{"", ""}
		}
	}

	return FormatOutput(maps, "table", headers, rowFunc)
}

func serviceStatusRunE(cmd *cobra.Command, args []string) error {
	scan, _ := cmd.Flags().GetBool("scan")

	// 单个服务状态
	if len(args) == 1 {
		name := args[0]
		var status string
		var err error
		if scan {
			status, err = userservice.ScanStatus(name)
		} else {
			status, err = userservice.Status(name)
		}
		if err != nil {
			return fmt.Errorf("查询服务状态失败: %w", err)
		}

		// 用友好的中文描述
		desc := statusLabel(status)
		fmt.Printf("%s: %s\n", name, desc)
		return nil
	}

	// 列出所有服务状态
	names, err := userservice.List()
	if err != nil {
		return fmt.Errorf("列出服务失败: %w", err)
	}

	var orphaned []string
	if scan {
		orphaned, err = userservice.Scan()
		if err != nil {
			return fmt.Errorf("扫描孤儿服务失败: %w", err)
		}
	}

	if len(names) == 0 && len(orphaned) == 0 {
		fmt.Println("没有内嵌服务。使用 'dscli service create' 创建服务。")
		return nil
	}

	var maps []map[string]string
	for _, name := range names {
		status, err := userservice.Status(name)
		if err != nil {
			status = fmt.Sprintf("error: %v", err)
		}
		maps = append(maps, map[string]string{
			"name":   name,
			"status": statusLabel(status),
		})
	}
	for _, name := range orphaned {
		status, err := userservice.ScanStatus(name)
		if err != nil {
			status = fmt.Sprintf("error: %v", err)
		}
		label := statusLabel(status)
		label = label + "（孤儿：缺少注册表，请重新 create）"
		maps = append(maps, map[string]string{
			"name":   name,
			"status": label,
		})
	}

	headers := []string{"名称", "状态"}
	rowFunc := func(data any) []string {
		switch m := data.(type) {
		case map[string]string:
			return []string{m["name"], m["status"]}
		default:
			return []string{"", ""}
		}
	}

	return FormatOutput(maps, "table", headers, rowFunc)
}

// statusLabel 将 userservice 状态码转换为中文标签。
func statusLabel(s string) string {
	switch s {
	case "running":
		return "运行中"
	case "stale":
		return "已过期（自动修复失败）"
	case "stopped":
		return "已停止"
	case "not_found":
		return "未找到"
	default:
		return s
	}
}
