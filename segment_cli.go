package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

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

// segmentDeleteCmd 删除段落
var segmentDeleteCmd = AddCommand(segmentCmd, &cobra.Command{
	Use:   "delete <id>",
	Short: "删除段落",
	Args:  cobra.ExactArgs(1),
	RunE:  segmentDeleteRunE,
})

// segmentEditCmd 编辑段落
var segmentEditCmd = AddCommand(segmentCmd, &cobra.Command{
	Use:   "edit <id>",
	Short: "编辑段落",
	Args:  cobra.ExactArgs(1),
	RunE:  segmentEditRunE,
})

func init() {
	// 添加标志
	segmentListCmd.Flags().Int64P("domain", "d", 0, "按领域ID筛选")
	segmentListCmd.Flags().Int64P("model", "m", -2, "按模型ID筛选 (-2:不筛选, -1:通用, 0:chat, 1:reasoner)")

	segmentCreateCmd.Flags().Int64P("domain", "d", 0, "领域ID (0:系统级, >0:领域级)")
	segmentCreateCmd.Flags().Int64P("model", "m", -1, "模型ID (-1:通用, 0:chat, 1:reasoner)")
	segmentCreateCmd.Flags().StringP("name", "n", "", "段落名称")
	segmentCreateCmd.Flags().IntP("order", "o", 0, "排序顺序")
	segmentCreateCmd.Flags().StringP("content", "c", "", "段落内容")
	segmentCreateCmd.MarkFlagRequired("name")

	segmentEditCmd.Flags().StringP("name", "n", "", "段落名称")
	segmentEditCmd.Flags().IntP("order", "o", -1, "排序顺序 (-1表示不修改)")
	segmentEditCmd.Flags().StringP("content", "c", "", "段落内容")
	segmentEditCmd.Flags().Bool("enabled", true, "是否启用")
}

func segmentListRunE(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	sm := &SegmentManager{}

	domainID, _ := cmd.Flags().GetInt64("domain")
	modelID, _ := cmd.Flags().GetInt64("model")

	segments, err := sm.ListSegments(ctx)
	if err != nil {
		return fmt.Errorf("列出段落失败: %w", err)
	}

	// 过滤结果
	var filtered []PromptSegment
	for _, seg := range segments {
		if domainID != 0 && seg.DomainID != domainID {
			continue
		}
		if modelID != -2 && seg.ModelID != modelID {
			continue
		}
		filtered = append(filtered, seg)
	}

	if len(filtered) == 0 {
		Println("没有找到段落")
		return nil
	}

	Printf("找到 %d 个段落:\n\n", len(filtered))
	for _, seg := range filtered {
		domainType := "系统级"
		if seg.DomainID > 0 {
			domainType = "领域级"
		}

		modelName := "通用"
		switch seg.ModelID {
		case DeepseekChat:
			modelName = "Chat"
		case DeepseekReasoner:
			modelName = "Reasoner"
		}

		status := "✅"
		if !seg.Enabled {
			status = "❌"
		}

		Printf("%s [%d] %s (领域ID: %d[%s], 模型: %s, 排序: %d)\n",
			status, seg.ID, seg.Name, seg.DomainID, domainType, modelName, seg.SortOrder)

		// 显示内容预览
		preview := strings.TrimSpace(seg.Content)
		if len(preview) > 80 {
			preview = preview[:77] + "..."
		}
		if preview != "" {
			Printf("   内容预览: %s\n", preview)
		}
		Println()
	}

	return nil
}

func segmentCreateRunE(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	sm := &SegmentManager{}

	domainID, _ := cmd.Flags().GetInt64("domain")
	modelID, _ := cmd.Flags().GetInt64("model")
	name, _ := cmd.Flags().GetString("name")
	order, _ := cmd.Flags().GetInt("order")
	content, _ := cmd.Flags().GetString("content")

	if content == "" {
		Println("请输入段落内容（以空行结束）:")
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

	err := sm.CreateSegment(ctx, domainID, modelID, name, content, order)
	if err != nil {
		return fmt.Errorf("创建段落失败: %w", err)
	}

	Printf("✅ 段落创建成功: %s\n", name)
	return nil
}

func segmentDeleteRunE(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	sm := &SegmentManager{}

	// 解析ID
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("无效的段落ID: %s", args[0])
	}

	// 获取段落信息用于确认
	segment, err := sm.GetSegmentByID(ctx, id)
	if err != nil {
		return fmt.Errorf("获取段落失败: %w", err)
	}

	// 确认删除
	Printf("⚠️  确认删除段落: [%d] %s\n", segment.ID, segment.Name)
	Printf("   内容预览: %s\n\n", TruncateString(strings.TrimSpace(segment.Content), 80))
	Printf("确定要删除吗？(y/N): ")

	var confirm string
	fmt.Scanln(&confirm)
	if strings.ToLower(confirm) != "y" {
		Println("取消删除")
		return nil
	}

	// 执行删除
	err = sm.DeleteSegment(ctx, id)
	if err != nil {
		return fmt.Errorf("删除段落失败: %w", err)
	}

	Printf("✅ 段落删除成功: [%d] %s\n", segment.ID, segment.Name)
	return nil
}

func segmentEditRunE(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	sm := &SegmentManager{}

	// 解析ID
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("无效的段落ID: %s", args[0])
	}

	// 获取当前段落信息
	segment, err := sm.GetSegmentByID(ctx, id)
	if err != nil {
		return fmt.Errorf("获取段落失败: %w", err)
	}

	// 获取命令行参数
	newName, _ := cmd.Flags().GetString("name")
	newOrder, _ := cmd.Flags().GetInt("order")
	newContent, _ := cmd.Flags().GetString("content")
	newEnabled, _ := cmd.Flags().GetBool("enabled")

	// 使用原值作为默认值
	finalName := segment.Name
	if newName != "" {
		finalName = newName
	}

	finalOrder := segment.SortOrder
	if newOrder != -1 {
		finalOrder = newOrder
	}

	finalContent := segment.Content
	if newContent != "" {
		finalContent = newContent
	}

	finalEnabled := newEnabled

	// 显示将要更新的信息
	Println("将更新以下信息:")
	Printf("  ID: %d\n", segment.ID)
	Printf("  名称: %s → %s\n", segment.Name, finalName)
	Printf("  排序: %d → %d\n", segment.SortOrder, finalOrder)
	Printf("  启用: %v → %v\n", segment.Enabled, finalEnabled)
	Println("  内容: (将更新)")

	// 确认更新
	Println("\n确定要更新吗？(y/N): ")
	var confirm string
	fmt.Scanln(&confirm)
	if strings.ToLower(confirm) != "y" {
		Println("取消更新")
		return nil
	}

	// 更新段落
	err = sm.UpdateSegment(ctx, id, finalName, finalContent, finalOrder, finalEnabled)
	if err != nil {
		return fmt.Errorf("更新段落失败: %w", err)
	}

	Printf("✅ 段落更新成功: [%d] %s\n", segment.ID, finalName)
	return nil
}

// TruncateString 截断字符串
// TruncateString 截断字符串
// 使用[]rune处理Unicode字符，避免截断时出现乱码
// TruncateString 截断字符串
// 使用[]rune处理Unicode字符，避免截断时出现乱码
// TruncateString 截断字符串
// 使用[]rune处理Unicode字符，避免截断时出现乱码
// 示例：TruncateString("你好世界", 5) 返回 "你好..."
// TruncateString 截断字符串
// 使用[]rune处理Unicode字符，避免截断时出现乱码
// 示例：TruncateString("你好世界", 5) 返回 "你好..."
// 示例：TruncateString("Hello World", 8) 返回 "Hello..."
func TruncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-3]) + "..."
}
