package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	balanceFormat string
	balanceCmd    = &cobra.Command{
		Use:   "balance",
		Short: "查询账户余额",
		Run: func(cmd *cobra.Command, args []string) {
			resp, err := DeepseekClient.Balance()
			if err != nil {
				fmt.Fprintf(os.Stderr, "查询余额失败: %v\n", err)
				os.Exit(1)
			}

			// 使用新的格式化接口
			headers := []string{"货币", "总余额", "赠送余额", "充值余额"}
			rowFunc := func(data any) []string {
				switch info := data.(type) {
				case BalanceInfo:
					return []string{info.Currency, info.TotalBalance, info.GrantedBalance, info.ToppedUpBalance}
				default:
					return []string{"", "", "", ""}
				}
			}

			err = FormatOutput(resp.BalanceInfos, balanceFormat, headers, rowFunc)
			if err != nil {
				fmt.Fprintf(os.Stderr, "格式化输出失败: %v\n", err)
				os.Exit(1)
			}

			if !resp.IsAvailable {
				fmt.Fprintln(os.Stderr, "警告: 账户当前不可用")
			}
		},
	}
)

func init() {
	balanceCmd.Flags().StringVarP(&balanceFormat, "format", "f", "table", "输出格式：table（表格）、json（JSON）")
	RootCmd.AddCommand(balanceCmd)
}
