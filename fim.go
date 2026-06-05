package main

import (
	"fmt"
	"os"

	"github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/dsc"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/spf13/cobra"
)

func init() {
	fimCmd := AddRootCommand(&cobra.Command{
		Use:   "fim [prompt...]",
		Short: "FIM code completion",
		Long: `Send a prompt to the DeepSeek FIM model for code completion.
Content can be provided via positional arguments, stdin, or --input file.

Examples:
  dscli fim implement a quicksort function
  echo "func fib(n int) int {" | dscli fim --suffix "}"
  dscli fim --input prompt.txt
  dscli fim <<EOF
  func handleError(err error) {
  EOF
  dscli fim "implement bubble sort" --stop '###' --stop 'END'`,

		RunE: FimRunE,
	})
	flags := fimCmd.Flags()
	flags.String("model", context.ModelDeepseekChat, "Model name to use")
	flags.String("suffix", "", "Completion suffix (optional)")
	flags.Int("max-tokens", 0, "Max generated tokens (0 uses config default)")
	flags.Float64("temperature", 0.7, "Sampling temperature")
	flags.StringArray("stop", nil, "Stop sequences, repeatable (e.g. --stop '###' --stop 'END')")
	flags.String("input", "", "Read prompt from file (empty reads from stdin)")
}

func FimRunE(cmd *cobra.Command, args []string) (err error) {
	prompt, err := ReadInput(cmd, args)
	if err != nil {
		return err
	}
	if prompt == "" {
		err = fmt.Errorf("error: prompt cannot be empty")
		return err
	}
	fimModel, err := cmd.Flags().GetString("model")
	if err != nil {
		return err
	}

	fimSuffix, err := cmd.Flags().GetString("suffix")
	if err != nil {
		return err
	}

	fimMaxTokens, err := cmd.Flags().GetInt("max-tokens")
	if err != nil {
		return err
	}
	fimTemp, err := cmd.Flags().GetFloat64("temperature")
	if err != nil {
		return err
	}

	fimStop, err := cmd.Flags().GetStringArray("stop")
	if err != nil {
		return err
	}
	var stop any
	switch len(fimStop) {
	case 0:
	case 1:
		stop = fimStop[0]
	default:
		stop = fimStop
	}

	ctx := cmd.Context()
	ctx = context.WithValue(ctx, context.CurrentModelIDKey, fimModel)
	resp, err := DeepseekClient.FIM(ctx, dsc.FIMRequest{
		Model:       fimModel,
		Prompt:      prompt,
		Suffix:      fimSuffix,
		MaxTokens:   fimMaxTokens,
		Temperature: fimTemp,
		Stop:        stop,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "FIM request failed: %v\n", err)
		os.Exit(1)
	}
	if len(resp.Choices) == 0 {
		fmt.Fprintln(os.Stderr, "error: no response received")
		os.Exit(1)
	}

	outfmt.Println(resp.Choices[0].Text)
	return nil
}
