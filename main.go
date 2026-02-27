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

	ModelDeepseekChat, ModelDeepseekReasoner = func() (modelChat string, modelReasoner string) {
		osGetenv := func(k, dv string) string {
			v := os.Getenv(k)
			if v == "" {
				v = dv
			}
			return v
		}
		modelChat = osGetenv("MODEL_DEEPSEEK_CHAT", "deepseek-chat")
		modelReasoner = osGetenv("MODEL_DEEPSEEK_REASONER", "deepseek-reasoner")
		return
	}()

	client      *Client
	logfile     *os.File
	ProjectRoot = func() (projectRoot string) {
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
	}()

	ConfigDir = func() (configDir string) {
		configDir = filepath.Join(os.Getenv("HOME"), ".dscli")
		err := os.MkdirAll(configDir, 0o755)
		if err != nil {
			log.Fatalln(err)
			return
		}
		return
	}()
	EnvPath = filepath.Join(ConfigDir, "dscli.env")
	LogPath = filepath.Join(ConfigDir, "dscli.log")

	rootCmd = &cobra.Command{
		Use:   "dscli",
		Short: "DeepSeek CLI - 与 DeepSeek API 交互",
		Long: `dscli 是一个命令行工具，用于调用 DeepSeek 的 API。
支持 models、balance、chat 和 fim 四个子命令。`,
		PersistentPreRunE:  RootPreRunE,
		PersistentPostRunE: RootPreRunE,
	}
)

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
	client = NewClient(key, url)
	return nil
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
