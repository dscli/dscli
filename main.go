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
	client      *Client
	logfile     *os.File
	ProjectRoot string
	ProjectHash string
	SqliteDB    *DB
	SessionID   int64

	rootCmd     = &cobra.Command{
		Use:   "dscli",
		Short: "DeepSeek CLI - 与 DeepSeek API 交互",
		Long: `dscli 是一个命令行工具，用于调用 DeepSeek 的 API。
支持 models、balance、chat 和 fim 四个子命令。`,
		PersistentPreRunE:  RootPreRunE,
		PersistentPostRunE: RootPreRunE,
	}
)

// GetProjectHash 获取项目路径的哈希值
func getProjectHash(projectPath string) string {
	// 简单实现：使用路径作为哈希（实际可以使用MD5等）
	// 这里为了简单，直接使用路径，实际应该使用哈希函数
	return projectPath
}

// getProjectRoot 获取当前项目根目录（用于会话隔离）
func getProjectRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	gitRoot, err := findGitRoot(cwd)
	if err == nil && gitRoot != "" {
		return gitRoot, nil
	}
	return filepath.Abs(cwd)
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
	if SqliteDB != nil {
		err = SqliteDB.Close()
		if err != nil {
			return
		}
	}

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
	ProjectRoot, err = getProjectRoot()
	if err != nil {
		return
	}
	cwd, err := os.Getwd()
	if err != nil {
		return
	}

	if cwd != ProjectRoot {
		err = os.Chdir(ProjectRoot)
		if err != nil {
			return
		}
	}

	cwd, err = os.Getwd()
	if err != nil {
		return
	}

	if cwd != ProjectRoot {
		err = fmt.Errorf("cwd(%s) != ProjectRoot(%s)", cwd, ProjectRoot)
		return
	}

     ProjectHash = getProjectHash(ProjectRoot)
	// 2. 打开数据库
	SqliteDB, err = New()
	if err != nil {
		err = fmt.Errorf("初始化数据库失败: %w", err)
		return
	}

	// 3. 获取会话ID
	SessionID, err = SqliteDB.GetOrCreateSession(ProjectRoot)
	if err != nil {
		err = fmt.Errorf("获取会话失败: %w", err)
		return
	}

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
	client = NewClient(key, url, false)
	return nil
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
