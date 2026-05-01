// Package shell 提供安全的 Shell 脚本执行功能
// 基于 mvdan/sh interp 实现，替代 os/exec
package shell

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gitcode.com/dscli/dscli/internal/context"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// Executor Shell 脚本执行器
type Executor struct {
	parser *syntax.Parser
	config *Config
}

// Config 执行器配置
type Config struct {
	// 工作目录
	WorkingDir string

	// 环境变量（如果为空则使用系统环境变量）
	EnvVars []string

	// 默认执行超时
	Timeout time.Duration

	// 最大输出大小（字节）
	MaxOutputSize int

	// 是否启用严格模式（-e -u 参数）
	StrictMode bool

	// 是否启用沙箱模式
	SandboxMode bool

	// 沙箱配置（仅在 SandboxMode=true 时生效）
	SandboxConfig *SandboxConfig
}

// SandboxConfig 沙箱配置
type SandboxConfig struct {
	// 允许的命令列表（如果为空则允许所有命令）
	AllowedCommands []string

	// 允许访问的路径列表（如果为空则允许所有路径）
	AllowedPaths []string

	// 允许的环境变量列表（如果为空则允许所有环境变量）
	AllowedEnvVars []string

	// 是否允许网络访问
	AllowNetwork bool

	// 是否允许执行外部程序
	AllowExternalExec bool
}

// Result 执行结果
type Result struct {
	// 标准输出
	Stdout string

	// 标准错误
	Stderr string

	// 退出码
	ExitCode int

	// 执行耗时
	Duration time.Duration

	// 执行错误（如果有）
	Err error

	// 输出是否被截断
	Truncated bool
}

// DefaultConfig 返回默认配置
func DefaultConfig(ctx context.Context) *Config {
	return &Config{
		WorkingDir:    ".",
		EnvVars:       os.Environ(),
		Timeout:       30 * time.Second,
		MaxOutputSize: 1024 * 1024, // 1MB
		StrictMode:    true,
		SandboxMode:   false,
		SandboxConfig: DefaultSandboxConfig(ctx),
	}
}

// DefaultSandboxConfig 返回默认沙箱配置
func DefaultSandboxConfig(ctx context.Context) *SandboxConfig {
	return &SandboxConfig{
		AllowedCommands:   getAllowedCommands(),
		AllowedPaths:      []string{"."},
		AllowedEnvVars:    []string{"PATH", "HOME", "USER", "PWD", "LANG", "TERM"},
		AllowNetwork:      false,
		AllowExternalExec: false,
	}
}

// CommandCategory 命令分类，用于组织和描述允许的命令
type CommandCategory struct {
	Name     string   // 分类名称，如"基础命令"、"文件系统工具"
	Commands []string // 该分类下的命令列表
}

// getCommandCategories 返回按功能分组的命令分类
//
// 分类设计原则：
// 1. baseCommands 包含所有 Shell 脚本的基石命令
// 2. 扩展命令按功能领域分组，便于理解和维护
// 3. 每个命令只属于一个分类，避免重复
func getCommandCategories() []CommandCategory {
	return []CommandCategory{
		{
			Name: "基础命令",
			Commands: []string{
				"echo", "cat", "ls", "pwd", "grep", "wc", "find",
				"mkdir", "rmdir", "touch", "rm", "cp", "mv",
				"head", "tail", "sort", "uniq", "cut", "paste",
				"tr", "sed", "awk", "xargs",
				"date",
			},
		},
		{
			Name: "文件系统工具",
			Commands: []string{
				"du", "basename", "dirname", "which", "chmod", "chown",
				"stat", "file", "realpath", "readlink", "ln",
			},
		},
		{
			Name: "文档处理工具",
			Commands: []string{
				"pandoc", "tectonic", "xelatex",
			},
		},
		{
			Name: "版本控制工具",
			Commands: []string{
				"git",
			},
		},
		{
			Name: "网络工具",
			Commands: []string{
				"curl", "wget",
			},
		},
		{
			Name: "压缩工具",
			Commands: []string{
				"tar", "gzip", "bzip2", "unzip", "zip",
			},
		},
		{
			Name: "开发工具",
			Commands: []string{
				"go", "make", "cmake", "python", "python3", "dscli",
			},
		},
		{
			Name: "数据处理工具",
			Commands: []string{
				"bc", "jq", "diff",
			},
		},
		{
			Name: "搜索工具",
			Commands: []string{
				"rg",
			},
		},
		{
			Name: "系统/通用工具",
			Commands: []string{
				"nproc", "df", "tee", "sleep",
			},
		},
	}
}

// getAllowedCommands 返回完整的允许命令列表（从分类中提取）
func getAllowedCommands() []string {
	categories := getCommandCategories()

	// 预估容量，避免多次扩容
	totalCmds := 0
	for _, cat := range categories {
		totalCmds += len(cat.Commands)
	}

	allCommands := make([]string, 0, totalCmds)
	commandSet := make(map[string]bool, totalCmds)

	for _, cat := range categories {
		for _, cmd := range cat.Commands {
			if !commandSet[cmd] {
				commandSet[cmd] = true
				allCommands = append(allCommands, cmd)
			}
		}
	}

	return allCommands
}

// ShellExecConfig 在 Shell 执行上下文中创建配置
func ShellExecConfig(ctx context.Context) *Config {
	projectRoot := context.ProjectRoot
	if projectRoot == "" {
		panic("project root not set")
	}

	isTesting := context.IsTesting()
	config := DefaultConfig(ctx)

	// 使用完整的允许命令列表
	config.SandboxConfig.AllowedCommands = getAllowedCommands()

	config.SandboxConfig.AllowedPaths = append(config.SandboxConfig.AllowedPaths, projectRoot)
	config.WorkingDir = projectRoot
	config.Timeout = 60 * time.Second
	config.SandboxMode = isTesting
	config.EnvVars = append(os.Environ(), "InsideShellExec=1")
	return config
}

// NewExecutor 创建新的执行器
func NewExecutor(ctx context.Context, config *Config) *Executor {
	if config == nil {
		config = DefaultConfig(ctx)
	}

	// 确保工作目录存在
	if config.WorkingDir != "" {
		os.MkdirAll(config.WorkingDir, 0o755)
	}

	return &Executor{
		parser: syntax.NewParser(),
		config: config,
	}
}

// Execute 执行 Shell 脚本
// 注意：超时由上层（工具调用层）通过 context 控制，执行器不重复设置超时
func (e *Executor) Execute(ctx context.Context, script string) (*Result, error) {
	start := time.Now()

	// 从 context 获取脚本名（summary），供语法错误消息使用
	name := context.ContextValue(ctx, context.ShellSummaryKey, "")

	// 解析脚本
	prog, err := e.parser.Parse(strings.NewReader(script), name)
	if err != nil {
		return nil, fmt.Errorf("语法解析失败: %w", err)
	}

	// 创建输出缓冲区
	var stdoutBuf, stderrBuf bytes.Buffer

	// 构建 runner 选项
	opts, err := e.buildRunnerOptions(ctx, &stdoutBuf, &stderrBuf)
	if err != nil {
		return nil, fmt.Errorf("构建执行器选项失败: %w", err)
	}

	// 创建 runner
	runner, err := interp.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("创建解释器失败: %w", err)
	}

	// 执行脚本
	execErr := runner.Run(ctx, prog)
	duration := time.Since(start)

	// 构建结果
	result := &Result{
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		Duration: duration,
		Err:      execErr,
		ExitCode: extractExitCode(execErr),
	}

	return result, nil
}

// buildRunnerOptions 构建 runner 选项
func (e *Executor) buildRunnerOptions(ctx context.Context, stdout, stderr *bytes.Buffer) ([]interp.RunnerOption, error) {
	opts := []interp.RunnerOption{
		interp.StdIO(nil, stdout, stderr),
		interp.Dir(e.config.WorkingDir),
	}

	// 环境变量
	envVars := e.config.EnvVars
	if e.config.SandboxMode && e.config.SandboxConfig != nil {
		envVars = e.filterEnvironment(envVars)
	}
	opts = append(opts, interp.Env(expand.ListEnviron(envVars...)))

	// Shell 参数
	shellParams := []string{}
	if e.config.StrictMode {
		shellParams = append(shellParams, "-e", "-u")
	}
	if len(shellParams) > 0 {
		opts = append(opts, interp.Params(shellParams...))
	}

	// 沙箱处理器
	if e.config.SandboxMode && e.config.SandboxConfig != nil {
		sandboxOpts, err := e.buildSandboxOptions(ctx)
		if err != nil {
			return nil, err
		}
		opts = append(opts, sandboxOpts...)
	}

	return opts, nil
}

// buildSandboxOptions 构建沙箱选项
func (e *Executor) buildSandboxOptions(ctx context.Context) ([]interp.RunnerOption, error) {
	config := e.config.SandboxConfig
	var opts []interp.RunnerOption

	// 命令执行处理器
	if !config.AllowExternalExec && len(config.AllowedCommands) > 0 {
		opts = append(opts, interp.ExecHandlers(e.createCommandFilter))
	}

	// 文件访问处理器
	if len(config.AllowedPaths) > 0 {
		opts = append(opts,
			interp.OpenHandler(e.createPathFilter()),
			interp.ReadDirHandler2(e.createReadDirFilter()),
		)
	}

	return opts, nil
}

// createCommandFilter 创建命令过滤器
func (e *Executor) createCommandFilter(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	allowedCommands := make(map[string]bool)
	for _, cmd := range e.config.SandboxConfig.AllowedCommands {
		allowedCommands[cmd] = true
	}

	return func(ctx context.Context, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("空命令")
		}

		cmd := args[0]
		if !allowedCommands[cmd] {
			return fmt.Errorf("命令不在白名单中: %s", cmd)
		}

		// 使用默认执行处理器（带超时）
		return next(ctx, args)
	}
}

// createPathFilter 创建路径过滤器
func (e *Executor) createPathFilter() interp.OpenHandlerFunc {
	allowedPaths := e.config.SandboxConfig.AllowedPaths

	return func(ctx context.Context, path string, flag int, perm os.FileMode) (io.ReadWriteCloser, error) {
		// 检查路径是否允许访问
		if !isPathAllowed(path, allowedPaths) {
			return nil, fmt.Errorf("禁止访问路径: %s", path)
		}

		// 使用默认处理器
		return interp.DefaultOpenHandler()(ctx, path, flag, perm)
	}
}

// createReadDirFilter 创建目录读取过滤器
func (e *Executor) createReadDirFilter() interp.ReadDirHandlerFunc2 {
	allowedPaths := e.config.SandboxConfig.AllowedPaths

	return func(ctx context.Context, path string) ([]os.DirEntry, error) {
		// 检查路径是否允许访问
		if !isPathAllowed(path, allowedPaths) {
			return nil, fmt.Errorf("禁止读取目录: %s", path)
		}

		// 使用默认处理器
		return interp.DefaultReadDirHandler2()(ctx, path)
	}
}

// filterEnvironment 过滤环境变量
func (e *Executor) filterEnvironment(envVars []string) []string {
	if len(e.config.SandboxConfig.AllowedEnvVars) == 0 {
		return envVars
	}

	allowedSet := make(map[string]bool)
	for _, envVar := range e.config.SandboxConfig.AllowedEnvVars {
		allowedSet[envVar] = true
	}

	var filtered []string
	for _, envVar := range envVars {
		parts := strings.SplitN(envVar, "=", 2)
		if len(parts) == 2 && allowedSet[parts[0]] {
			filtered = append(filtered, envVar)
		}
	}

	return filtered
}

// isPathAllowed 检查路径是否允许访问
func isPathAllowed(path string, allowedPaths []string) bool {
	if len(allowedPaths) == 0 {
		return true
	}

	// 获取绝对路径
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	// 特殊处理 /dev/null，这是一个安全的特殊设备文件
	if absPath == "/dev/null" {
		return true
	}

	// 检查路径是否在允许的路径范围内
	for _, allowed := range allowedPaths {
		absAllowed, err := filepath.Abs(allowed)
		if err != nil {
			continue
		}

		// 检查路径是否以允许的路径开头
		if strings.HasPrefix(absPath, absAllowed) {
			return true
		}

		// 对于相对路径，检查是否在当前目录下
		if allowed == "." {
			cwd, err := os.Getwd()
			if err == nil {
				relPath, err := filepath.Rel(cwd, absPath)
				if err == nil && !strings.HasPrefix(relPath, "..") {
					return true
				}
			}
		}
	}

	return false
}

// extractExitCode 从错误中提取退出码
func extractExitCode(err error) int {
	if err == nil {
		return 0
	}

	if exitErr, ok := err.(interp.ExitStatus); ok {
		return int(exitErr)
	}

	return 1
}

// SimpleExecute 简单执行 Shell 脚本（使用默认配置）
func SimpleExecute(ctx context.Context, script string) (string, error) {
	executor := NewExecutor(ctx, DefaultConfig(ctx))
	result, err := executor.Execute(ctx, script)
	if err != nil {
		return "", err
	}

	if result.Err != nil {
		return result.Stdout + result.Stderr, result.Err
	}

	return result.Stdout, nil
}

// SafeExecute 安全执行 Shell 脚本（启用沙箱模式）
func SafeExecute(ctx context.Context, script string) (string, error) {
	config := DefaultConfig(ctx)
	config.SandboxMode = true
	config.SandboxConfig = DefaultSandboxConfig(ctx)

	executor := NewExecutor(ctx, config)
	result, err := executor.Execute(ctx, script)
	if err != nil {
		return "", err
	}

	if result.Err != nil {
		return result.Stdout + result.Stderr, result.Err
	}

	return result.Stdout, nil
}