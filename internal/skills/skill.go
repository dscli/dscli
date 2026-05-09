package skills

import (
	"bufio"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/goccy/go-yaml"
)

//go:embed how_to_use_a_skill.md
var how_to_use_a_skill_md string

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
	Examples    []Resource `yaml:"examples,omitzero"`    // 示例文件
	AutoInject  bool       `yaml:"auto_inject,omitzero"` // 自动注入到对话上下文，无需 LLM 主动获取
	Source      string     `yaml:"-"`                    // "local" 或 "global"，由加载侧注入
}

func (skill *Skill) Summary() string {
	var builder strings.Builder
	builder.WriteString("name: ")
	builder.WriteString(skill.Name)
	builder.WriteRune('\n')
	builder.WriteString("path: ")
	builder.WriteString(sanitizePath(skill.Path))
	builder.WriteRune('\n')
	builder.WriteString("description: ")
	builder.WriteString(skill.Description)
	builder.WriteRune('\n')
	return builder.String()
}

// FormatFull 格式化技能的完整信息，供 LLM 使用。
// 包含：摘要、正文、资源列表（仅路径和描述，不含内容，LLM 按需读取）。
func (skill *Skill) FormatFull() string {
	var builder strings.Builder

	// 摘要
	builder.WriteString("---\n")
	builder.WriteString(skill.Summary())
	builder.WriteString("---\n\n")

	// 正文
	builder.WriteString(skill.Content)

	// 资源列表
	formatResourceSection(&builder, "Scripts", skill.Scripts)
	formatResourceSection(&builder, "References", skill.References)
	formatResourceSection(&builder, "Templates", skill.Templates)
	formatResourceSection(&builder, "Examples", skill.Examples)

	return builder.String()
}

// formatResourceSection 格式化单个资源类型的列表，仅包含路径和描述。
func formatResourceSection(builder *strings.Builder, title string, resources []Resource) {
	if len(resources) == 0 {
		return
	}
	builder.WriteString("\n## ")
	builder.WriteString(title)
	builder.WriteString("\n\n")
	for _, res := range resources {
		// 相对路径（Name）便于在 SKILL.md 中引用
		// 绝对路径（sanitized）供 LLM 直接访问
		sanitized := sanitizePath(res.Path)
		builder.WriteString("- `")
		builder.WriteString(res.Name)
		builder.WriteString("`")
		if res.Description != "" {
			builder.WriteString(" — ")
			builder.WriteString(res.Description)
		}
		builder.WriteString("\n  path: ")
		builder.WriteString(sanitized)
		builder.WriteString("\n")
	}
}

// sanitizePath 脱敏绝对路径，将用户主目录替换为 ~
func sanitizePath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if strings.HasPrefix(p, home) {
		return "~" + p[len(home):]
	}
	return p
}

// FormatSkillMD 生成带 frontmatter 的 SKILL.md 内容，用于 skill_save 工具。
// 只序列化 frontmatter 必要字段（name, description, keywords, auto_inject），
// 正文部分直接拼接。
func FormatSkillMD(skill *Skill) (string, error) {
	// 构建 frontmatter YAML
	type frontmatter struct {
		Name        string   `yaml:"name"`
		Description string   `yaml:"description"`
		Keywords    []string `yaml:"keywords,omitzero"`
		AutoInject  bool     `yaml:"auto_inject,omitzero"`
	}
	fm := frontmatter{
		Name:        skill.Name,
		Description: skill.Description,
		Keywords:    skill.Keywords,
		AutoInject:  skill.AutoInject,
	}

	yamlBytes, err := yaml.Marshal(fm)
	if err != nil {
		return "", fmt.Errorf("marshal frontmatter: %w", err)
	}

	var b strings.Builder
	b.WriteString("---\n")
	b.Write(yamlBytes)
	b.WriteString("---\n\n")
	b.WriteString(skill.Content)
	return b.String(), nil
}

func LoadSkills(dir string) (skills map[string]Skill) {
	skills = map[string]Skill{}
	filenames := SkillFiles(dir)
	for _, filename := range filenames {
		var skill Skill
		if err := ParseSkill(filename, &skill); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: parse skill %s failed: %v\n", filename, err)
			continue
		}
		skills[skill.Name] = skill
	}
	return skills
}

// SkillFiles returns all skill files (SKILL.md) under `dir`.
// It walks the directory tree recursively and returns absolute paths.
// Skips common non-skill directories (.git, node_modules, etc.) and respects
// a max depth bound to prevent runaway scanning in large directory trees.
func SkillFiles(dir string) (skillFiles []string) {
	// Count depth relative to base directory by counting path separators
	relDepth := func(base, target string) int {
		// Use relative path to count separator differences
		rel, err := filepath.Rel(base, target)
		if err != nil {
			return 0
		}
		return strings.Count(rel, string(filepath.Separator))
	}

	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // 忽略错误，继续遍历
		}
		if d.IsDir() {
			name := d.Name()
			// Skip directories that won't contain skills
			if name == ".git" || name == "node_modules" ||
				name == ".venv" || name == "__pycache__" ||
				name == "vendor" || (strings.HasPrefix(name, ".") && name != "." && name != "..") {
				return filepath.SkipDir
			}
			// Max depth bound: 6 levels from the base
			if relDepth(dir, path) > 6 {
				return filepath.SkipDir
			}
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
	return skillFiles
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

	// 名称验证（宽松模式：警告但不阻断加载）
	validateSkillName(skill.Name, skillDir, absPath)

	// 描述验证（规范要求：空描述必须跳过技能）
	if strings.TrimSpace(skill.Description) == "" {
		return fmt.Errorf("description is empty")
	}
	// 如果YAML中没有关键词，则从description中提取
	if len(skill.Keywords) == 0 {
		skill.Keywords = extractKeywords(skill.Name, skill.Description)
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
	if err := loadExamples(skillDir, skill); err != nil {
		return fmt.Errorf("load examples: %w", err)
	}

	return nil
}

// validateSkillName checks skill name against the spec constraints.
// Issues are logged as warnings; loading continues (lenient validation).
func validateSkillName(name, skillDir, skillPath string) {
	dirName := filepath.Base(skillDir)

	// Check: name matches parent directory (spec requirement)
	if name != dirName {
		fmt.Fprintf(os.Stderr,
			"Warning: skill name %q does not match directory %q in %s\n",
			name, dirName, skillPath)
	}

	// Check: name length ≤ 64 characters
	if len(name) > 64 {
		fmt.Fprintf(os.Stderr,
			"Warning: skill name %q exceeds 64 characters (is %d) in %s\n",
			name, len(name), skillPath)
	}

	// Check: name format — lowercase alphanumeric + hyphens only (kebab-case)
	if !validSkillNamePattern(name) {
		fmt.Fprintf(os.Stderr,
			"Warning: skill name %q contains invalid characters; use only lowercase letters, digits, and hyphens in %s\n",
			name, skillPath)
	}
}

// validSkillNamePattern checks whether the name follows the spec:
// lowercase alphanumeric characters and hyphens only (kebab-case).
func validSkillNamePattern(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if r >= 'a' && r <= 'z' {
			continue
		}
		if r >= '0' && r <= '9' {
			continue
		}
		if r == '-' {
			// hyphens allowed but not at start or end
			if i == 0 || i == len(name)-1 {
				return false
			}
			continue
		}
		return false
	}
	return true
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

	// 解析 YAML（先规范化易出错字段）
	frontMatter := normalizeFrontmatter(strings.Join(yamlLines, "\n"))
	if err := yaml.Unmarshal([]byte(frontMatter), skill); err != nil {
		return fmt.Errorf("yaml unmarshal: %w", err)
	}

	// 读取剩余内容作为正文
	var builder strings.Builder
	for scanner.Scan() {
		builder.WriteString(scanner.Text())
		builder.WriteString("\n")
	}
	// 去除前导空行（frontmatter 闭合符 "---" 后的分隔空行）
	skill.Content = strings.TrimLeft(builder.String(), "\n")
	return nil
}

// reKeywordLine 匹配单行 keywords: <value>（非 block 格式）。
var reKeywordLine = regexp.MustCompile(`(?m)^keywords:\s*(.+)$`)

// reDescColon 匹配单行 description: <value>，用于检测未引号冒号。
var reDescColon = regexp.MustCompile(`(?m)^description:\s*(.+)$`)

// normalizeFrontmatter 修复常见的 YAML frontmatter 书写错误，使其能被解析。
// 例如：keywords: go, modern → keywords: [go, modern]
// 例如：description: Use when: user asks → description: 'Use when: user asks'
func normalizeFrontmatter(fm string) string {
	fm = reKeywordLine.ReplaceAllStringFunc(fm, func(match string) string {
		after := strings.TrimSpace(match[len("keywords:"):])
		// Already valid YAML list (block or inline)? Keep.
		if after == "" || after[0] == '[' || after[0] == '-' {
			return match
		}
		// Plain string — treat as comma-separated, convert to YAML inline list.
		after = strings.Trim(after, `"'`)
		parts := strings.Split(after, ",")
		var list []string
		for _, p := range parts {
			if p = strings.TrimSpace(p); p != "" {
				list = append(list, p)
			}
		}
		if len(list) == 0 {
			return "keywords: []"
		}
		return "keywords: [" + strings.Join(list, ", ") + "]"
	})
	fm = reDescColon.ReplaceAllStringFunc(fm, func(match string) string {
		after := strings.TrimSpace(match[len("description:"):])
		if after == "" {
			return match
		}
		// Already block scalar or quoted? Keep.
		if after[0] == '|' || after[0] == '>' || after[0] == '"' || after[0] == '\'' {
			return match
		}
		// No colon to escape? Keep.
		if !strings.Contains(after, ":") {
			return match
		}
		// Quote the value to prevent YAML from treating colon as mapping indicator.
		return "description: " + quoteYAMLValue(after)
	})
	return fm
}

// quoteYAMLValue 对包含特殊字符的 YAML 值加引号包裹。
// 优先使用单引号（无需转义），包含单引号时用双引号并转义反斜杠和双引号。
func quoteYAMLValue(value string) string {
	if !strings.Contains(value, "'") {
		return "'" + value + "'"
	}
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}

// extractKeywords 从 description 中提取关键词。
// 支持格式：
//   - "关键词：abc, def, xyz"
//   - "Keywords: abc, def, xyz"
//   - "TRIGGER when: ... 关键词: foo, bar"
//
// 如果以上模式均未匹配，则回退到基于 name 和 description 的自动提取。
func extractKeywords(name, desc string) []string {
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

	// 回退：从 name 和 description 自动提取关键词
	return extractKeywordsFromNameAndDesc(name, desc)
}

// extractKeywordsFromNameAndDesc 从技能名称和描述中自动提取关键词。
// 策略：
//  1. 名称分词（按 - _ . 分割）
//  2. 描述分词（过滤英文停用词和短词）
//  3. 合并去重
func extractKeywordsFromNameAndDesc(name, desc string) []string {
	seen := make(map[string]bool)

	// 名称分词：user-modern-go → ["user", "modern", "go"]
	for _, token := range tokenizeName(name) {
		if !isStopword(token) {
			seen[token] = true
		}
	}

	// 描述分词
	for _, token := range tokenizeText(desc) {
		seen[token] = true
	}

	keywords := make([]string, 0, len(seen))
	for kw := range seen {
		keywords = append(keywords, kw)
	}
	sort.Strings(keywords)
	return keywords
}

// tokenizeName 将技能名称按分隔符拆分为 token。
// "use-modern-go" → ["use", "modern", "go"]
// "go_fix" → ["go", "fix"]
func tokenizeName(name string) []string {
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '-' || r == '_' || r == '.'
	})
	tokens := make([]string, 0, len(parts))
	seen := make(map[string]bool, len(parts))
	for _, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		if p != "" && !seen[p] {
			seen[p] = true
			tokens = append(tokens, p)
		}
	}
	return tokens
}
// tokenizeText 将文本拆分为英文 token（小写、去标点、去停用词）。
// 用于 description 的关键词提取，过滤高频无信息量词汇。
// 短词（≤2 字符）也被过滤，防止 "on", "s" 等造成误匹配。
// 用于 description 的关键词提取，过滤高频无信息量词汇。
// 短词（≤2 字符）也被过滤，防止 "on", "s" 等造成误匹配。
func tokenizeText(text string) []string {
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	tokens := make([]string, 0, len(words))
	seen := make(map[string]bool, len(words))
	for _, w := range words {
		w = strings.ToLower(strings.TrimSpace(w))
		if w != "" && len(w) > 2 && !isStopword(w) && !seen[w] {
			seen[w] = true
			tokens = append(tokens, w)
		}
	}
	return tokens
}

// tokenizeQuery 将查询字符串拆分为 token（小写、去标点、不过滤停用词）。
// 与 tokenizeText 不同：查询中不应过滤停用词，因为用户可能按 skill 名称搜索。
func tokenizeQuery(text string) []string {
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	tokens := make([]string, 0, len(words))
	seen := make(map[string]bool, len(words))
	for _, w := range words {
		w = strings.ToLower(strings.TrimSpace(w))
		if w != "" && !seen[w] {
			seen[w] = true
			tokens = append(tokens, w)
		}
	}
	return tokens
}

// isStopword 判断是否为英文停用词（高频低信息量词汇）。
//
// 停用词列表涵盖英语中最常见的功能词（冠词、代词、介词等）
// 以及与 skill 描述相关的常见高频词（use, ask, apply, based 等）。
// 停用词总数为 ~80 个，在实际场景中经过调优。
func isStopword(word string) bool {
	// 内联停用词集合 — 避免运行时构建开销
	switch word {
	case "a", "an", "the",
		"is", "are", "was", "were", "be", "been", "being",
		"have", "has", "had", "do", "does", "did",
		"will", "would", "shall", "should", "may", "might", "must", "can", "could",
		"you", "he", "she", "they", "him", "her", "them",
		"this", "that", "these", "those",
		"and", "but", "nor", "not", "then", "else",
		"how", "what", "which", "who", "whom", "why",
		"from", "into", "through", "during", "before", "after",
		"above", "below", "between", "under", "over",
		"all", "any", "both", "each", "few", "more", "most", "other", "some", "such",
		"only", "own", "same", "than", "too", "very", "just",
		"use", "used", "using", "ask", "asked", "apply",
		"applied", "based", "when", "where", "here", "there",
		"about", "also", "back", "make", "made",
		"get", "got", "see", "need":
		return true
	}
	return false
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

// loadReferences 加载 references（或 reference）目录下的文档资源。
// 优先检查 "references" 目录，不存在则回退到 "reference"（单数形式）。

func loadReferences(skillDir string, skill *Skill) error {
	// 先尝试 "references"，再尝试 "reference"
	refDir := filepath.Join(skillDir, "references")
	entries, err := os.ReadDir(refDir)
	if os.IsNotExist(err) {
		refDir = filepath.Join(skillDir, "reference")
		entries, err = os.ReadDir(refDir)
		if os.IsNotExist(err) {
			return nil
		}
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

		// Name 使用加载时实际的目录名（references 或 reference）
		dirName := filepath.Base(refDir)
		res := Resource{
			Name: filepath.Join(dirName, name),
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

// loadExamples 加载 examples 目录下的示例文件。
func loadExamples(skillDir string, skill *Skill) error {
	examplesDir := filepath.Join(skillDir, "examples")
	entries, err := os.ReadDir(examplesDir)
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
			Name: filepath.Join("examples", name),
			Path: filepath.Join(examplesDir, name),
		}
		// 提取描述：Markdown 取标题，否则取第一行注释
		if strings.HasSuffix(name, ".md") {
			res.Description = extractMarkdownTitle(res.Path)
		} else {
			res.Description = extractFirstCommentOrEmpty(res.Path)
		}
		skill.Examples = append(skill.Examples, res)
	}
	return nil
}

// BuildSkillPrompt builds the skill injection prompt.
//
// It loads from local (.dscli/skills) and global (~/.dscli/skills) stores,
// merging with local priority. Two injection strategies:
//  1. auto_inject skills: full content injected directly, no LLM fetch needed
//  2. manual skills: name/description list only, LLM fetches via skill_by_name
//
// Use skill_search tool for keyword-based discovery of manual skills.
// Store loading errors are gracefully degraded to empty stores
// so that skill failures never block conversation.
//
// When allowed is empty or nil, all skills are included (backward compatible).
// When allowed contains "all", all skills are included.
// When allowed contains specific names, only those skills are included.
func BuildSkillPrompt(ctx context.Context, allowed ...string) string {
	localStore, localErr := LocalStore()
	if localErr != nil {
		localStore = &Store{}
	}

	globalStore, globalErr := GlobalStore()
	if globalErr != nil {
		globalStore = &Store{}
	}

	// Merge skills: local takes priority over global
	allSkills := make(map[string]Skill)
	maps.Copy(allSkills, globalStore.Skills)
	maps.Copy(allSkills, localStore.Skills)

	// Apply allowlist filter if specified
	if len(allowed) > 0 && !(len(allowed) == 1 && allowed[0] == "all") {
		allowSet := make(map[string]bool, len(allowed))
		for _, a := range allowed {
			allowSet[a] = true
		}
		filtered := make(map[string]Skill)
		for name, skill := range allSkills {
			if allowSet[name] {
				filtered[name] = skill
			}
		}
		allSkills = filtered
	}

	if len(allSkills) == 0 {
		return "" // No skills, no injection
	}

	// Sort for stable output
	names := make([]string, 0, len(allSkills))
	for name := range allSkills {
		names = append(names, name)
	}
	sort.Strings(names)

	// Separate auto_inject skills from manual ones
	var autoSkills, manualSkills []Skill
	for _, name := range names {
		skill := allSkills[name]
		if skill.AutoInject {
			autoSkills = append(autoSkills, skill)
		} else {
			manualSkills = append(manualSkills, skill)
		}
	}

	// Cap manual skills listed to avoid token waste
	const maxManualListed = 20
	hasMore := len(manualSkills) > maxManualListed
	if hasMore {
		manualSkills = manualSkills[:maxManualListed]
	}

	var builder strings.Builder

	// === Part 1: Auto-inject skills (full content) ===
	for _, skill := range autoSkills {
		builder.WriteString("---\n")
		fmt.Fprintf(&builder, "## Skill: %s (auto-loaded)\n\n", skill.Name)
		builder.WriteString(skill.Content)
		builder.WriteString("\n\n")
	}
	// === Part 2: Manual skill list ===
	if len(manualSkills) > 0 {
		builder.WriteString("## Available Skills\n\n")
		builder.WriteString("Fetch full content via `skill_by_name` tool, ")
		builder.WriteString("then execute scripts via `shell` tool.\n")
		builder.WriteString("Not sure which skill to use? Try `skill_search` with keywords.\n\n")
		builder.WriteString("| Name | Description | Keywords |\n")
		builder.WriteString("|------|-------------|----------|\n")

		for _, skill := range manualSkills {
			keywords := "-"
			if len(skill.Keywords) > 0 {
				keywords = strings.Join(skill.Keywords, ", ")
			}
			// Include sanitized path in description for path resolution
			desc := truncateSkillDesc(skill.Description, 80)
			fmt.Fprintf(&builder, "| %s | %s | %s |\n",
				skill.Name,
				desc,
				keywords,
			)
		}

		if hasMore {
			fmt.Fprintf(&builder, "| ... | _(%d more skills, use skill_search to discover)_ | ... |\n",
				len(names)-len(autoSkills)-maxManualListed)
		}
		// Provide valid skill names to prevent hallucination
		// List ALL valid names (not just capped table entries) so the LLM
		// knows the complete set of skills available via skill_by_name.
		allManualNames := make([]string, 0, len(names)-len(autoSkills))
		for _, name := range names {
			if skill := allSkills[name]; !skill.AutoInject {
				allManualNames = append(allManualNames, name)
			}
		}
		builder.WriteString("\n**Valid skill names**: ")
		builder.WriteString(strings.Join(allManualNames, ", "))
		builder.WriteString("\n_(use exact names above with `skill_by_name` tool)_\n")
	}

	// === Part 3: Usage instructions with example ===
	builder.WriteString("\n")
	builder.WriteString(how_to_use_a_skill_md)
	return builder.String()
}

// truncateSkillDesc truncates a skill description to maxLen runes, appending "..." if needed.
func truncateSkillDesc(desc string, maxLen int) string {
	if maxLen < 3 {
		maxLen = 3 // guard against negative slice index
	}
	runes := []rune(desc)
	if len(runes) <= maxLen {
		return desc
	}
	return string(runes[:maxLen-3]) + "..."
}