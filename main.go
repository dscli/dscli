package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gitcode.com/dscli/dscli/internal/context"
	"github.com/joho/godotenv"
)

var (
	_ = func() error {
		return godotenv.Load(filepath.Join(ConfigDir, "dscli.env"))
	}()
	// Version information - set via ldflags during build
	Version = "0.7.1"
	Build   = ""
	// ModelDeepseekChat     = context.Getenv("MODEL_DEEPSEEK_CHAT", "deepseek-chat")
	// ModelDeepseekReasoner = context.Getenv("MODEL_DEEPSEEK_REASONER", "deepseek-reasoner")

	DeepseekClient Client
	ConfigDir      = context.GetConfigDir()
)

func main() {
	if err := RootExecute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
