package skills

import (
	"fmt"
	"maps"
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
	globalOnce  sync.Once
	localStore  *Store
	localOnce   sync.Once
)

type Store struct {
	dir      string
	source   string              // "local" 或 "global"
	Skills   map[string]Skill    `yaml:"skills,omitzero"`
	Keywords map[string][]string `yaml:"keywords,omitzero"`
}

func LocalStore() (*Store, error) {
	var err error
	localOnce.Do(func() {
		dir := filepath.Join(context.ProjectRoot, ".dscli", "skills")
		err = os.MkdirAll(dir, 0o755)
		if err != nil {
			return
		}
		localStore, err = NewSkillStore(dir, "local")
	})
	return localStore, err
}

func GlobalStore() (*Store, error) {
	var err error
	globalOnce.Do(func() {
		dir := filepath.Join(config.ConfigDir, "skills")
		err = os.MkdirAll(dir, 0o755)
		if err != nil {
			return
		}
		globalStore, err = NewSkillStore(dir, "global")
	})
	return globalStore, err
}

func NewSkillStore(dir string, source string) (*Store, error) {
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
			return
		}
		err = yaml.Unmarshal(data, store)
		if err != nil {
			return
		}
		// 注入 Source（缓存中不保存 Source）
		for name, skill := range store.Skills {
			skill.Source = store.source
			store.Skills[name] = skill
		}
		return
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
		return
	}

	kws := map[string][]string{}
	for name, skill := range skills {
		skill.Source = store.source // 注入来源

		// 保留旧的 auto_inject 设置（用户偏好，非 skill 定义）
		if old, ok := oldSkills[name]; ok {
			skill.AutoInject = old.AutoInject
		}

		for _, word := range skill.Keywords {
			var kw []string
			var ok bool
			if kw, ok = kws[word]; !ok {
				kw = []string{}
			}
			kw = append(kw, name)
			kws[word] = kw
		}
		skills[name] = skill
	}
	store.Skills = skills
	store.Keywords = kws

	// 保存到yaml文件，以便下次直接加载
	if err := store.Save(); err != nil {
		// 保存失败不影响使用，只是下次需要重新加载
		fmt.Printf("Warning: failed to save skills.yaml: %v\n", err)
	}

	return
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

func (store *Store) Query(query string) (matched map[string]Skill) {
	matched = map[string]Skill{}
	queryLower := strings.ToLower(query)
	for keyword, names := range store.Keywords {
		if strings.Contains(queryLower, keyword) {
			for _, name := range names {
				if skill, ok := store.Skills[name]; ok {
					matched[name] = skill
				}
			}
		}
	}
	return
}

func (store *Store) Use(name string) (content string, err error) {
	skill, ok := store.Skills[name]
	if ok {
		content = skill.FormatFull()
		return
	}

	err = fmt.Errorf("skill %s not exists", name)
	return
}

func Query(q string) (string, error) {
	localStore, err := LocalStore()
	if err != nil {
		return "", fmt.Errorf("failed to load local store: %w", err)
	}

	globalStore, err := GlobalStore()
	if err != nil {
		return "", fmt.Errorf("failed to load global store: %w", err)
	}

	localMatched := localStore.Query(q)
	matched := globalStore.Query(q)
	maps.Copy(matched, localMatched)

	if len(matched) == 0 {
		return "", fmt.Errorf("no skills found for query: %s", q)
	}

	var builder strings.Builder
	for name, skill := range matched {
		builder.WriteString("---skill name: ")
		builder.WriteString(name)
		builder.WriteString("---\n")
		builder.WriteString(skill.Summary())
	}
	return builder.String(), nil
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
	return "", fmt.Errorf("skill %s not found in local or global store", name)
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

// ListAll 返回所有技能（本地和全局）的列表
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
			skillInfos = append(skillInfos, SkillInfo{
				Name:       name,
				Scope:      "global",
				AutoInject: skill.AutoInject,
			})
		}
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
func SetAutoInject(name string, autoInject bool, global bool) error {
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

func HandleSkillCreate(ctx context.Context, name string, description, content, keywordsStr string, autoInject bool) (result string, warning string, err error) {
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
		return
	}

	// Create local skill directory: .dscli/skills/<name>/
	localDir := filepath.Join(context.ProjectRoot, ".dscli", "skills", name)
	if err = os.MkdirAll(localDir, 0o755); err != nil {
		err = fmt.Errorf("failed to create skill directory: %w", err)
		return
	}

	// Write SKILL.md
	skillFile := filepath.Join(localDir, "SKILL.md")
	if err = os.WriteFile(skillFile, []byte(skillMD), 0o644); err != nil {
		err = fmt.Errorf("failed to write SKILL.md: %w", err)
		return
	}

	// Register in local store so it's immediately usable via skill_by_name / skill_search
	localStore, err := LocalStore()
	if err != nil {
		// Non-fatal: skill file is on disk, will be picked up on next load
		warning = fmt.Sprintf("Warning: could not update local store cache: %v", err)
		outfmt.Println(warning)
		result = fmt.Sprintf("Skill %q created at %s (store cache update skipped).", name, localDir)
		err = nil
		return
	}

	// Parse the newly created SKILL.md to get the full Skill (with resources, etc.)
	var parsedSkill Skill
	if err = ParseSkill(skillFile, &parsedSkill); err != nil {
		err = fmt.Errorf("failed to parse created skill: %w", err)
		return
	}

	// Preserve auto_inject if set
	if autoInject {
		parsedSkill.AutoInject = true
	}

	// Add to in-memory store
	localStore.Skills[name] = parsedSkill

	// Update keywords index: ensure no duplicates
	for _, kw := range parsedSkill.Keywords {
		names := localStore.Keywords[kw]
		if !slices.Contains(names, name) {
			localStore.Keywords[kw] = append(names, name)
		}
	}

	// Persist skills.yaml
	if err = localStore.Save(); err != nil {
		warning = fmt.Sprintf("Warning: failed to save skills.yaml: %v", err)
		outfmt.Println(warning)
		err = nil
	}

	result = fmt.Sprintf("Local skill %q created successfully.\n\nPath: %s\nKeywords: %s",
		name, localDir, strings.Join(parsedSkill.Keywords, ", "))
	return
}
