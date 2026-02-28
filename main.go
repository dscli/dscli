package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

var (
	_ = func() error {
		return godotenv.Load(EnvPath)
	}()

	Mode                  string
	ModelDeepseekChat     = Getenv("MODEL_DEEPSEEK_CHAT", "deepseek-chat")
	ModelDeepseekReasoner = Getenv("MODEL_DEEPSEEK_REASONER", "deepseek-reasoner")

	DeepseekClient Client
	logfile        *os.File
	ProjectRoot    = GetProjectRoot()

	ConfigDir = GetConfigDir()
	EnvPath   = filepath.Join(ConfigDir, "dscli.env")
	LogPath   = filepath.Join(ConfigDir, "dscli.log")

	RootCmd = &cobra.Command{
		Use:   "dscli",
		Short: "DeepSeek CLI - 与 DeepSeek API 交互",
		Long: `dscli 是一个命令行工具，用于调用 DeepSeek 的 API。
支持 models、balance、chat 和 fim 四个子命令。`,
		PersistentPreRunE:  RootPreRunE,
		PersistentPostRunE: RootPostRunE,
	}
)

func init() {
	RootCmd.PersistentFlags().StringVar(&Mode, "mode", "markdown", "输出模式：markdown（Markdown格式）、org（Org模式格式）")
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
	if logfile != nil {
		err = logfile.Close()
		if err != nil {
			return
		}
	}
	return
}

func RootPreRunE(cmd *cobra.Command, args []string) (err error) {
	// 设置输出模式
	SetOutputMode(Mode)

	// change cwd if needed
	key := os.Getenv("DEEPSEEK_API_KEY")

	if key == "" {
		err = fmt.Errorf("no api key specified")
		return
	}

	url := os.Getenv("DEEPSEEK_BASE_URL")
	if url == "" {
		url = "https://api.deepseek.com" // 默认值
	}

	logfile, err = os.OpenFile(LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}

	log.SetOutput(logfile)

	log.SetFlags(log.Lshortfile | log.LstdFlags)
	DeepseekClient = NewClient(key, url)
	return nil
}

func main() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
