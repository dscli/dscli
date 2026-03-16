package main

import (
	"context"
	"fmt"
	"os"
)

// CodeMakeFormat - run code format command provided in the context
func CodeMakeFormat(ctx context.Context) (output string, err error) {
	mkfmt := ContextValue(ctx, MakeFormatKey, "make fmt")
	ctx = context.WithValue(ctx, ShellStdinKey, os.Stdin)
	output, err = ShellExec(ctx, mkfmt)
	if err != nil {
		err = fmt.Errorf("failed to make code format: %w", err)
	}
	return
}
