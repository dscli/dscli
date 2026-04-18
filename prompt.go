package main

import (
	"fmt"
	"os"

	"gitcode.com/dscli/dscli/internal/editor"
	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/prompt"
	"github.com/spf13/cobra"
)

func init() {
	promptCmd := AddRootCommand(&cobra.Command{
		Use: "prompt",
	})
	showCmd := AddCommand(promptCmd, &cobra.Command{
		Use:  "show",
		RunE: promptShowRunE,
	})

	editCmd := AddCommand(promptCmd, &cobra.Command{
		Use:  "edit",
		RunE: promptEditRunE,
	})
	editCmd.Flags().Bool("global", false, "global")
	editCmd.Flags().Bool("reasoner", false, "reasoner")
	showCmd.Flags().Bool("reasoner", false, "reasoner")
}

func promptShowRunE(cmd *cobra.Command, args []string) (err error) {
	reasoner, err := cmd.Flags().GetBool("reasoner")
	if err != nil {
		return
	}
	model := "chat"
	if reasoner {
		model = "reasoner"
	}
	promptTemplate := prompt.GetPromptTemplate(model)
	outfmt.Println(promptTemplate)
	return
}

func promptEditRunE(cmd *cobra.Command, args []string) (err error) {
	global, err := cmd.Flags().GetBool("global")
	if err != nil {
		return
	}
	reasoner, err := cmd.Flags().GetBool("reasoner")
	if err != nil {
		return
	}
	model := "chat"
	if reasoner {
		model = "reasoner"
	}
	p := prompt.GetPromptPath(model, global)
	if p == "" {
		err = fmt.Errorf("no prompt %s path found", model)
		return
	}
	promptTemplate := prompt.GetPromptTemplate(model)
	_, err = os.Stat(p)
	if err != nil {
		if !os.IsNotExist(err) {
			return
		}
		err = os.WriteFile(p, []byte(promptTemplate), 0o644)
		if err != nil {
			return
		}
	}
	ctx := cmd.Context()
	err = editor.Edit(ctx, p)
	return
}
