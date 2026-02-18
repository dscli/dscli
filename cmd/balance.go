package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"gitcode.com/nanjunjie/dscli/internal/log"
)

var balanceCmd = &cobra.Command{
	Use:   "balance",
	Short: "查询账户余额",
	Run: func(cmd *cobra.Command, args []string) {
		log.Info("开始查询账户余额")
		log.Info("开始查询账户余额")
		resp, err := client.Balance()
		log.Info("成功查询账户余额")
		if err != nil {
			fmt.Fprintf(os.Stderr, "查询余额失败: %v\n", err)
			os.Exit(1)
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "货币\t总余额\t赠送余额\t充值余额")
		for _, info := range resp.BalanceInfos {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", info.Currency, info.TotalBalance, info.GrantedBalance, info.ToppedUpBalance)
		}
		w.Flush()
		if !resp.IsAvailable {
			fmt.Fprintln(os.Stderr, "警告: 账户当前不可用")
		}
	},
}

func init() {
	rootCmd.AddCommand(balanceCmd)
}
