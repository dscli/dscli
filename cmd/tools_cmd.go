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
	toolsDays      int
	toolsCategory  string
	toolsFormat    string
	toolsProject   string
	toolsOutput    string
)

var toolsCmd = &cobra.Command{
	Use:   "tools",
	Short: "查看工具使用统计",
	Long: `查看 dscli 工具的使用统计信息。
可以查看全局工具使用情况，也可以查看特定项目的工具使用情况。

示例：
  # 查看所有工具使用统计（最近30天）
  dscli tools list
  
  # 查看Git相关工具使用统计
  dscli tools list --category git
  
  # 查看最近7天的工具使用
  dscli tools list --days 7
  
  # 查看特定项目的工具使用
  dscli tools project --project /path/to/project
  
  # 查看项目最近30天的工具使用
  dscli tools project --project /path/to/project --days 30`,
}

var toolsListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出工具使用统计",
	RunE:  toolsListRunE,
}

var toolsProjectCmd = &cobra.Command{
	Use:   "project",
	Short: "查看项目工具使用统计",
	RunE:  toolsProjectRunE,
}

func toolsListRunE(cmd *cobra.Command, args []string) error {
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

func toolsProjectRunE(cmd *cobra.Command, args []string) error {
	// 确定项目路径
	projectRoot := toolsProject
	if projectRoot == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("获取当前目录失败: %w", err)
		}
		projectRoot = cwd
	}

	// 获取项目哈希
	projectHash := db.GetProjectHash(projectRoot)

	// 打开数据库
	database, err := db.New()
	if err != nil {
		return fmt.Errorf("初始化数据库失败: %w", err)
	}
	defer database.Close()

	// 获取项目工具使用统计
	stats, err := database.GetProjectToolUsage(projectHash, toolsDays)
	if err != nil {
		return fmt.Errorf("获取项目工具使用失败: %w", err)
	}

	// 输出结果
	switch toolsFormat {
	case "json":
		return outputProjectJSON(stats, projectRoot)
	case "csv":
		return outputProjectCSV(stats, projectRoot)
	default:
		return outputProjectTable(stats, projectRoot)
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

func outputProjectTable(stats []struct {
	Name       string
	UsageCount int
	LastUsed   time.Time
}, projectRoot string) error {
	if len(stats) == 0 {
		fmt.Printf("项目 '%s' 没有工具使用数据\n", projectRoot)
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "工具名称\t使用次数\t最后使用")
	fmt.Fprintln(w, "--------\t--------\t--------")

	for _, stat := range stats {
		lastUsed := ""
		if !stat.LastUsed.IsZero() {
			lastUsed = stat.LastUsed.Format("2006-01-02 15:04")
		}
		fmt.Fprintf(w, "%s\t%d\t%s\n", 
			stat.Name, stat.UsageCount, lastUsed)
	}

	w.Flush()
	
	// 输出统计信息
	fmt.Printf("\n📊 项目工具使用统计: %s\n", projectRoot)
	fmt.Printf("   工具使用数: %d\n", len(stats))
	
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

func outputProjectJSON(stats []struct {
	Name       string
	UsageCount int
	LastUsed   time.Time
}, projectRoot string) error {
	fmt.Printf("{\n")
	fmt.Printf("  \"project\": \"%s\",\n", projectRoot)
	fmt.Printf("  \"tools\": [\n")
	
	for i, stat := range stats {
		lastUsed := ""
		if !stat.LastUsed.IsZero() {
			lastUsed = stat.LastUsed.Format(time.RFC3339)
		}
		fmt.Printf("    {\n")
		fmt.Printf("      \"name\": \"%s\",\n", stat.Name)
		fmt.Printf("      \"usage_count\": %d,\n", stat.UsageCount)
		fmt.Printf("      \"last_used\": \"%s\"\n", lastUsed)
		if i < len(stats)-1 {
			fmt.Printf("    },\n")
		} else {
			fmt.Printf("    }\n")
		}
	}
	
	fmt.Printf("  ]\n")
	fmt.Printf("}\n")
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

func outputProjectCSV(stats []struct {
	Name       string
	UsageCount int
	LastUsed   time.Time
}, projectRoot string) error {
	fmt.Println("project,tool_name,usage_count,last_used")
	for _, stat := range stats {
		lastUsed := ""
		if !stat.LastUsed.IsZero() {
			lastUsed = stat.LastUsed.Format(time.RFC3339)
		}
		fmt.Printf("%s,%s,%d,%s\n", 
			projectRoot, stat.Name, stat.UsageCount, lastUsed)
	}
	return nil
}

func init() {
	// 添加list子命令标志
	toolsListCmd.Flags().IntVar(&toolsDays, "days", 30, "统计天数（0表示全部）")
	toolsListCmd.Flags().StringVar(&toolsCategory, "category", "", "工具分类过滤")
	toolsListCmd.Flags().StringVar(&toolsFormat, "format", "table", "输出格式：table, json, csv")
	toolsListCmd.Flags().StringVar(&toolsOutput, "output", "", "输出文件（默认输出到控制台）")
	
	// 添加project子命令标志
	toolsProjectCmd.Flags().StringVar(&toolsProject, "project", "", "项目路径（默认当前目录）")
	toolsProjectCmd.Flags().IntVar(&toolsDays, "days", 30, "统计天数（0表示全部）")
	toolsProjectCmd.Flags().StringVar(&toolsFormat, "format", "table", "输出格式：table, json, csv")
	toolsProjectCmd.Flags().StringVar(&toolsOutput, "output", "", "输出文件（默认输出到控制台）")
	
	// 添加子命令
	toolsCmd.AddCommand(toolsListCmd)
	toolsCmd.AddCommand(toolsProjectCmd)
	
	// 添加到根命令
	rootCmd.AddCommand(toolsCmd)
	
	log.Info("工具统计命令已注册")
}
