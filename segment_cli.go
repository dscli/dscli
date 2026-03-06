package main

import (
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

	segmentEditCmd.Flags().StringP("domain", "d", "", "领域名称")
	segmentEditCmd.Flags().StringP("model", "m", "", "模型 (chat|reasoner|all)")
	segmentEditCmd.Flags().StringP("name", "n", "", "段落名称")
	segmentEditCmd.Flags().IntP("order", "o", -1, "排序顺序 (-1表示不修改)")
	segmentEditCmd.Flags().StringP("content", "c", "", "段落内容")
	segmentEditCmd.Flags().Bool("enabled", true, "是否启用")
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

func segmentDeleteRunE(cmd *cobra.Command, args []string) error {
	// 解析ID
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("无效的段落ID: %s", args[0])
	}

	// 获取段落信息用于确认
	manager := NewSegmentManager()
	segment, err := manager.GetSegment(id)
	if err != nil {
		return fmt.Errorf("获取段落失败: %w", err)
	}

	// 确认删除
	fmt.Printf("⚠️  确认删除段落: [%d] %s\n", segment.ID, segment.Name)
	fmt.Printf("   内容预览: %s\n\n", truncateString(strings.TrimSpace(segment.Content), 80))
	fmt.Print("确定要删除吗？(y/N): ")

	var confirm string
	fmt.Scanln(&confirm)
	if strings.ToLower(confirm) != "y" {
		fmt.Println("取消删除")
		return nil
	}

	// 执行删除
	err = manager.DeleteSegment(id)
	if err != nil {
		return fmt.Errorf("删除段落失败: %w", err)
	}

	fmt.Printf("✅ 段落删除成功: [%d] %s\n", segment.ID, segment.Name)
	return nil
}

func segmentEditRunE(cmd *cobra.Command, args []string) error {
	// 解析ID
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("无效的段落ID: %s", args[0])
	}

	// 获取当前段落信息
	manager := NewSegmentManager()
	segment, err := manager.GetSegment(id)
	if err != nil {
		return fmt.Errorf("获取段落失败: %w", err)
	}

	// 获取命令行参数
	newDomain, _ := cmd.Flags().GetString("domain")
	newModel, _ := cmd.Flags().GetString("model")
	newName, _ := cmd.Flags().GetString("name")
	newOrder, _ := cmd.Flags().GetInt("order")
	newContent, _ := cmd.Flags().GetString("content")
	newEnabled, _ := cmd.Flags().GetBool("enabled")

	// 检查是否有命令行参数，决定使用哪种编辑模式
	hasCmdArgs := newDomain != "" || newModel != "" || newName != "" || newOrder != -1 || newContent != ""

	if hasCmdArgs {
		// 命令行参数模式
		return editSegmentWithArgs(id, segment, manager, newDomain, newModel, newName, newOrder, newContent, newEnabled)
	} else {
		// 交互式编辑模式
		return editSegmentInteractive(id, segment, manager)
	}
}

func editSegmentWithArgs(id int64, segment *PromptSegment, manager *SegmentManager,
	newDomain, newModel, newName string, newOrder int, newContent string, newEnabled bool,
) error {
	db, err := OpenDB()
	if err != nil {
		return err
	}
	defer db.Close()

	// 使用原值作为默认值
	finalDomain := newDomain
	finalModel := newModel
	finalName := newName
	finalOrder := newOrder
	finalContent := newContent
	finalEnabled := newEnabled

	// 获取当前领域名称
	var currentDomainName string
	err = db.QueryRow("SELECT name FROM domains WHERE id = ?", segment.DomainID).Scan(&currentDomainName)
	if err != nil {
		currentDomainName = "unknown"
	}

	// 处理领域
	if finalDomain == "" {
		finalDomain = currentDomainName
	}

	// 处理模型
	if finalModel == "" {
		// 获取当前模型名称
		currentModelName := "all"
		if segment.ModelID == DeepseekChat {
			currentModelName = "chat"
		} else if segment.ModelID == DeepseekReasoner {
			currentModelName = "reasoner"
		}
		finalModel = currentModelName
	}

	// 处理名称
	if finalName == "" {
		finalName = segment.Name
	}

	// 处理排序
	if finalOrder == -1 {
		finalOrder = segment.SortOrder
	}

	// 处理内容
	if finalContent == "" {
		finalContent = segment.Content
	}

	// 获取领域ID
	var finalDomainID int64
	err = db.QueryRow("SELECT id FROM domains WHERE name = ?", finalDomain).Scan(&finalDomainID)
	if err != nil {
		return fmt.Errorf("领域不存在: %s", finalDomain)
	}

	// 解析模型ID
	var finalModelID int64 = -1 // 通用
	switch strings.ToLower(finalModel) {
	case "chat":
		finalModelID = DeepseekChat
	case "reasoner":
		finalModelID = DeepseekReasoner
	case "all", "通用":
		finalModelID = -1
	default:
		return fmt.Errorf("无效的模型: %s", finalModel)
	}

	// 显示将要更新的信息
	fmt.Println("将更新以下信息:")
	fmt.Printf("  ID: %d\n", segment.ID)
	fmt.Printf("  名称: %s → %s\n", segment.Name, finalName)
	fmt.Printf("  领域: %s → %s\n", currentDomainName, finalDomain)
	fmt.Printf("  模型: %d → %s\n", segment.ModelID, finalModel)
	fmt.Printf("  排序: %d → %d\n", segment.SortOrder, finalOrder)
	fmt.Printf("  启用: %v → %v\n", segment.Enabled, finalEnabled)
	fmt.Println("  内容: (将更新)")

	// 确认更新
	fmt.Print("\n确定要更新吗？(y/N): ")
	var confirm string
	fmt.Scanln(&confirm)
	if strings.ToLower(confirm) != "y" {
		fmt.Println("取消更新")
		return nil
	}

	// 更新段落
	err = manager.UpdateSegment(id, finalDomainID, finalModelID, finalName, finalContent, finalOrder, finalEnabled)
	if err != nil {
		return fmt.Errorf("更新段落失败: %w", err)
	}

	fmt.Printf("✅ 段落更新成功: [%d] %s\n", segment.ID, finalName)
	return nil
}

func editSegmentInteractive(id int64, segment *PromptSegment, manager *SegmentManager) error {
	db, err := OpenDB()
	if err != nil {
		return err
	}
	defer db.Close()

	fmt.Printf("📝 编辑段落: [%d] %s\n\n", segment.ID, segment.Name)
	fmt.Println("当前信息:")
	fmt.Printf("  ID: %d\n", segment.ID)
	fmt.Printf("  名称: %s\n", segment.Name)

	// 获取领域名称
	var domainName string
	err = db.QueryRow("SELECT name FROM domains WHERE id = ?", segment.DomainID).Scan(&domainName)
	if err != nil {
		domainName = "unknown"
	}
	fmt.Printf("  领域: %s\n", domainName)

	// 显示模型
	modelName := "通用"
	if segment.ModelID == DeepseekChat {
		modelName = "Chat"
	} else if segment.ModelID == DeepseekReasoner {
		modelName = "Reasoner"
	}
	fmt.Printf("  模型: %s\n", modelName)
	fmt.Printf("  排序: %d\n", segment.SortOrder)
	fmt.Printf("  启用: %v\n", segment.Enabled)
	fmt.Printf("  内容:\n---\n%s\n---\n\n", segment.Content)

	// 交互式编辑
	fmt.Println("开始编辑（直接回车保持原值）:")

	// 编辑名称
	fmt.Printf("新名称 [%s]: ", segment.Name)
	var newName string
	fmt.Scanln(&newName)
	if newName == "" {
		newName = segment.Name
	}

	// 编辑领域
	fmt.Printf("新领域 [%s]: ", domainName)
	var newDomain string
	fmt.Scanln(&newDomain)
	if newDomain == "" {
		newDomain = domainName
	}

	// 编辑模型
	fmt.Printf("新模型 (chat/reasoner/all) [%s]: ", modelName)
	var newModel string
	fmt.Scanln(&newModel)
	if newModel == "" {
		newModel = modelName
	}

	// 编辑排序
	fmt.Printf("新排序 [%d]: ", segment.SortOrder)
	var newOrderStr string
	fmt.Scanln(&newOrderStr)
	newOrder := segment.SortOrder
	if newOrderStr != "" {
		if order, err := strconv.Atoi(newOrderStr); err == nil {
			newOrder = order
		}
	}

	// 编辑启用状态
	fmt.Printf("启用 (true/false) [%v]: ", segment.Enabled)
	var enabledStr string
	fmt.Scanln(&enabledStr)
	newEnabled := segment.Enabled
	if enabledStr != "" {
		newEnabled = strings.ToLower(enabledStr) == "true"
	}

	// 编辑内容
	fmt.Println("编辑内容（输入空行结束，输入'.'保持原内容）:")
	fmt.Println("当前内容:")
	fmt.Println("---")
	fmt.Println(segment.Content)
	fmt.Println("---")
	fmt.Println("请输入新内容:")

	lines := []string{}
	for {
		var line string
		fmt.Scanln(&line)
		if line == "" {
			break
		}
		if line == "." && len(lines) == 0 {
			// 保持原内容
			lines = nil
			break
		}
		lines = append(lines, line)
	}

	newContent := segment.Content
	if lines != nil {
		newContent = strings.Join(lines, "\n")
	}

	// 获取领域ID
	var newDomainID int64
	err = db.QueryRow("SELECT id FROM domains WHERE name = ?", newDomain).Scan(&newDomainID)
	if err != nil {
		return fmt.Errorf("领域不存在: %s", newDomain)
	}

	// 解析模型ID
	var newModelID int64 = -1 // 通用
	switch strings.ToLower(newModel) {
	case "chat":
		newModelID = DeepseekChat
	case "reasoner":
		newModelID = DeepseekReasoner
	case "all", "通用":
		newModelID = -1
	default:
		return fmt.Errorf("无效的模型: %s", newModel)
	}

	// 更新段落
	err = manager.UpdateSegment(id, newDomainID, newModelID, newName, newContent, newOrder, newEnabled)
	if err != nil {
		return fmt.Errorf("更新段落失败: %w", err)
	}

	fmt.Printf("✅ 段落更新成功: [%d] %s\n", segment.ID, newName)
	return nil
}
