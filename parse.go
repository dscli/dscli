package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/toolcall"

	"github.com/spf13/cobra"
)

func init() {
	parseCmd := &cobra.Command{
		Use:   "parse <file>",
		Short: "Parse file structure for LLM editing",
		Long: `Parse file structure (functions, classes, imports) for LLM-assisted editing.
Supports Go files with built-in parser, other languages with Python-based parsing.`,
		Args: cobra.ExactArgs(1),
		RunE: runParse,
	}

	// 添加选项
	parseCmd.Flags().StringP("language", "l", "", "Specify language (auto-detected by default)")
	parseCmd.Flags().BoolP("verbose", "v", false, "Verbose output")
	parseCmd.Flags().BoolP("use-python", "p", false, "Force use Python parser (for non-Go languages)")

	AddRootCommand(parseCmd)
}

// runParse 是 parse 子命令的入口函数
func runParse(cmd *cobra.Command, args []string) error {
	filePath := args[0]

	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("file not found: %s", filePath)
	}

	// 获取语言选项
	lang, _ := cmd.Flags().GetString("language")
	if lang == "" {
		lang = toolcall.GuessLanguage(filePath)
	}

	verbose, _ := cmd.Flags().GetBool("verbose")
	usePython, _ := cmd.Flags().GetBool("use-python")
	ctx := cmd.Context()
	ctx = context.WithValue(ctx, context.VerboseKey, verbose)
	// 解析文件结构
	fs, err := toolcall.ParseFileStructure0(ctx, filePath, lang, usePython)
	if err != nil {
		return fmt.Errorf("failed to parse file: %w", err)
	}

	// 输出JSON
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(fs)
}
