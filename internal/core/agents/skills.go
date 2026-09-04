// skills.go — lazyskills mechanism for charmulet: markdown skill files in
// ~/.config/charmulet/skills/, toggled by renaming file.md <-> file.md.disabled.
// Charmulet never touches ~/.claude/skills — that is her live claude env.
package agents

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Skill is one lazyskill entry, markdown with an optional TOML-ish
// frontmatter block (--- delimited) carrying name/description.
type Skill struct {
	Name    string // filename stem, or frontmatter name
	Desc    string // frontmatter description or first body line
	Path    string
	Enabled bool
}

// SkillsDir returns the charmulet skills directory, creating it on first use.
func SkillsDir() (string, error) {
	root := os.Getenv("CHARMULET_CONFIG_DIR")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(home, ".config", "charmulet")
	}
	d := filepath.Join(root, "skills")
	return d, os.MkdirAll(d, 0o755)
}

// ListSkills enumerates *.md and *.md.disabled skill files, sorted by name.
// Missing dir = empty list.
func ListSkills() ([]Skill, error) {
	d, err := SkillsDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(d)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Skill
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		enabled := true
		if strings.HasSuffix(name, ".disabled") {
			name = strings.TrimSuffix(name, ".disabled")
			enabled = false
		}
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		stem := strings.TrimSuffix(name, ".md")
		s := Skill{Name: stem, Enabled: enabled, Path: filepath.Join(d, e.Name())}
		if b, err := os.ReadFile(s.Path); err == nil {
			parseSkillMeta(string(b), &s)
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// parseSkillMeta pulls name/description out of a --- frontmatter block,
// falling back to the first non-empty body line for the description.
func parseSkillMeta(text string, s *Skill) {
	body := text
	if strings.HasPrefix(text, "---\n") {
		if end := strings.Index(text[4:], "\n---"); end >= 0 {
			fm := text[4 : 4+end]
			body = text[4+end+4:]
			for _, line := range strings.Split(fm, "\n") {
				if v, ok := cutPrefix(line, "name = "); ok {
					s.Name = strings.Trim(v, `"`)
				}
				if v, ok := cutPrefix(line, "description = "); ok {
					s.Desc = strings.Trim(v, `"`)
				}
			}
		}
	}
	if s.Desc == "" {
		for _, line := range strings.Split(body, "\n") {
			t := strings.TrimSpace(line)
			if t != "" && t != "---" && !strings.HasPrefix(t, "#") {
				if len(t) > 80 {
					t = t[:79] + "…"
				}
				s.Desc = t
				return
			}
		}
	}
}

func cutPrefix(line, prefix string) (string, bool) {
	if strings.HasPrefix(line, prefix) {
		return strings.TrimSpace(line[len(prefix):]), true
	}
	return "", false
}

// ToggleSkill flips a skill between enabled and disabled via atomic rename.
func ToggleSkill(s Skill) (Skill, error) {
	var np string
	if s.Enabled {
		np = s.Path + ".disabled"
	} else {
		if !strings.HasSuffix(s.Path, ".disabled") {
			return s, fmt.Errorf("skill path %q lacks .disabled marker", s.Path)
		}
		np = strings.TrimSuffix(s.Path, ".disabled")
	}
	if err := os.Rename(s.Path, np); err != nil {
		return s, err
	}
	s.Path, s.Enabled = np, !s.Enabled
	return s, nil
}

// WriteSkill saves a skill's markdown body (creating or overwriting).
func WriteSkill(name, body string) error {
	if name == "" || strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		return fmt.Errorf("invalid skill name %q", name)
	}
	d, err := SkillsDir()
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(d, name+".md"), []byte(body), 0o644)
}
