package main

import (
	"fmt"
	"os/exec"

	"gitcode.com/dscli/dscli/internal/userservice"
	"github.com/spf13/cobra"
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
		Use:   "create <name> <desc> <cmd> [args...]",
		Short: "创建或更新内嵌服务",
		Long: `创建或更新一个 dscli 管理的用户服务。

参数：
  name    服务名称，用作服务标识符和文件名
  desc    人类可读的服务描述
  cmd     要执行的命令（可以是路径或 $PATH 中的名称）
  args    命令参数（可选）

Create 会解析 cmd 的绝对路径并写入平台特定的服务配置，但不会启动服务。
如需启动，请使用 'dscli service start <name>'。

Create 是幂等的：如果服务文件已存在且内容相同，不会重复写入。

示例：
  dscli service create lp "Lightpanda Browser" lightpanda serve --host 127.2.2.9 --port 9227
  dscli service create myapp "My Application" /usr/local/bin/myapp --verbose`,
		Args: cobra.MinimumNArgs(3),
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
	AddCommand(serviceCmd, &cobra.Command{
		Use:   "list",
		Short: "列出所有内嵌服务",
		Long: `列出所有由 dscli 管理的用户服务。

读取 ~/.dscli/services/ 下的 JSON 注册表，仅列出通过
'dscli service create' 创建的服务。

示例：
  dscli service list`,
		Args: cobra.NoArgs,
		RunE: serviceListRunE,
	})

	// status 子命令
	AddCommand(serviceCmd, &cobra.Command{
		Use:   "status [name]",
		Short: "查看服务状态",
		Long: `查看一个或所有内嵌服务的状态。

状态含义：
  running   — 服务正在运行，配置为最新
  stale     — 配置已过期（dscli 或配置文件更新过，需要重新 create）
  stopped   — 配置存在且为最新，但服务未运行
  not_found — 未找到该服务的配置

不指定名称时，列出所有服务的状态摘要。

示例：
  dscli service status lp
  dscli service status`,
		Args: cobra.MaximumNArgs(1),
		RunE: serviceStatusRunE,
	})
}

func serviceCreateRunE(cmd *cobra.Command, args []string) error {
	name := args[0]
	desc := args[1]
	command := args[2]
	cmdArgs := args[3:]

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
	names, err := userservice.List()
	if err != nil {
		return fmt.Errorf("列出服务失败: %w", err)
	}

	if len(names) == 0 {
		fmt.Println("没有内嵌服务。使用 'dscli service create' 创建服务。")
		return nil
	}

	// 收集状态信息，使用中文标签
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
	// 单个服务状态
	if len(args) == 1 {
		name := args[0]
		status, err := userservice.Status(name)
		if err != nil {
			return fmt.Errorf("查询服务状态失败: %w", err)
		}

		// 用友好的中文描述
		desc := statusLabel(status)
		fmt.Printf("%s: %s\n", name, desc)
		return nil
	}

	// 列出所有服务状态（复用 list 逻辑）
	names, err := userservice.List()
	if err != nil {
		return fmt.Errorf("列出服务失败: %w", err)
	}

	if len(names) == 0 {
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
		return "已过期（需要重新 create）"
	case "stopped":
		return "已停止"
	case "not_found":
		return "未找到"
	default:
		return s
	}
}
