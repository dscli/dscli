package skills

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/goccy/go-yaml"
)

var ErrInvalidFrontmatter = errors.New("invalid frontmatter format")

type Resource struct {
	Name        string `yaml:"name,omitzero"`
	Description string `yaml:"description,omitzero"`
	Path        string `yaml:"path,omitzero"`
}

type Skill struct {
	Name        string     `yaml:"name,omitzero"`
	Description string     `yaml:"description,omitzero"`
	Path        string     `yaml:"path,omitzero"`    // 技能根目录
	Content     string     `yaml:"content,omitzero"` // SKILL.md 正文
	Keywords    []string   `yaml:"keywords,omitzero"`
	Scripts     []Resource `yaml:"scripts,omitzero"`
	References  []Resource `yaml:"references,omitzero"`
	Templates   []Resource `yaml:"templates,omitzero"`
}

func (skill *Skill) Summary() string {
	var builder strings.Builder
	builder.WriteString("name: ")
	builder.WriteString(skill.Name)
	builder.WriteRune('\n')
	builder.WriteString("description: ")
	builder.WriteString(skill.Description)
	builder.WriteRune('\n')
	return builder.String()
}

func LoadSkills(dir string) (skills map[string]Skill) {
	skills = map[string]Skill{}
	filenames := SkillFiles(dir)
	for _, filename := range filenames {
		var skill Skill
		err := ParseSkill(filename, &skill)
		if err == nil {
			skills[skill.Name] = skill
		}
	}
	return
}

// SkillFiles returns all skill files (SKILL.md) under `dir`.
// It walks the directory tree recursively and returns absolute paths.
func SkillFiles(dir string) (skillFiles []string) {
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // 忽略错误，继续遍历
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() == "SKILL.md" {
			absPath, err := filepath.Abs(path)
			if err == nil {
				skillFiles = append(skillFiles, absPath)
			}
		}
		return nil
	})
	return
}

// ParseSkill 解析指定路径的 SKILL.md 文件，填充 skill 对象。
// path 必须是 SKILL.md 文件的完整路径。
func ParseSkill(path string, skill *Skill) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	f, err := os.Open(absPath)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := parseSkillFrontmatter(f, skill); err != nil {
		return err
	}

	// 技能根目录 = SKILL.md 所在目录
	skillDir := filepath.Dir(absPath)
	skill.Path = skillDir

	// 如果YAML中没有关键词，则从description中提取
	if len(skill.Keywords) == 0 {
		skill.Keywords = extractKeywords(skill.Description)
	}

	// 加载子资源
	if err := loadScripts(skillDir, skill); err != nil {
		return fmt.Errorf("load scripts: %w", err)
	}
	if err := loadReferences(skillDir, skill); err != nil {
		return fmt.Errorf("load references: %w", err)
	}
	if err := loadTemplates(skillDir, skill); err != nil {
		return fmt.Errorf("load templates: %w", err)
	}

	return nil
}

// parseSkillFrontmatter 解析 r 中的 frontmatter 和正文。
func parseSkillFrontmatter(r io.Reader, skill *Skill) error {
	scanner := bufio.NewScanner(r)

	// 检查第一行是否为 "---"
	if !scanner.Scan() {
		return ErrInvalidFrontmatter
	}
	line := strings.TrimSpace(scanner.Text())
	if !strings.HasPrefix(line, "---") {
		return ErrInvalidFrontmatter
	}

	// 收集 frontmatter 直到下一个 "---"
	var yamlLines []string
	for scanner.Scan() {
		line = scanner.Text()
		if strings.HasPrefix(strings.TrimSpace(line), "---") {
			break
		}
		yamlLines = append(yamlLines, line)
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	// 解析 YAML
	frontMatter := strings.Join(yamlLines, "\n")
	if err := yaml.Unmarshal([]byte(frontMatter), skill); err != nil {
		return fmt.Errorf("yaml unmarshal: %w", err)
	}

	// 读取剩余内容作为正文
	var builder strings.Builder
	for scanner.Scan() {
		builder.WriteString(scanner.Text())
		builder.WriteString("\n")
	}
	skill.Content = builder.String()
	return nil
}

// extractKeywords 从 description 中提取关键词。
// 支持格式：
//   - "关键词：abc, def, xyz"
//   - "Keywords: abc, def, xyz"
//   - "TRIGGER when: ... 关键词: foo, bar"
func extractKeywords(desc string) []string {
	// 正则匹配中文或英文关键词前缀后的内容
	patterns := []string{
		`关键词[:：]\s*([^\n]+)`,
		`Keywords[:：]\s*([^\n]+)`,
		`TRIGGER.*?关键词[:：]\s*([^\n]+)`,
	}
	for _, pat := range patterns {
		re := regexp.MustCompile(pat)
		matches := re.FindStringSubmatch(desc)
		if len(matches) > 1 {
			kwStr := strings.TrimSpace(matches[1])
			// 按逗号或空格分割
			parts := strings.FieldsFunc(kwStr, func(r rune) bool {
				return r == ',' || r == '，' || r == ' ' || r == '、'
			})
			keywords := make([]string, 0, len(parts))
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					keywords = append(keywords, strings.ToLower(p))
				}
			}
			return keywords
		}
	}
	return nil
}

// loadScripts 加载 scripts 目录下的可执行脚本资源。
func loadScripts(skillDir string, skill *Skill) error {
	scriptsDir := filepath.Join(skillDir, "scripts")
	entries, err := os.ReadDir(scriptsDir)
	if os.IsNotExist(err) {
		return nil // 没有 scripts 目录是允许的
	}
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := filepath.Ext(name)
		if ext != ".sh" && ext != ".py" {
			continue // 只处理 .sh 和 .py
		}

		res := Resource{
			Name: filepath.Join("scripts", name),
			Path: filepath.Join(scriptsDir, name),
		}
		res.Description = extractScriptDescription(res.Path)
		skill.Scripts = append(skill.Scripts, res)
	}
	return nil
}

// extractScriptDescription 读取脚本文件，跳过 shebang 行，提取第一行有效注释作为描述。
func extractScriptDescription(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// 跳过 shebang 行
		if strings.HasPrefix(line, "#!") {
			continue
		}
		// 提取注释行（# 或 //）
		if after, ok := strings.CutPrefix(line, "#"); ok {
			return strings.TrimSpace(after)
		}
		if after, ok := strings.CutPrefix(line, "//"); ok {
			return strings.TrimSpace(after)
		}
		// 如果第一行非注释非 shebang，说明脚本开头没有注释描述，直接返回空
		break
	}
	return ""
}

// loadReferences 加载 references 目录下的文档资源。
func loadReferences(skillDir string, skill *Skill) error {
	refDir := filepath.Join(skillDir, "references")
	entries, err := os.ReadDir(refDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}

		res := Resource{
			Name: filepath.Join("references", name),
			Path: filepath.Join(refDir, name),
		}
		res.Description = extractMarkdownTitle(res.Path)
		skill.References = append(skill.References, res)
	}
	return nil
}

// extractMarkdownTitle 提取 Markdown 文件的第一行一级标题作为描述。
func extractMarkdownTitle(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if after, ok := strings.CutPrefix(line, "# "); ok {
			return strings.TrimSpace(after)
		}
		break
	}
	return ""
}

// loadTemplates 加载 templates 目录下的模板文件。
func loadTemplates(skillDir string, skill *Skill) error {
	tplDir := filepath.Join(skillDir, "templates")
	entries, err := os.ReadDir(tplDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		res := Resource{
			Name: filepath.Join("templates", name),
			Path: filepath.Join(tplDir, name),
		}
		// 尝试提取描述：如果是 Markdown 则取标题，否则留空或取第一行注释
		if strings.HasSuffix(name, ".md") {
			res.Description = extractMarkdownTitle(res.Path)
		} else {
			res.Description = extractFirstCommentOrEmpty(res.Path)
		}
		skill.Templates = append(skill.Templates, res)
	}
	return nil
}

// extractFirstCommentOrEmpty 尝试从文件中提取第一行注释作为描述，支持常见格式。
func extractFirstCommentOrEmpty(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// 支持 #、//、/* 等注释风格
		if after, ok := strings.CutPrefix(line, "#"); ok {
			return strings.TrimSpace(after)
		}
		if after, ok := strings.CutPrefix(line, "//"); ok {
			return strings.TrimSpace(after)
		}
		if after, ok := strings.CutPrefix(line, "/*"); ok {
			// 简单处理，去掉前缀和可能的后缀 */
			desc := after
			desc = strings.TrimSuffix(desc, "*/")
			return strings.TrimSpace(desc)
		}
		break
	}
	return ""
}
