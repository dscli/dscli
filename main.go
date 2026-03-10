package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

var (
	_ = func() error {
		return godotenv.Load(EnvPath)
	}()
	// Version information - set via ldflags during build
	Version = "0.5.5"
	Build   = ""

	ModelDeepseekChat     = Getenv("MODEL_DEEPSEEK_CHAT", "deepseek-chat")
	ModelDeepseekReasoner = Getenv("MODEL_DEEPSEEK_REASONER", "deepseek-reasoner")

	DeepseekClient Client
	ProjectRoot    = GetProjectRoot()

	ConfigDir = GetConfigDir()
	EnvPath   = filepath.Join(ConfigDir, "dscli.env")
	LogPath   = filepath.Join(ConfigDir, "dscli.log")
)

func GetConfigDir() (configDir string) {
	configDir = filepath.Join(os.Getenv("HOME"), ".dscli")
	err := os.MkdirAll(configDir, 0o755)
	if err != nil {
		panic(err)
	}
	return
}

func GetProjectRoot() (projectRoot string) {
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	gitRoot, err := findGitRoot(cwd)
	if err != nil {
		gitRoot = cwd
	}
	projectRoot, err = filepath.Abs(gitRoot)
	if err != nil {
		panic(err)
	}

	if cwd != projectRoot {
		err = os.Chdir(projectRoot)
		if err != nil {
			panic(err)
		}
	}

	cwd, err = os.Getwd()
	if err != nil {
		panic(err)
	}

	if cwd != projectRoot {
		err = fmt.Errorf("cwd(%s) != ProjectRoot(%s)", cwd, projectRoot)
		panic(err)
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

func main() {
	if err := RootExecute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
