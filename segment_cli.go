package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// segmentCmd 段落管理命令
var segmentCmd = AddRootCommand(&cobra.Command{
	Use:   "segment",
	Short: "管理提示词段落",
	Long:  `管理数据库中的提示词段落，支持模板化内容`,
})

// segmentListCmd 列出段落
var segmentListCmd = AddCommand(segmentCmd, &cobra.Command{
	Use:   "list",
	Short: "列出所有段落",
	RunE:  segmentListRunE,
})

// segmentCreateCmd 创建段落
var segmentCreateCmd = AddCommand(segmentCmd, &cobra.Command{
	Use:   "create",
	Short: "创建新段落",
	RunE:  segmentCreateRunE,
})

// segmentPreviewCmd 预览段落
var segmentPreviewCmd = AddCommand(segmentCmd, &cobra.Command{
	Use:   "preview [id|content]",
	Short: "预览段落渲染结果",
	Args:  cobra.MinimumNArgs(1),
	RunE:  segmentPreviewRunE,
})

// segmentTestCmd 测试模板
var segmentTestCmd = AddCommand(segmentCmd, &cobra.Command{
	Use:   "test",
	Short: "测试模板语法",
	RunE:  segmentTestRunE,
})

// segmentExamplesCmd 查看示例
var segmentExamplesCmd = AddCommand(segmentCmd, &cobra.Command{
	Use:   "examples",
	Short: "查看段落示例",
	RunE:  segmentExampleRunE,
})

// truncateString 截断字符串
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func init() {
	// 添加标志
	segmentListCmd.Flags().StringP("domain", "d", "", "按领域筛选")
	segmentListCmd.Flags().StringP("model", "m", "", "按模型筛选 (chat|reasoner|all)")

	segmentCreateCmd.Flags().StringP("domain", "d", "programming", "领域名称")
	segmentCreateCmd.Flags().StringP("model", "m", "all", "模型 (chat|reasoner|all)")
	segmentCreateCmd.Flags().StringP("name", "n", "", "段落名称")
	segmentCreateCmd.Flags().IntP("order", "o", 0, "排序顺序")
	segmentCreateCmd.Flags().StringP("content", "c", "", "段落内容")
	segmentCreateCmd.MarkFlagRequired("name")
}

func segmentListRunE(cmd *cobra.Command, args []string) error {
	manager := NewSegmentManager()

	domain, _ := cmd.Flags().GetString("domain")
	model, _ := cmd.Flags().GetString("model")

	var modelID int64 = -2 // 不筛选
	if model != "" {
		switch model {
		case "chat":
			modelID = DeepseekChat
		case "reasoner":
			modelID = DeepseekReasoner
		case "all":
			modelID = -1
		default:
			return fmt.Errorf("无效的模型: %s", model)
		}
	}

	segments, err := manager.ListSegments(domain, modelID)
	if err != nil {
		return err
	}

	if len(segments) == 0 {
		fmt.Println("没有找到段落")
		return nil
	}

	fmt.Printf("找到 %d 个段落:\n\n", len(segments))
	for _, seg := range segments {
		modelName := "通用"
		if seg.ModelID == DeepseekChat {
			modelName = "Chat"
		} else if seg.ModelID == DeepseekReasoner {
			modelName = "Reasoner"
		}

		status := "✅"
		if !seg.Enabled {
			status = "❌"
		}

		fmt.Printf("%s [%d] %s (模型: %s, 排序: %d)\n",
			status, seg.ID, seg.Name, modelName, seg.SortOrder)
		fmt.Printf("   内容预览: %s\n\n",
			truncateString(strings.TrimSpace(seg.Content), 80))
	}

	return nil
}

func segmentCreateRunE(cmd *cobra.Command, args []string) error {
	domain, err := cmd.Flags().GetString("domain")
	if err != nil {
		return err
	}
	model, err := cmd.Flags().GetString("model")
	if err != nil {
		return err
	}
	name, err := cmd.Flags().GetString("name")
	if err != nil {
		return err
	}
	order, err := cmd.Flags().GetInt("order")
	if err != nil {
		return err
	}

	// 获取内容
	content, err := cmd.Flags().GetString("content")
	if err != nil {
		return err
	}

	if content == "" {
		// 可以从文件读取或交互式输入
		fmt.Println("请输入段落内容（以空行结束）:")
		lines := []string{}
		for {
			var line string
			fmt.Scanln(&line)
			if line == "" {
				break
			}
			lines = append(lines, line)
		}
		content = strings.Join(lines, "\n")
	}

	// 获取领域ID
	db, err := OpenDB()
	if err != nil {
		return err
	}
	defer db.Close()

	var domainID int64
	err = db.QueryRow("SELECT id FROM domains WHERE name = ?", domain).Scan(&domainID)
	if err != nil {
		return fmt.Errorf("领域不存在: %s", domain)
	}

	// 解析模型ID
	var modelID int64 = -1 // 通用
	switch model {
	case "chat":
		modelID = DeepseekChat
	case "reasoner":
		modelID = DeepseekReasoner
	case "all", "":
		modelID = -1
	default:
		return fmt.Errorf("无效的模型: %s", model)
	}

	// 创建段落
	err = CreateSegment(domainID, modelID, name, content, order)
	if err != nil {
		return fmt.Errorf("创建段落失败: %w", err)
	}

	fmt.Printf("✅ 段落创建成功: %s\n", name)
	return nil
}

func segmentPreviewRunE(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	manager := NewSegmentManager()

	input := args[0]

	// 判断是ID还是内容
	if id, err := strconv.ParseInt(input, 10, 64); err == nil {
		// 是ID，从数据库获取
		segment, err := manager.GetSegment(id)
		if err != nil {
			return fmt.Errorf("获取段落失败: %w", err)
		}

		fmt.Printf("📝 段落: %s (ID: %d)\n\n", segment.Name, segment.ID)
		fmt.Println("原始内容:")
		fmt.Println("---")
		fmt.Println(segment.Content)
		fmt.Println("---")

		// 预览渲染结果
		result, err := manager.PreviewSegment(ctx, segment.Content)
		if err != nil {
			return fmt.Errorf("渲染失败: %w", err)
		}

		fmt.Println("渲染结果:")
		fmt.Println("---")
		fmt.Println(result)
		fmt.Println("---")

	} else {
		// 是内容，直接预览
		fmt.Println("预览模板内容:")
		fmt.Println("---")
		fmt.Println(input)
		fmt.Println("---")

		result, err := manager.PreviewSegment(ctx, input)
		if err != nil {
			return fmt.Errorf("渲染失败: %w", err)
		}

		fmt.Println("渲染结果:")
		fmt.Println("---")
		fmt.Println(result)
		fmt.Println("---")
	}

	return nil
}

func segmentTestRunE(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	manager := NewSegmentManager()

	fmt.Println("输入模板内容进行测试（以空行结束）:")
	lines := []string{}
	for {
		var line string
		fmt.Scanln(&line)
		if line == "" {
			break
		}
		lines = append(lines, line)
	}

	templateStr := strings.Join(lines, "\n")

	result, err := manager.TestSegmentTemplate(ctx, templateStr)
	if err != nil {
		return fmt.Errorf("模板测试失败: %w", err)
	}

	fmt.Println("\n✅ 模板语法正确")
	fmt.Println("\n渲染结果:")
	fmt.Println("---")
	fmt.Println(result)
	fmt.Println("---")

	return nil
}

func segmentExampleRunE(cmd *cobra.Command, args []string) error {
	fmt.Println("可用段落示例:")
	fmt.Println()

	for i, name := range AllExampleNames() {
		example := GetExampleSegment(name)
		fmt.Printf("%d. %s\n", i+1, name)
		fmt.Printf("   预览: %s\n\n",
			truncateString(strings.TrimSpace(example), 60))
	}

	fmt.Println("\n使用示例:")
	fmt.Println(ExampleUsage())

	return nil
}
