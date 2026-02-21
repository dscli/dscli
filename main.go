package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"gitcode.com/nanjunjie/dscli/internal/api"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

var (
	client  *api.Client
	logfile *os.File
	rootCmd = &cobra.Command{
		Use:   "dscli",
		Short: "DeepSeek CLI - 与 DeepSeek API 交互",
		Long: `dscli 是一个命令行工具，用于调用 DeepSeek 的 API。
支持 models、balance、chat 和 fim 四个子命令。`,
		PersistentPreRunE:  RootPreRunE,
		PersistentPostRunE: RootPreRunE,
	}
)

func RootPostRunE(cmd *cobra.Command, args []string) (err error) {
	if logfile != nil {
		err = logfile.Close()
	}
	return
}

func RootPreRunE(cmd *cobra.Command, args []string) (err error) {
	// 初始化 API 客户端
	confdir := filepath.Join(os.Getenv("HOME"), ".dscli")
	err = os.MkdirAll(confdir, 0o644)
	if err != nil {
		return
	}
	envpath := filepath.Join(confdir, "dscli.env")
	logpath := filepath.Join(confdir, "dscli.log")
	key := os.Getenv("DEEPSEEK_API_KEY")
	if key == "" {
		if err = godotenv.Load(envpath); err != nil {
			return
		}
	}

	if key = os.Getenv("DEEPSEEK_API_KEY"); key == "" {
		err = fmt.Errorf("no api key specified")
		return
	}

	url := os.Getenv("DEEPSEEK_BASE_URL")
	if url == "" {
		url = "https://api.deepseek.com" // 默认值
	}
	logfile, err = os.OpenFile(logpath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	log.SetOutput(logfile)
	log.SetFlags(log.Lshortfile | log.LstdFlags)
	client = api.NewClient(key, url, false)
	return nil
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
