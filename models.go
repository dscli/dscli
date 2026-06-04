package main

import (
	"fmt"
	"os"
	"strconv"

	"gitcode.com/dscli/dscli/internal/dsc"
	"gitcode.com/dscli/dscli/internal/price"
	"github.com/spf13/cobra"
)

var modelsFormat string
var modelsPrice bool

// priceRow 价格表格的行
type priceRow struct {
	Model           string
	PromptCacheHit  float64
	PromptCacheMiss float64
	Completion      float64
}

func init() {
	modelsCmd := AddRootCommand(&cobra.Command{
		Use:   "models",
		Short: "列出 DeepSeek 支持的模型",
		Run:   ModelsRun,
	})
	modelsCmd.Flags().StringVarP(&modelsFormat, "format", "f", "table", "输出格式：table（表格）、json（JSON）")
	modelsCmd.Flags().BoolVarP(&modelsPrice, "price", "p", false, "列出模型价格")
}

func ModelsRun(cmd *cobra.Command, args []string) {
	if modelsPrice {
		priceData := price.GetPrice()
		if priceData == nil {
			fmt.Fprintf(os.Stderr, "获取价格信息失败\n")
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

		headers := []string{"模型", "缓存命中", "缓存未命中", "输出"}
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
			fmt.Fprintf(os.Stderr, "格式化输出失败: %v\n", err)
			os.Exit(1)
		}
		return
	}

	resp, err := DeepseekClient.Models()
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取模型列表失败: %v\n", err)
		os.Exit(1)
	}

	// 使用新的格式化接口
	headers := []string{"ID", "对象", "拥有者"}
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
		fmt.Fprintf(os.Stderr, "格式化输出失败: %v\n", err)
		os.Exit(1)
	}
}
