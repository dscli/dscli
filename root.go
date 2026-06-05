package main

import (
	"fmt"

	"github.com/dscli/dscli/internal/config"
	"github.com/dscli/dscli/internal/dsc"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/sqlite"
	"github.com/spf13/cobra"
)

var (
	DeepseekClient dsc.Client

	rootCmd = &cobra.Command{
		Use:   "dscli",
		Short: "DeepSeek CLI - AI-powered development toolkit",
		Long: `dscli is a CLI tool for interacting with the DeepSeek API.
Supports models, balance, chat, and fim subcommands.

Output options:
  --mode          Output mode: markdown (default), org (Org mode)
  --verbose       Enable debug mode with detailed output
  --no-color      Disable colored output
  --no-timestamp  Disable timestamp display`,
		PersistentPreRunE: RootPersistentPreRunE,
		Version:           Version,
	}
)

func init() {
	rootCmd.PersistentFlags().String("mode", "markdown", "Output mode: markdown (default), org (Org mode)")
	rootCmd.PersistentFlags().Bool("no-color", false, "Disable colored output")
	rootCmd.PersistentFlags().Bool("no-timestamp", false, "Disable timestamp display")
	rootCmd.PersistentFlags().Bool("verbose", false, "Enable debug mode (detailed output)")
}

func AddCommand(parent, child *cobra.Command) *cobra.Command {
	parent.AddCommand(child)
	return child
}

func AddRootCommand(child *cobra.Command) *cobra.Command {
	return AddCommand(rootCmd, child)
}

func RootPersistentPreRunE(cmd *cobra.Command, args []string) (err error) {
	verbose, err := cmd.Flags().GetBool("verbose")
	if err != nil {
		return err
	}

	mode, err := cmd.Flags().GetString("mode")
	if err != nil {
		return err
	}

	colorEnabled, err := cmd.Flags().GetBool("no-color")
	if err != nil {
		return err
	}

	showTimestamp, err := cmd.Flags().GetBool("no-timestamp")
	if err != nil {
		return err
	}

	// Configure color output
	outfmt.SetColorEnabled(!colorEnabled) // Note: --no-color=true disables colors

	// Configure timestamp display
	outfmt.SetShowTimestamp(!showTimestamp) // Note: --no-timestamp=true disables timestamps

	// Configure verbose output
	outfmt.SetVerbose(verbose)

	// Configure output system
	outfmt.SetOutputWriter(cmd.OutOrStdout())
	switch mode {
	case "markdown":
	case "org":
		outfmt.SetOutputMode(mode)
	default:
		err = fmt.Errorf("unsupported output mode: %s", mode)
		return err
	}
	// Initialize database (ensures all init() functions have executed)
	db, err := sqlite.OpenDB()
	if err != nil {
		return fmt.Errorf("database init failed: %w", err)
	}
	db.Close() // Release connection after init; subsequent commands acquire as needed

	key := config.Get("deepseek-api-key", "")
	if key == "" && cmd.Name() != "flycheck" && cmd.Name() != "version" {
		err = fmt.Errorf("no api key specified")
		return err
	}

	if key != "" {
		url := config.Get("deepseek-base-url", "https://api.deepseek.com")
		DeepseekClient = dsc.NewClient(key, url)
	}
	return nil
}

func RootExecute() error { return rootCmd.Execute() }
