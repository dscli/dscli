package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var balanceFormat string

func init() {
	balanceCmd := AddRootCommand(&cobra.Command{
		Use:   "balance",
		Short: "Query account balance",
		RunE:  BalanceRunE,
	})
	balanceCmd.Flags().StringVarP(&balanceFormat, "format", "f", "table", "Output format: table (default), json")
}

func BalanceRunE(cmd *cobra.Command, args []string) (err error) {
	resp, err := DeepseekClient.Balance()
	if err != nil {
		fmt.Fprintf(os.Stderr, "balance query failed: %v\n", err)
		return nil
	}

	headers := []string{"Currency", "Total Balance", "Granted Balance", "Topped-up Balance"}
	rowFunc := func(data any) []string {
		switch info := data.(type) {
		case map[string]string:
			return []string{info["currency"], info["total_balance"], info["granted_balance"], info["topped_up_balance"]}
		default:
			return []string{"", "", "", ""}
		}
	}

	err = FormatOutput(resp.BalanceInfos, balanceFormat, headers, rowFunc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "output formatting failed: %v\n", err)
		os.Exit(1)
	}

	if !resp.IsAvailable {
		fmt.Fprintln(os.Stderr, "warning: account currently unavailable")
	}
	return nil
}
