package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	"gitcode.com/dscli/dscli/internal/config"
	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/outfmt"
	"github.com/goccy/go-yaml"
)

var (
	globalStore *Store
	globalErr   error
	globalOnce  sync.Once
	localStore  *Store
	localErr    error
	localOnce   sync.Once
)

// ResetLocalStore clears the cached local store, forcing re-initialization on next LocalStore() call.
// This is primarily for tests that switch between different ProjectRoot directories.
func ResetLocalStore() {
	localStore = nil
	localErr = nil
	localOnce = sync.Once{}
}

type Store struct {
	dir      string
	source   string              // "local" 或 "global"
	Skills   map[string]Skill    `yaml:"skills,omitzero"`
	Keywords map[string][]string `yaml:"keywords,omitzero"`
}

// ScoredSkill 带匹配分数的技能，用于搜索结果排序。
type ScoredSkill struct {
	Skill Skill
	Score int
}

func LocalStore() (*Store, error) {
	localOnce.Do(func() {
		// Primary: <project>/.dscli/skills/ (dscli-native location)
		dscliDir := filepath.Join(context.ProjectRoot, ".dscli", "skills")
		localErr = os.MkdirAll(dscliDir, 0o755)
		if localErr != nil {
			return
		}
		localStore, localErr = NewSkillStore(dscliDir, "local")

		// Secondary: <project>/.agents/skills/ (cross-client interoperability)
		// Skills from .agents/skills/ are loaded and merged with lower priority:
		// same-name skills in .dscli/skills/ take precedence.
		if localErr == nil {
			agentsDir := filepath.Join(context.ProjectRoot, ".agents", "skills")
			if info, statErr := os.Stat(agentsDir); statErr == nil && info.IsDir() {
				mergeCrossClientSkills(localStore, agentsDir)
			}
		}
	})
	return localStore, localErr
}

func GlobalStore() (*Store, error) {
	globalOnce.Do(func() {
		dir := filepath.Join(config.ConfigDir, "skills")
		globalErr = os.MkdirAll(dir, 0o755)
		if globalErr != nil {
			return
		}
		globalStore, globalErr = NewSkillStore(dir, "global")

		// Secondary: ~/.agents/skills/ (cross-client user-level interoperability)
		// Skills installed by other agents (Claude Code, Codex, etc.) at the
		// user level are merged with lower priority.
		if globalErr == nil {
			home, homeErr := os.UserHomeDir()
			if homeErr == nil {
				agentsDir := filepath.Join(home, ".agents", "skills")
				if info, statErr := os.Stat(agentsDir); statErr == nil && info.IsDir() {
					mergeCrossClientSkills(globalStore, agentsDir)
				}
			}
		}
	})
	return globalStore, globalErr
}

func NewSkillStore(dir, source string) (*Store, error) {
	store := &Store{
		dir:    dir,
		source: source,
	}
	if err := store.Load(); err != nil {
		// 返回空存储而非错误，确保调用方始终获得有效对象
		store.Skills = map[string]Skill{}
		store.Keywords = map[string][]string{}
		return store, nil
	}
	return store, nil
}

// mergeCrossClientSkills loads skills from a secondary directory (e.g. .agents/skills/)
// and merges them into the primary store. Skills already present in the primary store
// are NOT overwritten — the primary store (dscli-native) takes precedence.
// This enables cross-client interoperability: skills installed by other agents
// (Claude Code, Codex, etc.) in .agents/skills/ become automatically available.
func mergeCrossClientSkills(store *Store, agentsDir string) {
	crossSkills := LoadSkills(agentsDir)
	if len(crossSkills) == 0 {
		return
	}

	merged := 0
	for name, skill := range crossSkills {
		// Don't overwrite skills already in the primary store
		if _, exists := store.Skills[name]; exists {
			continue
		}
		skill.Source = store.source
		store.Skills[name] = skill
		indexSkillKeywords(store.Keywords, name, skill.Keywords)
		merged++
	}
	if merged > 0 {
		fmt.Fprintf(os.Stderr, "Loaded %d skill(s) from %s (cross-client)\n", merged, agentsDir)
	}
}

// indexSkillKeywords 将技能关键词和名称索引到倒排表中。
// 除了 skill.Keywords 中的显式关键词外，还会将技能名称（全名和分词后的 token）
// 加入索引（过滤停用词），使得 skill_search("use-modern-go") 可以精确命中。
func indexSkillKeywords(kws map[string][]string, name string, keywords []string) {
	addKW := func(kw string) {
		names := kws[kw]
		if !slices.Contains(names, name) {
			kws[kw] = append(names, name)
		}
	}
	for _, kw := range keywords {
		addKW(kw)
	}
	// 名称全名作为关键词（退化为精确查找）
	addKW(strings.ToLower(name))
	// 名称分词作为关键词（过滤停用词，与 extractKeywordsFromNameAndDesc 一致）
	for _, token := range tokenizeName(name) {
		if !isStopword(token) {
			addKW(token)
		}
	}
}

func (store *Store) Load() (err error) {
	path := filepath.Join(store.dir, "skills.yaml")
	yamlInfo, yamlErr := os.Stat(path)

	// 检查缓存是否需要更新：比较 skills.yaml 和 SKILL.md 文件的修改时间
	needReload := yamlErr != nil
	if yamlErr == nil {
		skillFiles := SkillFiles(store.dir)
		for _, sf := range skillFiles {
			info, statErr := os.Stat(sf)
			if statErr != nil {
				continue
			}
			if info.ModTime().After(yamlInfo.ModTime()) {
				needReload = true
				break
			}
		}
	}

	// 如果缓存有效，直接加载
	if !needReload {
		var data []byte
		data, err = os.ReadFile(path)
		if err != nil {
			return err
		}
		err = yaml.Unmarshal(data, store)
		if err != nil {
			return err
		}
		// 验证缓存条目：如果技能目录已被手动删除，则缓存失效，触发重建
		for _, skill := range store.Skills {
			if _, statErr := os.Stat(skill.Path); os.IsNotExist(statErr) {
				needReload = true
				break
			}
		}
		if !needReload {
			// 注入 Source（缓存中不保存 Source）
			for name, skill := range store.Skills {
				skill.Source = store.source
				store.Skills[name] = skill
			}
			return err
		}
		// 有僵尸条目，继续执行下面的重建逻辑
	}

	// 缓存无效或不存在，重新从 SKILL.md 文件加载
	// 先读取旧缓存中的 auto_inject 设置，以便刷新后保留用户偏好
	var oldSkills map[string]Skill
	if yamlErr == nil {
		// 旧缓存存在，尝试读取
		var oldStore Store
		if data, readErr := os.ReadFile(path); readErr == nil {
			if unmarshalErr := yaml.Unmarshal(data, &oldStore); unmarshalErr == nil {
				oldSkills = oldStore.Skills
			}
		}
	}

	err = nil
	skills := LoadSkills(store.dir)
	if len(skills) == 0 {
		err = fmt.Errorf("no skill loaded")
		return err
	}

	kws := map[string][]string{}
	for name, skill := range skills {
		skill.Source = store.source // 注入来源

		// 保留旧的 auto_inject 设置（用户偏好，非 skill 定义）
		if old, ok := oldSkills[name]; ok {
			skill.AutoInject = old.AutoInject
		}

		indexSkillKeywords(kws, name, skill.Keywords)
		skills[name] = skill
	}
	store.Skills = skills
	store.Keywords = kws

	// 保存到yaml文件，以便下次直接加载
	if err := store.Save(); err != nil {
		// 保存失败不影响使用，只是下次需要重新加载
		fmt.Printf("Warning: failed to save skills.yaml: %v\n", err)
	}

	return err
}

func (store *Store) Save() error {
	path := filepath.Join(store.dir, "skills.yaml")
	data, err := yaml.Marshal(store)
	if err != nil {
		return fmt.Errorf("failed to marshal skills: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("failed to write skills.yaml: %w", err)
	}

	return nil
}

// Query 使用 token 匹配搜索技能。
// 保留向后兼容的 map 返回类型。
func (store *Store) Query(query string) (matched map[string]Skill) {
	scored := store.QueryScored(query)
	matched = make(map[string]Skill, len(scored))
	for _, s := range scored {
		matched[s.Skill.Name] = s.Skill
	}
	return matched
}

// QueryScored 使用 token 匹配搜索技能，返回按分数降序排列的结果。
//
// 评分策略（按优先级）：
//  1. 精确名称匹配（大小写不敏感）：+100
//  2. 查询 token 命中名称整体或名称分词：每个 +10
//  3. 查询 token 命中显式关键词（skill.Keywords）：每个 +5
//  4. 查询 token 命中描述词（回退）：每个 +1
//
// 匹配采用双向子串包含：token 包含 keyword 或 keyword 包含 token。
// 所有匹配均为大小写不敏感。
func (store *Store) QueryScored(query string) []ScoredSkill {
	if query == "" {
		return nil
	}

	var results []ScoredSkill
	for name, skill := range store.Skills {
		if s := skill.Score(query); s > 0 {
			results = append(results, ScoredSkill{Skill: store.Skills[name], Score: s})
		}
	}

	// 按分数降序，分数相同时按名称字母序
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Skill.Name < results[j].Skill.Name
	})

	return results
}

// matchToken 检查 query token 是否匹配索引 token。
// 双向子串包含：query 包含 idx 或 idx 包含 query。
// 空字符串不做匹配。匹配为大小写不敏感。
func matchToken(queryToken, idxToken string) bool {
	if queryToken == "" || idxToken == "" {
		return false
	}
	return strings.Contains(strings.ToLower(queryToken), strings.ToLower(idxToken)) ||
		strings.Contains(strings.ToLower(idxToken), strings.ToLower(queryToken))
}

// Use looks up a skill by name within this specific store.
// It does not check global or built-in skills.
// For full resolution including fallback, use the package-level Use function.
func (store *Store) Use(name string) (content string, err error) {
	skill, ok := store.Skills[name]
	if ok {
		content = skill.FormatFull()
		return content, err
	}

	err = fmt.Errorf("skill %s not exists", name)
	return content, err
}

// Query 在本地和全局 store 中搜索技能，返回按分数排序的格式化结果。
func Query(q string) (string, error) {
	localStore, err := LocalStore()
	if err != nil {
		return "", fmt.Errorf("failed to load local store: %w", err)
	}

	globalStore, err := GlobalStore()
	if err != nil {
		return "", fmt.Errorf("failed to load global store: %w", err)
	}

	localScored := localStore.QueryScored(q)
	globalScored := globalStore.QueryScored(q)

	// 合并：local 优先（同分时 local 排前面）
	merged := mergeScored(localScored, globalScored)

	// Fallback: query built-in skills not shadowed by local/global
	seen := make(map[string]bool, len(merged))
	for _, s := range merged {
		seen[s.Skill.Name] = true
	}
	for name, skill := range builtinSkills() {
		if seen[name] {
			continue
		}
		if s := skill.Score(q); s > 0 {
			merged = append(merged, ScoredSkill{Skill: skill, Score: s})
		}
	}

	// Re-sort after adding built-in results
	sort.Slice(merged, func(i, j int) bool {
		if merged[i].Score != merged[j].Score {
			return merged[i].Score > merged[j].Score
		}
		return merged[i].Skill.Name < merged[j].Skill.Name
	})

	if len(merged) == 0 {
		return "", fmt.Errorf("no skills found for query: %s", q)
	}

	var builder strings.Builder
	for _, s := range merged {
		builder.WriteString("---skill name: ")
		builder.WriteString(s.Skill.Name)
		builder.WriteString("---\n")
		builder.WriteString(s.Skill.Summary())
	}
	return builder.String(), nil
}

// mergeScored 合并 local 和 global 评分结果。
// local 优先：同名 skill 只保留 local；其他按分数降序排列。
func mergeScored(local, global []ScoredSkill) []ScoredSkill {
	seen := make(map[string]bool, len(local)+len(global))
	var merged []ScoredSkill

	for _, s := range local {
		seen[s.Skill.Name] = true
		merged = append(merged, s)
	}
	for _, s := range global {
		if seen[s.Skill.Name] {
			continue
		}
		seen[s.Skill.Name] = true
		merged = append(merged, s)
	}

	sort.Slice(merged, func(i, j int) bool {
		if merged[i].Score != merged[j].Score {
			return merged[i].Score > merged[j].Score
		}
		return merged[i].Skill.Name < merged[j].Skill.Name
	})
	return merged
}

func Use(name string) (content string, err error) {
	local, err := LocalStore()
	if err != nil {
		return "", fmt.Errorf("failed to load local store: %w", err)
	}

	content, err = local.Use(name)
	if err == nil {
		return content, nil
	}

	global, err := GlobalStore()
	if err != nil {
		return "", fmt.Errorf("failed to load global store: %w", err)
	}

	content, err = global.Use(name)
	if err == nil {
		return content, nil
	}
	// Fallback to built-in skills
	if skill, ok := builtinSkills()[name]; ok {
		return skill.FormatFull(), nil
	}

	return "", fmt.Errorf("skill %s not found in local, global, or built-in stores", name)
}

// ResolveSkillDir resolves a skill name to its directory path by looking up
// local and global stores. Returns empty string if the skill is not found.
// The built-in dscli skill (virtual, no directory) is NOT resolved by this function.
func ResolveSkillDir(name string) string {
	localStore, err := LocalStore()
	if err == nil {
		if skill, ok := localStore.Skills[name]; ok && skill.Path != "(built-in)" {
			return skill.Path
		}
	}
	globalStore, err := GlobalStore()
	if err == nil {
		if skill, ok := globalStore.Skills[name]; ok && skill.Path != "(built-in)" {
			return skill.Path
		}
	}
	return ""
}

// List 返回存储中的所有技能名称
func (store *Store) List() []string {
	if store == nil || store.Skills == nil {
		return []string{}
	}

	names := make([]string, 0, len(store.Skills))
	for name := range store.Skills {
		names = append(names, name)
	}

	// 按名称排序
	sort.Strings(names)
	return names
}

// ListAll 返回所有技能（本地、全局和内置）的列表。
// 优先级：local > global > built-in（同名时只保留最高优先级的）。
func ListAll() ([]SkillInfo, error) {
	localStore, err := LocalStore()
	if err != nil {
		return nil, fmt.Errorf("failed to load local store: %w", err)
	}

	globalStore, err := GlobalStore()
	if err != nil {
		return nil, fmt.Errorf("failed to load global store: %w", err)
	}

	// 收集所有技能信息
	skillInfos := make([]SkillInfo, 0)

	// 添加本地技能
	for _, name := range localStore.List() {
		skill := localStore.Skills[name]
		// Skip built-in skill sentinel (should never be in a store, but guard defensively)
		if skill.Source == "built-in" || skill.Path == "(built-in)" {
			continue
		}
		skillInfos = append(skillInfos, SkillInfo{
			Name:       name,
			Scope:      "local",
			AutoInject: skill.AutoInject,
		})
	}

	// 添加全局技能（排除与本地技能同名的）
	for _, name := range globalStore.List() {
		// 检查是否已有同名的本地技能
		hasLocal := false
		for _, info := range skillInfos {
			if info.Name == name {
				hasLocal = true
				break
			}
		}

		if !hasLocal {
			skill := globalStore.Skills[name]
			// Skip built-in skill sentinel (should never be in a store, but guard defensively)
			if skill.Source == "built-in" || skill.Path == "(built-in)" {
				continue
			}
			skillInfos = append(skillInfos, SkillInfo{
				Name:       name,
				Scope:      "global",
				AutoInject: skill.AutoInject,
			})
		}
	}

	// 添加内置技能（排除已被 local/global 覆盖的）
	// 添加内置技能（排除已被 local/global 覆盖的）
	seen := make(map[string]bool, len(skillInfos))
	for _, info := range skillInfos {
		seen[info.Name] = true
	}
	for name, skill := range builtinSkills() {
		if seen[name] {
			continue
		}
		skillInfos = append(skillInfos, SkillInfo{
			Name:       name,
			Scope:      "built-in",
			AutoInject: skill.AutoInject,
		})
	}
	// 按名称排序
	sort.Slice(skillInfos, func(i, j int) bool {
		return skillInfos[i].Name < skillInfos[j].Name
	})

	return skillInfos, nil
}

// SkillInfo 包含技能的基本信息和作用域
type SkillInfo struct {
	Name       string `json:"name"`
	Scope      string `json:"scope"`       // "local" 或 "global"
	AutoInject bool   `json:"auto_inject"` // 是否自动注入到对话上下文
}

// SetAutoInject 设置指定技能的 auto_inject 属性并保存。
func (store *Store) SetAutoInject(name string, autoInject bool) error {
	skill, ok := store.Skills[name]
	if !ok {
		return fmt.Errorf("skill %q not found in %s store", name, store.source)
	}
	skill.AutoInject = autoInject
	store.Skills[name] = skill
	return store.Save()
}

// SetAutoInject 设置技能的 auto_inject 属性。
// 优先修改本地 store；若指定了 global 则修改全局 store。
func SetAutoInject(name string, autoInject, global bool) error {
	if global {
		globalStore, err := GlobalStore()
		if err != nil {
			return fmt.Errorf("failed to load global store: %w", err)
		}
		return globalStore.SetAutoInject(name, autoInject)
	}

	localStore, err := LocalStore()
	if err != nil {
		return fmt.Errorf("failed to load local store: %w", err)
	}
	return localStore.SetAutoInject(name, autoInject)
}

func HandleSkillCreate(ctx context.Context, name, description, content, keywordsStr string, autoInject bool) (result, warning string, err error) {
	// Validate name against spec rules
	errs := validateName(name)
	if len(errs) > 0 {
		err = fmt.Errorf("invalid skill name %q: %s", name, strings.Join(errs, "; "))
		return result, warning, err
	}

	// Parse keywords from comma-separated string
	var keywords []string
	if keywordsStr != "" {
		for kw := range strings.SplitSeq(keywordsStr, ",") {
			kw = strings.TrimSpace(kw)
			if kw != "" {
				keywords = append(keywords, kw)
			}
		}
	}

	// Build skill struct
	skill := Skill{
		Name:        name,
		Description: description,
		Content:     content,
		Keywords:    keywords,
		AutoInject:  autoInject,
	}

	// Generate SKILL.md content with frontmatter
	skillMD, err := FormatSkillMD(&skill)
	if err != nil {
		err = fmt.Errorf("failed to format SKILL.md: %w", err)
		return result, warning, err
	}

	// Create local skill directory: .dscli/skills/<name>/
	localDir := filepath.Join(context.ProjectRoot, ".dscli", "skills", name)
	if err = os.MkdirAll(localDir, 0o755); err != nil {
		err = fmt.Errorf("failed to create skill directory: %w", err)
		return result, warning, err
	}

	// Write SKILL.md
	skillFile := filepath.Join(localDir, "SKILL.md")
	if err = os.WriteFile(skillFile, []byte(skillMD), 0o644); err != nil {
		err = fmt.Errorf("failed to write SKILL.md: %w", err)
		return result, warning, err
	}

	// Register in local store so it's immediately usable via skill_by_name / skill_search
	localStore, err := LocalStore()
	if err != nil {
		// Non-fatal: skill file is on disk, will be picked up on next load
		warning = fmt.Sprintf("Warning: could not update local store cache: %v", err)
		outfmt.Println(warning)
		result = fmt.Sprintf("Skill %q created at %s (store cache update skipped).", name, localDir)
		err = nil
		return result, warning, err
	}

	// Parse the newly created SKILL.md to get the full Skill (with resources, etc.)
	var parsedSkill Skill
	if err = ParseSkill(skillFile, &parsedSkill); err != nil {
		err = fmt.Errorf("failed to parse created skill: %w", err)
		return result, warning, err
	}

	// Preserve auto_inject if set
	if autoInject {
		parsedSkill.AutoInject = true
	}

	// Add to in-memory store
	localStore.Skills[name] = parsedSkill

	// Update keywords index (including name-based indexing)
	indexSkillKeywords(localStore.Keywords, name, parsedSkill.Keywords)

	// Persist skills.yaml
	if err = localStore.Save(); err != nil {
		warning = fmt.Sprintf("Warning: failed to save skills.yaml: %v", err)
		outfmt.Println(warning)
		err = nil
	}

	result = fmt.Sprintf("Local skill %q created successfully.\n\nPath: %s\nKeywords: %s",
		name, localDir, strings.Join(parsedSkill.Keywords, ", "))
	return result, warning, err
}
