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
		return godotenv.Load(filepath.Join(context.ConfigDir, "dscli.env"))
	}()
	// Version information - set via ldflags during build
	Version = "0.7.1"
	Build   = ""
)

func main() {
	if err := RootExecute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
