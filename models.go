package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/dscli/dscli/internal/dsc"
	"github.com/dscli/dscli/internal/price"
	"github.com/spf13/cobra"
)

// Price stores model pricing information
type Price struct {
	PromptCacheHit  float64
	PromptCacheMiss float64
	Completion      float64
}

type priceRow struct {
	Model           string
	PromptCacheHit  float64
	PromptCacheMiss float64
	Completion      float64
}

var modelsFormat string
var modelsPrice bool

func init() {
	modelsCmd := AddRootCommand(&cobra.Command{
		Use:   "models",
		Short: "List DeepSeek supported models",
		Run:   ModelsRun,
	})
	modelsCmd.Flags().StringVarP(&modelsFormat, "format", "f", "table", "Output format: table (default), json")
	modelsCmd.Flags().BoolVarP(&modelsPrice, "price", "p", false, "List model prices")
}

func ModelsRun(cmd *cobra.Command, args []string) {
	if modelsPrice {
		priceData := price.GetPrice()
		if priceData == nil {
			fmt.Fprintf(os.Stderr, "failed to get price info\n")
			os.Exit(1)
		}

		rows := make([]priceRow, 0, len(priceData))
		for model, p := range priceData {
			rows = append(rows, priceRow{
				Model:           model,
				PromptCacheHit:  p.PromptCacheHit,
				PromptCacheMiss: p.PromptCacheMiss,
				Completion:      p.Completion,
			})
		}

		headers := []string{"Model", "Cache Hit", "Cache Miss", "Output"}
		rowFunc := func(data any) []string {
			switch r := data.(type) {
			case priceRow:
				return []string{
					r.Model,
					strconv.FormatFloat(r.PromptCacheHit, 'f', -1, 64),
					strconv.FormatFloat(r.PromptCacheMiss, 'f', -1, 64),
					strconv.FormatFloat(r.Completion, 'f', -1, 64),
				}
			default:
				return []string{"", "", "", ""}
			}
		}

		err := FormatOutput(rows, modelsFormat, headers, rowFunc)
		if err != nil {
			fmt.Fprintf(os.Stderr, "output formatting failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	resp, err := DeepseekClient.Models()
	if err != nil {
		fmt.Fprintf(os.Stderr, "model list query failed: %v\n", err)
		os.Exit(1)
	}

	// Use new formatting interface
	headers := []string{"ID", "Object", "Owner"}
	rowFunc := func(data any) []string {
		switch m := data.(type) {
		case dsc.Model:
			return []string{m.ID, m.Object, m.OwnedBy}
		default:
			return []string{"", "", ""}
		}
	}

	err = FormatOutput(resp.Data, modelsFormat, headers, rowFunc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "output formatting failed: %v\n", err)
		os.Exit(1)
	}
}
