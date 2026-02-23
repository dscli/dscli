package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// TestSkillsDatabase 测试Skills数据库表结构
func TestSkillsDatabase(t *testing.T) {
	// 使用测试数据库，避免影响生产数据
	testDBPath := filepath.Join(t.TempDir(), "test_skills.db")
	
	// 创建测试数据库
	db, err := sql.Open("sqlite3", testDBPath)
	if err != nil {
		t.Fatalf("无法打开测试数据库: %v", err)
	}
	defer db.Close()
	
	// 创建skills表
	_, err = db.Exec(`
		CREATE TABLE skills (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			description TEXT NOT NULL,
			content TEXT NOT NULL,
			category TEXT,
			priority INTEGER DEFAULT 50,
			is_global BOOLEAN DEFAULT 0,
			usage_count INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		t.Fatalf("无法创建skills表: %v", err)
	}
	
	// 创建project_skills表
	_, err = db.Exec(`
		CREATE TABLE project_skills (
			project_path TEXT NOT NULL,
			skill_id INTEGER NOT NULL,
			is_enabled BOOLEAN DEFAULT 1,
			enabled_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_used DATETIME,
			PRIMARY KEY (project_path, skill_id),
			FOREIGN KEY (skill_id) REFERENCES skills(id) ON DELETE CASCADE
		)
	`)
	if err != nil {
		t.Fatalf("无法创建project_skills表: %v", err)
	}
	
	// 测试1: 插入技能数据
	skillContent := map[string]interface{}{
		"trigger":  []string{"test", "测试"},
		"rules":    []string{"规则1", "规则2"},
		"examples": []string{"示例1", "示例2"},
	}
	contentJSON, _ := json.Marshal(skillContent)
	
	result, err := db.Exec(`
		INSERT INTO skills (name, description, content, category, priority, is_global)
		VALUES (?, ?, ?, ?, ?, ?)
	`, "测试技能", "测试技能描述", string(contentJSON), "test", 90, 1)
	if err != nil {
		t.Fatalf("无法插入技能数据: %v", err)
	}
	
	skillID, _ := result.LastInsertId()
	
	// 测试2: 查询技能数据
	var name, description, category string
	var priority int
	var isGlobal bool
	err = db.QueryRow(`
		SELECT name, description, category, priority, is_global
		FROM skills WHERE id = ?
	`, skillID).Scan(&name, &description, &category, &priority, &isGlobal)
	if err != nil {
		t.Fatalf("无法查询技能数据: %v", err)
	}
	
	if name != "测试技能" {
		t.Errorf("技能名称不匹配: 期望='测试技能', 实际='%s'", name)
	}
	if category != "test" {
		t.Errorf("技能分类不匹配: 期望='test', 实际='%s'", category)
	}
	if priority != 90 {
		t.Errorf("技能优先级不匹配: 期望=90, 实际=%d", priority)
	}
	if !isGlobal {
		t.Error("技能应为全局技能")
	}
	
	// 测试3: 关联项目技能
	projectPath := "/test/project/path"
	_, err = db.Exec(`
		INSERT INTO project_skills (project_path, skill_id, is_enabled)
		VALUES (?, ?, ?)
	`, projectPath, skillID, 1)
	if err != nil {
		t.Fatalf("无法关联项目技能: %v", err)
	}
	
	// 测试4: 查询项目启用的技能
	var enabled bool
	err = db.QueryRow(`
		SELECT ps.is_enabled
		FROM project_skills ps
		WHERE ps.project_path = ? AND ps.skill_id = ?
	`, projectPath, skillID).Scan(&enabled)
	if err != nil {
		t.Fatalf("无法查询项目技能状态: %v", err)
	}
	
	if !enabled {
		t.Error("项目技能应该启用")
	}
	
	// 测试5: 更新技能使用次数
	_, err = db.Exec(`
		UPDATE skills 
		SET usage_count = usage_count + 1
		WHERE id = ?
	`, skillID)
	if err != nil {
		t.Fatalf("无法更新技能使用次数: %v", err)
	}
	
	// 验证更新
	var usageCount int
	err = db.QueryRow(`SELECT usage_count FROM skills WHERE id = ?`, skillID).Scan(&usageCount)
	if err != nil {
		t.Fatalf("无法查询使用次数: %v", err)
	}
	
	if usageCount != 1 {
		t.Errorf("使用次数不匹配: 期望=1, 实际=%d", usageCount)
	}
	
	t.Log("Skills数据库测试通过")
}

// TestSkillContentFormat 测试技能内容格式
func TestSkillContentFormat(t *testing.T) {
	// 测试JSON格式的技能内容
	skillContent := map[string]interface{}{
		"trigger":  []string{"go", "test", "测试"},
		"rules": []string{
			"测试文件应以_test.go结尾",
			"测试函数名应以Test开头",
			"使用表格驱动测试",
		},
		"examples": []string{
			"func TestAdd(t *testing.T) {\n    // 测试代码\n}",
		},
	}
	
	// 序列化为JSON
	contentJSON, err := json.Marshal(skillContent)
	if err != nil {
		t.Fatalf("无法序列化技能内容: %v", err)
	}
	
	// 反序列化验证
	var decodedContent map[string]interface{}
	err = json.Unmarshal(contentJSON, &decodedContent)
	if err != nil {
		t.Fatalf("无法反序列化技能内容: %v", err)
	}
	
	// 验证结构
	if triggers, ok := decodedContent["trigger"].([]interface{}); ok {
		if len(triggers) != 3 {
			t.Errorf("触发词数量不匹配: 期望=3, 实际=%d", len(triggers))
		}
	} else {
		t.Error("触发词字段格式错误")
	}
	
	if rules, ok := decodedContent["rules"].([]interface{}); ok {
		if len(rules) != 3 {
			t.Errorf("规则数量不匹配: 期望=3, 实际=%d", len(rules))
		}
	} else {
		t.Error("规则字段格式错误")
	}
	
	t.Log("技能内容格式测试通过")
}

// TestSkillMatcherLogic 测试技能匹配逻辑
func TestSkillMatcherLogic(t *testing.T) {
	// 模拟技能数据
	skills := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name: "Go测试技能",
			content: `{
				"trigger": ["test", "测试", "go test"],
				"rules": ["规则1"],
				"examples": ["示例1"]
			}`,
			expected: true,
		},
		{
			name: "Git技能",
			content: `{
				"trigger": ["git", "commit", "提交"],
				"rules": ["规则2"],
				"examples": ["示例2"]
			}`,
			expected: false,
		},
	}
	
	// 测试查询
	userQuery := "如何编写Go测试？"
	queryLower := strings.ToLower(userQuery)
	
	for _, skill := range skills {
		var content map[string]interface{}
		err := json.Unmarshal([]byte(skill.content), &content)
		if err != nil {
			t.Errorf("无法解析技能内容: %v", err)
			continue
		}
		
		// 简单的匹配逻辑
		matched := false
		if triggers, ok := content["trigger"].([]interface{}); ok {
			for _, trigger := range triggers {
				if triggerStr, ok := trigger.(string); ok {
					if strings.Contains(queryLower, strings.ToLower(triggerStr)) {
						matched = true
						break
					}
				}
			}
		}
		
		if matched != skill.expected {
			t.Errorf("技能'%s'匹配结果不匹配: 期望=%v, 实际=%v", 
				skill.name, skill.expected, matched)
		}
	}
	
	t.Log("技能匹配逻辑测试通过")
}

// TestRealSkillsIntegration 测试真实Skills系统集成
func TestRealSkillsIntegration(t *testing.T) {
	// 跳过测试，如果数据库文件不存在
	dbPath := "/home/nanjj/.dscli/sqlite.db"
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Skip("Skills数据库文件不存在，跳过集成测试")
	}
	
	// 连接到真实数据库
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("无法连接到Skills数据库: %v", err)
	}
	defer db.Close()
	
	// 测试1: 验证skills表存在
	var tableName string
	err = db.QueryRow(`
		SELECT name FROM sqlite_master 
		WHERE type='table' AND name='skills'
	`).Scan(&tableName)
	if err != nil {
		t.Fatalf("skills表不存在: %v", err)
	}
	
	if tableName != "skills" {
		t.Errorf("表名不匹配: 期望='skills', 实际='%s'", tableName)
	}
	
	// 测试2: 验证有技能数据
	var skillCount int
	err = db.QueryRow(`SELECT COUNT(*) FROM skills`).Scan(&skillCount)
	if err != nil {
		t.Fatalf("无法查询技能数量: %v", err)
	}
	
	if skillCount == 0 {
		t.Log("警告: skills表中没有数据")
	} else {
		t.Logf("Skills数据库中有 %d 个技能", skillCount)
		
		// 测试3: 查询技能详情
		rows, err := db.Query(`
			SELECT name, category, priority, is_global, usage_count
			FROM skills 
			ORDER BY priority DESC 
			LIMIT 3
		`)
		if err != nil {
			t.Fatalf("无法查询技能详情: %v", err)
		}
		defer rows.Close()
		
		t.Log("前3个技能:")
		for rows.Next() {
			var name, category string
			var priority int
			var isGlobal bool
			var usageCount int
			
			err := rows.Scan(&name, &category, &priority, &isGlobal, &usageCount)
			if err != nil {
				t.Errorf("无法扫描技能数据: %v", err)
				continue
			}
			
			t.Logf("  - %s [%s] (优先级: %d, 全局: %v, 使用: %d次)", 
				name, category, priority, isGlobal, usageCount)
		}
	}
	
	// 测试4: 验证project_skills表
	err = db.QueryRow(`
		SELECT name FROM sqlite_master 
		WHERE type='table' AND name='project_skills'
	`).Scan(&tableName)
	if err != nil {
		t.Fatalf("project_skills表不存在: %v", err)
	}
	
	t.Log("Skills系统集成测试通过")
}

// TestSkillPrioritySystem 测试技能优先级系统
func TestSkillPrioritySystem(t *testing.T) {
	// 测试优先级排序逻辑
	skills := []struct {
		name     string
		priority int
	}{
		{"高优先级技能", 90},
		{"中优先级技能", 50},
		{"低优先级技能", 10},
		{"默认优先级技能", 50},
	}
	
	// 模拟按优先级排序
	// 在实际系统中，这应该由数据库ORDER BY priority DESC完成
	highPrioritySkills := 0
	mediumPrioritySkills := 0
	lowPrioritySkills := 0
	
	for _, skill := range skills {
		if skill.priority >= 80 {
			highPrioritySkills++
		} else if skill.priority >= 30 {
			mediumPrioritySkills++
		} else {
			lowPrioritySkills++
		}
	}
	
	if highPrioritySkills != 1 {
		t.Errorf("高优先级技能数量不匹配: 期望=1, 实际=%d", highPrioritySkills)
	}
	if mediumPrioritySkills != 2 {
		t.Errorf("中优先级技能数量不匹配: 期望=2, 实际=%d", mediumPrioritySkills)
	}
	if lowPrioritySkills != 1 {
		t.Errorf("低优先级技能数量不匹配: 期望=1, 实际=%d", lowPrioritySkills)
	}
	
	t.Log("技能优先级系统测试通过")
}

// BenchmarkSkillMatching 性能测试：技能匹配
func BenchmarkSkillMatching(b *testing.B) {
	// 准备测试数据
	skills := make([]map[string]interface{}, 100)
	for i := 0; i < 100; i++ {
		skills[i] = map[string]interface{}{
			"trigger": []string{fmt.Sprintf("keyword%d", i), "test", "example"},
			"rules":   []string{fmt.Sprintf("规则%d", i)},
		}
	}
	
	userQuery := "这是一个测试查询，包含test关键字"
	queryLower := strings.ToLower(userQuery)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matchedSkills := 0
		for _, skill := range skills {
			if triggers, ok := skill["trigger"].([]string); ok {
				for _, trigger := range triggers {
					if strings.Contains(queryLower, strings.ToLower(trigger)) {
						matchedSkills++
						break
					}
				}
			}
		}
		_ = matchedSkills // 避免编译器优化
	}
}

// ExampleSkillUsage 示例：如何使用Skills系统
func Example_skillUsage() { {
	// 这个示例展示了Skills系统的基本用法
	
	// 1. 连接到数据库
	dbPath := "/home/nanjj/.dscli/sqlite.db"
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		fmt.Printf("连接数据库失败: %v\n", err)
		return
	}
	defer db.Close()
	
	// 2. 查询技能
	rows, err := db.Query(`
		SELECT name, description, category, priority
		FROM skills 
		WHERE is_global = 1 
		ORDER BY priority DESC
	`)
	if err != nil {
		fmt.Printf("查询技能失败: %v\n", err)
		return
	}
	defer rows.Close()
	
	// 3. 显示技能
	fmt.Println("全局技能列表:")
	for rows.Next() {
		var name, description, category string
		var priority int
		err := rows.Scan(&name, &description, &category, &priority)
		if err != nil {
			fmt.Printf("读取技能失败: %v\n", err)
			continue
		}
		fmt.Printf("  - %s (%s): %s (优先级: %d)\n", 
			name, category, description, priority)
	}
	
	// 输出:
	// 全局技能列表:
	//   - Go测试规范 (go): Go语言测试最佳实践 (优先级: 90)
	//   - Git提交规范 (git): Git提交信息编写规范 (优先级: 85)
	//   - Markdown到Org转换 (markdown): Markdown到Org模式转换规则 (优先级: 80)
}
}
