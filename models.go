package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/dscli/dscli/internal/price"
	"github.com/nanjj/clog"
	"github.com/spf13/cobra"
)

// modelPriceRow is one output row: model ID plus its current
// per-million-token prices in yuan (listed prices before 2026-08-17,
// peak/off-peak prices afterwards).
type modelPriceRow struct {
	ID              string  `json:"id"`
	PromptCacheHit  float64 `json:"prompt_cache_hit,omitempty"`
	PromptCacheMiss float64 `json:"prompt_cache_miss,omitempty"`
	Completion      float64 `json:"completion,omitempty"`
}

var modelsFormat string

// modelsRefresh forces a pricing-page refetch, ignoring the daily cache TTL.
var modelsRefresh bool

func init() {
	modelsCmd := AddRootCommand(&cobra.Command{
		Use:   "models",
		Short: "List DeepSeek models with current token prices",
		Run:   ModelsRun,
	})
	modelsCmd.Flags().StringVarP(&modelsFormat, "format", "f", "table", "Output format: table (default), json")
	modelsCmd.Flags().BoolVar(&modelsRefresh, "refresh", false, "force refresh the price cache (ignore the 24h TTL)")
}

func ModelsRun(cmd *cobra.Command, args []string) {
	span, ctx := clog.StartSpanFromContext(cmd.Context(), "ModelRun")
	defer span.Finish()

	resp, err := DeepseekClient.Models(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "model list query failed: %v\n", err)
		os.Exit(1)
	}

	if modelsRefresh {
		if err := price.ForceRefresh(); err != nil {
			fmt.Fprintf(os.Stderr, "pricing refresh failed: %v (using cached prices)\n", err)
		}
	}

	rows := make([]modelPriceRow, 0, len(resp.Data))
	for _, m := range resp.Data {
		row := modelPriceRow{ID: m.ID}
		if p, ok := price.GetPriceFor(m.ID); ok {
			row.PromptCacheHit = p.PromptCacheHit
			row.PromptCacheMiss = p.PromptCacheMiss
			row.Completion = p.Completion
		}
		rows = append(rows, row)
	}

	headers := []string{"ID", "Cache Hit", "Cache Miss", "Output"}
	rowFunc := func(data any) []string {
		switch r := data.(type) {
		case modelPriceRow:
			return []string{r.ID, formatPrice(r.PromptCacheHit), formatPrice(r.PromptCacheMiss), formatPrice(r.Completion)}
		default:
			return []string{"", "", "", ""}
		}
	}

	err = FormatOutput(rows, modelsFormat, headers, rowFunc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "output formatting failed: %v\n", err)
		os.Exit(1)
	}
}

// formatPrice renders a price, leaving unknown prices blank.
func formatPrice(f float64) string {
	if f == 0 {
		return ""
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}
