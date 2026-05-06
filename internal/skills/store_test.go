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

	// 精确关键词匹配（名称分词 + 显式关键词都可命中）
	matched := store.Query("go")
	if _, ok := matched["go-fix"]; !ok {
		t.Error("关键词 'go' 应匹配 go-fix")
	}

	// 大小写不敏感 + 双向子串匹配
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

// TestStoreQueryByName 测试通过名称搜索技能。
// skill_search("go-fix") 应能直接找到 go-fix 技能。
func TestStoreQueryByName(t *testing.T) {
	tmpDir := t.TempDir()

	skillDir := filepath.Join(tmpDir, "go-fix")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(
		"---\nname: go-fix\ndescription: Go code modernizer\nkeywords: [go, fix, modernize]\n---\n# Go Fix\n",
	), 0o644)

	store, err := NewSkillStore(tmpDir, "local")
	if err != nil {
		t.Fatal(err)
	}

	// 通过完整名称搜索
	matched := store.Query("go-fix")
	if _, ok := matched["go-fix"]; !ok {
		t.Error("名称 'go-fix' 应匹配自身")
	}

	// 通过名称分词搜索
	matched = store.Query("go")
	if _, ok := matched["go-fix"]; !ok {
		t.Error("名称分词 'go' 应匹配 go-fix")
	}

	matched = store.Query("fix")
	if _, ok := matched["go-fix"]; !ok {
		t.Error("名称分词 'fix' 应匹配 go-fix")
	}
}

// TestStoreQueryByNameWithHyphens 测试带连字符的名称搜索。
// skill_search("use-modern-go") 应精确命中。
func TestStoreQueryByNameWithHyphens(t *testing.T) {
	tmpDir := t.TempDir()

	// 模拟 use-modern-go: YAML 无 keywords，全靠自动提取
	skillDir := filepath.Join(tmpDir, "use-modern-go")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(
		"---\nname: use-modern-go\ndescription: Apply modern Go syntax guidelines based on project's Go version.\n---\n# Use Modern Go\n",
	), 0o644)

	store, err := NewSkillStore(tmpDir, "local")
	if err != nil {
		t.Fatal(err)
	}

	// 完整名称匹配
	matched := store.Query("use-modern-go")
	if _, ok := matched["use-modern-go"]; !ok {
		t.Errorf("完整名称 'use-modern-go' 应匹配自身, got: %v", matched)
	}

	// 部分名称 token 匹配
	matched = store.Query("modern")
	if _, ok := matched["use-modern-go"]; !ok {
		t.Error("名称分词 'modern' 应匹配 use-modern-go")
	}

	matched = store.Query("go")
	if _, ok := matched["use-modern-go"]; !ok {
		t.Error("名称分词 'go' 应匹配 use-modern-go")
	}

	// 描述分词也会命中（自动提取的 keywords，≥3 字符）
	matched = store.Query("syntax")
	if _, ok := matched["use-modern-go"]; !ok {
		t.Error("描述词 'syntax' 应匹配 use-modern-go")
	}

	// 无关关键词不应匹配（特别是不能因为 "on" 而误匹配 "python"）
	matched = store.Query("python")
	if len(matched) != 0 {
		t.Errorf("'python' 不应匹配任何结果: %v", matched)
	}
}

// TestStoreQueryScored 测试评分排序。
func TestStoreQueryScored(t *testing.T) {
	tmpDir := t.TempDir()

	// go-fix: 有显式 keywords
	os.MkdirAll(filepath.Join(tmpDir, "go-fix"), 0o755)
	os.WriteFile(filepath.Join(tmpDir, "go-fix", "SKILL.md"), []byte(
		"---\nname: go-fix\ndescription: Go code modernizer\nkeywords: [go, fix, modernize]\n---\n# Go Fix\n",
	), 0o644)

	// use-modern-go: 无 keywords，靠自动提取
	os.MkdirAll(filepath.Join(tmpDir, "use-modern-go"), 0o755)
	os.WriteFile(filepath.Join(tmpDir, "use-modern-go", "SKILL.md"), []byte(
		"---\nname: use-modern-go\ndescription: Apply modern Go syntax guidelines.\n---\n# Use Modern Go\n",
	), 0o644)

	store, err := NewSkillStore(tmpDir, "local")
	if err != nil {
		t.Fatal(err)
	}

	// 搜索 "go modern": 两个技能都应匹配，按分数排序
	scored := store.QueryScored("go modern")
	if len(scored) != 2 {
		t.Fatalf("期望 2 个结果，得到 %d: %v", len(scored), scored)
	}

	// 第一个应具有更高分数（可能有名称匹配加成）
	if scored[0].Score < scored[1].Score {
		t.Errorf("结果未按分数降序排列: %+v", scored)
	}

	// 验证所有结果都包含搜索词
	for _, s := range scored {
		if s.Score <= 0 {
			t.Errorf("结果 %s 分数应为正数: %d", s.Skill.Name, s.Score)
		}
	}
}

// TestStoreQueryEmpty 测试空查询。
func TestStoreQueryEmpty(t *testing.T) {
	tmpDir := t.TempDir()

	os.MkdirAll(filepath.Join(tmpDir, "test-skill"), 0o755)
	os.WriteFile(filepath.Join(tmpDir, "test-skill", "SKILL.md"), []byte(
		"---\nname: test-skill\ndescription: test\n---\n# Test\n",
	), 0o644)

	store, err := NewSkillStore(tmpDir, "local")
	if err != nil {
		t.Fatal(err)
	}

	matched := store.Query("")
	if len(matched) != 0 {
		t.Errorf("空查询不应返回结果: %v", matched)
	}

	scored := store.QueryScored("")
	if scored != nil {
		t.Errorf("空查询 QueryScored 应返回 nil: %v", scored)
	}
}

// TestMatchToken 测试 token 匹配逻辑。
func TestMatchToken(t *testing.T) {
	tests := []struct {
		query, idx string
		want       bool
	}{
		{"go", "go", true},
		{"go", "go-fix", true},        // "go-fix" 包含 "go"
		{"modernize", "modern", true},  // query 包含 idx
		{"modern", "modernize", true},  // idx 包含 query
		{"python", "go", false},
		{"", "go", false},
		{"go", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		got := matchToken(tt.query, tt.idx)
		if got != tt.want {
			t.Errorf("matchToken(%q, %q) = %v, want %v", tt.query, tt.idx, got, tt.want)
		}
	}
}

// TestTokenizeQuery 测试查询 token 化。
func TestTokenizeQuery(t *testing.T) {
	tokens := tokenizeQuery("modern go")
	if len(tokens) != 2 {
		t.Fatalf("tokenizeQuery('modern go') = %v, want 2 tokens", tokens)
	}
	if tokens[0] != "modern" || tokens[1] != "go" {
		t.Errorf("unexpected tokens: %v", tokens)
	}

	// 停用词不应被过滤（查询中）
	tokens = tokenizeQuery("use modern go")
	if len(tokens) != 3 {
		t.Errorf("tokenizeQuery('use modern go') 不应过滤 'use': %v", tokens)
	}
}

// TestExtractKeywordsFromName 测试从名称自动提取关键词。
func TestExtractKeywordsFromName(t *testing.T) {
	kws := extractKeywords("use-modern-go", "Apply modern Go syntax.")
	if len(kws) == 0 {
		t.Fatal("extractKeywords 应返回非空关键词")
	}

	// 应包含名称分词
	found := make(map[string]bool)
	for _, kw := range kws {
		found[kw] = true
	}
	if !found["modern"] {
		t.Errorf("关键词应包含 'modern': %v", kws)
	}
	if !found["go"] {
		t.Errorf("关键词应包含 'go': %v", kws)
	}
	// "use" 是停用词，不应出现在关键词中
	if found["use"] {
		t.Errorf("关键词不应包含停用词 'use': %v", kws)
	}
}

// TestExtractKeywordsExplicit 测试显式关键词声明的技能。
func TestExtractKeywordsExplicit(t *testing.T) {
	kws := extractKeywords("go-fix", "Go 代码现代化助手。关键词：go, fix, modernize")
	if len(kws) != 3 {
		t.Fatalf("期望 3 个关键词，得到 %d: %v", len(kws), kws)
	}
	expected := []string{"go", "fix", "modernize"}
	for i, kw := range kws {
		if kw != expected[i] {
			t.Errorf("kws[%d] = %q, want %q", i, kw, expected[i])
		}
	}
}
