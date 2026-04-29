package main

import (
	"fmt"
	"os"
)

var (
	// Version information - set via ldflags during build
	Version = "0.7.4"
	Build   = ""
)

func main() {
	if err := RootExecute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
