package skills

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gitcode.com/dscli/dscli/internal/config"
	"gitcode.com/dscli/dscli/internal/context"
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
		localStore, err = NewSkillStore(dir)
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
		globalStore, err = NewSkillStore(dir)
	})
	return globalStore, err
}

func NewSkillStore(dir string) (*Store, error) {
	store := &Store{
		dir: dir,
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
	_, err = os.Stat(path)
	if err == nil {
		var data []byte
		data, err = os.ReadFile(path)
		if err != nil {
			return
		}
		err = yaml.Unmarshal(data, store)
		if err != nil {
			return
		}
	}

	if err == nil {
		return
	}

	err = nil
	skills := LoadSkills(store.dir)
	if len(skills) == 0 {
		err = fmt.Errorf("no skill loaded")
		return
	}

	kws := map[string][]string{}
	for name, skill := range skills {
		for _, word := range skill.Keywords {
			var kw []string
			var ok bool
			if kw, ok = kws[word]; !ok {
				kw = []string{}
			}
			kw = append(kw, name)
			kws[word] = kw
		}
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
		summary := skill.Summary()
		content = fmt.Sprintf("---\n%s---\n\n%s\n", summary, skill.Content)
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
		skillInfos = append(skillInfos, SkillInfo{
			Name:  name,
			Scope: "local",
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
			skillInfos = append(skillInfos, SkillInfo{
				Name:  name,
				Scope: "global",
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
// SkillInfo 包含技能的基本信息和作用域
type SkillInfo struct {
	Name  string `json:"name"`
	Scope string `json:"scope"` // "local" 或 "global"
}
