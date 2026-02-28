package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

var (
	_ = func() error {
		return godotenv.Load(EnvPath)
	}()

	// Version information - set via ldflags during build
	Version = "0.5.0"
	Build   = ""

	mode                  string
	logLevel              string
	colorEnabled          bool
	showTimestamp         bool
	ModelDeepseekChat     = Getenv("MODEL_DEEPSEEK_CHAT", "deepseek-chat")
	ModelDeepseekReasoner = Getenv("MODEL_DEEPSEEK_REASONER", "deepseek-reasoner")

	DeepseekClient Client
	closeAll       func() error
	ProjectRoot    = GetProjectRoot()

	ConfigDir = GetConfigDir()
	EnvPath   = filepath.Join(ConfigDir, "dscli.env")
	LogPath   = filepath.Join(ConfigDir, "dscli.log")

	RootCmd = &cobra.Command{
		Use:   "dscli",
		Short: "DeepSeek CLI - 与 DeepSeek API 交互",
		Long: `dscli 是一个命令行工具，用于调用 DeepSeek 的 API。
支持 models、balance、chat 和 fim 四个子命令。

输出选项：
  --mode          输出模式：markdown（Markdown格式）、org（Org模式格式）
  --log-level     日志级别：debug、info、warn、error、fatal
  --no-color      禁用颜色输出
  --no-timestamp  禁用时间戳显示`,
		PersistentPreRunE:  RootPreRunE,
		PersistentPostRunE: RootPostRunE,
		Version:            Version,
	}
)

func init() {
	RootCmd.PersistentFlags().StringVar(&mode, "mode", "markdown", "输出模式：markdown（Markdown格式）、org（Org模式格式）")
	RootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "日志级别：debug、info、warn、error、fatal")
	RootCmd.PersistentFlags().BoolVar(&colorEnabled, "no-color", false, "禁用颜色输出")
	RootCmd.PersistentFlags().BoolVar(&showTimestamp, "no-timestamp", false, "禁用时间戳显示")
}

func GetConfigDir() (configDir string) {
	configDir = filepath.Join(os.Getenv("HOME"), ".dscli")
	err := os.MkdirAll(configDir, 0o755)
	if err != nil {
		log.Fatalln(err)
		return
	}
	return
}

func GetProjectRoot() (projectRoot string) {
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalln(err)
		return
	}
	gitRoot, err := findGitRoot(cwd)
	if err != nil {
		gitRoot = cwd
	}
	projectRoot, err = filepath.Abs(gitRoot)
	if err != nil {
		log.Fatalln(err)
		return
	}

	if cwd != projectRoot {
		err = os.Chdir(projectRoot)
		if err != nil {
			log.Fatalln(err)
			return
		}
	}

	cwd, err = os.Getwd()
	if err != nil {
		log.Fatalln(err)
		return
	}

	if cwd != projectRoot {
		err = fmt.Errorf("cwd(%s) != ProjectRoot(%s)", cwd, projectRoot)
		log.Fatalln(err)
		return
	}
	return projectRoot
}

func Getenv(key, dvalue string) (value string) {
	value = os.Getenv(key)
	if value == "" {
		value = dvalue
	}
	return
}

func findGitRoot(dir string) (string, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		gitPath := filepath.Join(absDir, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			return absDir, nil
		}
		parent := filepath.Dir(absDir)
		if parent == absDir {
			break
		}
		absDir = parent
	}
	return "", fmt.Errorf("未找到 Git 仓库根目录")
}

func RootPostRunE(cmd *cobra.Command, args []string) (err error) {
	if closeAll != nil {
		err = closeAll()
	}
	return
}

func RootPreRunE(cmd *cobra.Command, args []string) (err error) {
	// 配置输出系统
	configureOutput()

	var output *os.File
	switch mode {
	case "markdown":
	case "org":
		var r *os.File
		r, output, err = os.Pipe()
		if err != nil {
			return err
		}
		SetOutputWriter(output)
		go func(input io.Reader) error {
			converter := NewMarkdownToOrgConverter()
			return converter.ConvertStream(input, os.Stdout)
		}(r)
	default:
		err = fmt.Errorf("do not support %s", mode)
		return
	}

	key := Getenv("DEEPSEEK_API_KEY", "")
	if key == "" {
		err = fmt.Errorf("no api key specified")
		return
	}

	url := os.Getenv("DEEPSEEK_BASE_URL")
	if url == "" {
		url = "https://api.deepseek.com" // 默认值
	}
	var logfile *os.File
	logfile, err = os.OpenFile(LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	log.SetOutput(logfile)

	closeAll = func() (err error) {
		var errs []error

		// 关闭 w（如果存在）
		if output != nil {
			// flush output - 写入一个换行符确保缓冲区被刷新
			if _, err := output.Write([]byte("\n")); err != nil {
				errs = append(errs, err)
			}
			if err := output.Close(); err != nil {
				errs = append(errs, err)
			}
		}

		// 关闭 logfile
		if err := logfile.Close(); err != nil {
			errs = append(errs, err)
		}

		// 如果有错误，返回第一个错误
		if len(errs) > 0 {
			return errs[0]
		}
		return nil
	}
	log.SetFlags(log.Lshortfile | log.LstdFlags)
	DeepseekClient = NewClient(key, url)
	return nil
}

// configureOutput 配置输出系统
func configureOutput() {
	// 设置日志级别
	switch strings.ToLower(logLevel) {
	case "debug":
		SetLogLevel(LogLevelDebug)
	case "info":
		SetLogLevel(LogLevelInfo)
	case "warn":
		SetLogLevel(LogLevelWarn)
	case "error":
		SetLogLevel(LogLevelError)
	case "fatal":
		SetLogLevel(LogLevelFatal)
	default:
		SetLogLevel(LogLevelInfo)
	}

	// 设置颜色输出
	SetColorEnabled(!colorEnabled) // 注意：--no-color 为 true 时禁用颜色

	// 设置时间戳显示
	SetShowTimestamp(!showTimestamp) // 注意：--no-timestamp 为 true 时禁用时间戳
}

func main() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
