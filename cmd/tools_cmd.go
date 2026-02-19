package cmd

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"gitcode.com/nanjunjie/dscli/internal/db"
	"gitcode.com/nanjunjie/dscli/internal/log"
	"github.com/spf13/cobra"
)

var (
	toolsDays     int
	toolsCategory string
	toolsFormat   string
)

var toolsCmd = &cobra.Command{
	Use:   "tools",
	Short: "查看工具使用统计",
	Long: `查看 dscli 工具的使用统计信息。
显示所有工具的使用次数、成功率和最后使用时间。

示例：
  # 查看所有工具使用统计（最近30天）
  dscli tools
  
  # 查看Git相关工具使用统计
  dscli tools --category git
  
  # 查看最近7天的工具使用
  dscli tools --days 7
  
  # JSON格式输出
  dscli tools --format json
  
  # CSV格式输出
  dscli tools --format csv`,
	RunE: toolsRunE,
}

func toolsRunE(cmd *cobra.Command, args []string) error {
	// 打开数据库
	database, err := db.New()
	if err != nil {
		return fmt.Errorf("初始化数据库失败: %w", err)
	}
	defer database.Close()

	// 获取工具使用统计
	stats, err := database.GetToolUsageStats(toolsDays)
	if err != nil {
		return fmt.Errorf("获取工具统计失败: %w", err)
	}

	// 过滤分类
	var filteredStats []struct {
		Name       string
		UsageCount int
		SuccessRate float64
		LastUsed   time.Time
	}
	
	for _, stat := range stats {
		if toolsCategory == "" || strings.Contains(strings.ToLower(stat.Name), strings.ToLower(toolsCategory)) {
			filteredStats = append(filteredStats, stat)
		}
	}

	// 输出结果
	switch toolsFormat {
	case "json":
		return outputJSON(filteredStats)
	case "csv":
		return outputCSV(filteredStats)
	default:
		return outputTable(filteredStats)
	}
}

func outputTable(stats []struct {
	Name       string
	UsageCount int
	SuccessRate float64
	LastUsed   time.Time
}) error {
	if len(stats) == 0 {
		fmt.Println("没有找到工具使用数据")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "工具名称\t使用次数\t成功率\t最后使用")
	fmt.Fprintln(w, "--------\t--------\t------\t--------")

	for _, stat := range stats {
		lastUsed := ""
		if !stat.LastUsed.IsZero() {
			lastUsed = stat.LastUsed.Format("2006-01-02")
		}
		fmt.Fprintf(w, "%s\t%d\t%.1f%%\t%s\n", 
			stat.Name, stat.UsageCount, stat.SuccessRate, lastUsed)
	}

	w.Flush()
	
	// 输出统计信息
	fmt.Printf("\n📊 统计摘要:\n")
	fmt.Printf("   工具总数: %d\n", len(stats))
	
	var totalUsage int
	for _, stat := range stats {
		totalUsage += stat.UsageCount
	}
	fmt.Printf("   总使用次数: %d\n", totalUsage)
	
	if toolsDays > 0 {
		fmt.Printf("   时间范围: 最近 %d 天\n", toolsDays)
	}
	
	return nil
}

func outputJSON(stats []struct {
	Name       string
	UsageCount int
	SuccessRate float64
	LastUsed   time.Time
}) error {
	// 简化的JSON输出
	fmt.Println("[")
	for i, stat := range stats {
		lastUsed := ""
		if !stat.LastUsed.IsZero() {
			lastUsed = stat.LastUsed.Format(time.RFC3339)
		}
		fmt.Printf("  {\n")
		fmt.Printf("    \"name\": \"%s\",\n", stat.Name)
		fmt.Printf("    \"usage_count\": %d,\n", stat.UsageCount)
		fmt.Printf("    \"success_rate\": %.2f,\n", stat.SuccessRate)
		fmt.Printf("    \"last_used\": \"%s\"\n", lastUsed)
		if i < len(stats)-1 {
			fmt.Printf("  },\n")
		} else {
			fmt.Printf("  }\n")
		}
	}
	fmt.Println("]")
	return nil
}

func outputCSV(stats []struct {
	Name       string
	UsageCount int
	SuccessRate float64
	LastUsed   time.Time
}) error {
	fmt.Println("name,usage_count,success_rate,last_used")
	for _, stat := range stats {
		lastUsed := ""
		if !stat.LastUsed.IsZero() {
			lastUsed = stat.LastUsed.Format(time.RFC3339)
		}
		fmt.Printf("%s,%d,%.2f,%s\n", 
			stat.Name, stat.UsageCount, stat.SuccessRate, lastUsed)
	}
	return nil
}

func init() {
	// 添加命令标志
	toolsCmd.Flags().IntVar(&toolsDays, "days", 30, "统计天数（0表示全部）")
	toolsCmd.Flags().StringVar(&toolsCategory, "category", "", "工具分类过滤")
	toolsCmd.Flags().StringVar(&toolsFormat, "format", "table", "输出格式：table, json, csv")
	
	// 添加到根命令
	rootCmd.AddCommand(toolsCmd)
	
	log.Info("工具统计命令已注册")
}
