package skills

import (
	"os"
	"path/filepath"
	"testing"
)

// TestNewSkillStoreLoadsFromDir 测试从目录加载技能。
func TestNewSkillStoreLoadsFromDir(t *testing.T) {
	tmpDir := t.TempDir()

	// 写入一个测试技能
	skillDir := filepath.Join(tmpDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// 注意：keywords 在 YAML frontmatter 中必须是列表格式
	skillMD := `---
name: test-skill
description: 测试技能
keywords: [test, skill]
---
# 测试技能

这是一个测试技能的内容。
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := NewSkillStore(tmpDir, "local")
	if err != nil {
		t.Fatalf("NewSkillStore 失败: %v", err)
	}

	if store.Skills == nil {
		t.Fatal("Skills map 为空")
	}

	skill, ok := store.Skills["test-skill"]
	if !ok {
		t.Fatalf("技能 test-skill 未找到, 可用: %v", store.List())
	}

	if skill.Name != "test-skill" {
		t.Errorf("Name = %q, want %q", skill.Name, "test-skill")
	}
	if skill.Description != "测试技能" {
		t.Errorf("Description = %q, want %q", skill.Description, "测试技能")
	}
	if skill.Source != "local" {
		t.Errorf("Source = %q, want %q", skill.Source, "local")
	}
}

// TestStoreList 测试列出所有技能名称。
func TestStoreList(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建两个技能
	for _, name := range []string{"alpha-skill", "beta-skill"} {
		skillDir := filepath.Join(tmpDir, name)
		os.MkdirAll(skillDir, 0o755)
		os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(
			"---\nname: "+name+"\ndescription: 测试\n---\n# 内容\n",
		), 0o644)
	}

	store, err := NewSkillStore(tmpDir, "local")
	if err != nil {
		t.Fatal(err)
	}

	names := store.List()
	if len(names) != 2 {
		t.Fatalf("期望 2 个技能，得到 %d: %v", len(names), names)
	}
	if names[0] != "alpha-skill" || names[1] != "beta-skill" {
		t.Errorf("技能列表未排序: %v", names)
	}
}

// TestStoreSaveAndReload 测试保存和重新加载保持数据一致。
func TestStoreSaveAndReload(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建技能
	skillDir := filepath.Join(tmpDir, "test-skill")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(
		"---\nname: test-skill\ndescription: 测试\n---\n# 内容\n",
	), 0o644)

	store1, err := NewSkillStore(tmpDir, "local")
	if err != nil {
		t.Fatal(err)
	}

	// 重新加载
	store2, err := NewSkillStore(tmpDir, "local")
	if err != nil {
		t.Fatal(err)
	}

	if len(store2.Skills) != 1 {
		t.Fatalf("重新加载后技能数不对: %d", len(store2.Skills))
	}

	s1 := store1.Skills["test-skill"]
	s2 := store2.Skills["test-skill"]

	if s1.Name != s2.Name || s1.Description != s2.Description {
		t.Error("保存/重新加载后数据不一致")
	}
}

// TestStoreQuery 测试关键词查询。
func TestStoreQuery(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建技能（keywords 使用 YAML 列表格式）
	skillDir := filepath.Join(tmpDir, "go-fix")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(
		"---\nname: go-fix\ndescription: Go 代码现代化助手\nkeywords: [go, fix, modernize]\n---\n# Go Fix\n",
	), 0o644)

	store, err := NewSkillStore(tmpDir, "local")
	if err != nil {
		t.Fatal(err)
	}

	// 精确关键词匹配
	matched := store.Query("go")
	if _, ok := matched["go-fix"]; !ok {
		t.Error("关键词 'go' 应匹配 go-fix")
	}

	// 大小写不敏感的子串匹配
	matched = store.Query("MODERNIZE")
	if _, ok := matched["go-fix"]; !ok {
		t.Error("关键词 'MODERNIZE' (小写后) 应匹配 go-fix")
	}

	// 不匹配的关键词
	matched = store.Query("python")
	if len(matched) != 0 {
		t.Errorf("关键词 'python' 不应匹配结果: %v", matched)
	}
}

// TestStoreLoadDetectsNewFiles 测试 Store.Load 检测新文件。
// 模拟 skill add 后自动检测新技能的场景。
func TestStoreLoadDetectsNewFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// 先创建一个技能
	dir1 := filepath.Join(tmpDir, "skill-a")
	os.MkdirAll(dir1, 0o755)
	os.WriteFile(filepath.Join(dir1, "SKILL.md"), []byte(
		"---\nname: skill-a\ndescription: A\n---\n# A\n",
	), 0o644)

	store, err := NewSkillStore(tmpDir, "local")
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Skills) != 1 {
		t.Fatalf("期望 1 个技能，得到 %d", len(store.Skills))
	}

	// 再创建一个新技能（模拟 skill add 后）
	dir2 := filepath.Join(tmpDir, "skill-b")
	os.MkdirAll(dir2, 0o755)
	os.WriteFile(filepath.Join(dir2, "SKILL.md"), []byte(
		"---\nname: skill-b\ndescription: B\n---\n# B\n",
	), 0o644)

	// 重新加载应检测到新文件
	if err := store.Load(); err != nil {
		t.Fatalf("Load 失败: %v", err)
	}
	if len(store.Skills) != 2 {
		t.Fatalf("期望 2 个技能，得到 %d，技能: %v", len(store.Skills), store.List())
	}
}
