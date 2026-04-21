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

func LocalStore() *Store {
	localOnce.Do(func() {
		dir := filepath.Join(context.ProjectRoot, ".dscli", "skills")
		err := os.MkdirAll(dir, 0o755)
		if err != nil {
			panic(err)
		}
		localStore = NewSkillStore(dir)
	})
	return localStore
}

func GlobalStore() *Store {
	globalOnce.Do(func() {
		dir := filepath.Join(config.ConfigDir, "skills")
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			panic(err)
		}
		globalStore = NewSkillStore(dir)

	})
	return globalStore
}

func NewSkillStore(dir string) (store *Store) {
	store = &Store{
		dir: dir,
	}
	err := store.Load()
	if err != nil {
		return nil
	}
	return
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
	for keyword, names := range store.Keywords {
		if strings.Contains(query, keyword) {
			for _, name := range names {
				skill, ok := store.Skills[name]
				if ok {
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

func Query(q string) string {
	localMatched := LocalStore().Query(q)
	matched := GlobalStore().Query(q)
	maps.Copy(matched, localMatched)
	var builder strings.Builder
	for name, skill := range matched {
		builder.WriteString("---skill name: ")
		builder.WriteString(name)
		builder.WriteString("---\n")
		builder.WriteString(skill.Summary())
	}
	return builder.String()
}
func Use(name string) (content string) {
	var err error
	local := LocalStore()
	content, err = local.Use(name)
	if err == nil {
		return
	}
	global := GlobalStore()
	content, err = global.Use(name)
	if err == nil {
		return
	}
	err = fmt.Errorf("skill %s not exists", name)
	return
}
