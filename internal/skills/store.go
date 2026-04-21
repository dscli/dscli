package skills

import (
	"maps"
	"fmt"
	"os"
	"path/filepath"
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
	return
}

func (store *Store) Query(query string) (matched map[string]Skill) {
	matched = map[string]Skill{}
	queryWords := strings.Fields(strings.ToLower(query))
	for _, qw := range queryWords {
		if skills, ok := store.Keywords[qw]; ok {
			for _, name := range skills {
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