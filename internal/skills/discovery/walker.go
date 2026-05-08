// Package discovery walks filesystem trees to find agent skills.
package discovery

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/psacc/omnisess/internal/skills"
)

// scanFrontmatterFn is the function used to parse frontmatter from an open file.
// It is a package-level var so tests can inject a broken reader to exercise
// the scanFrontmatter error path inside parseSkillFile without needing an
// actual unreadable file.
var scanFrontmatterFn = scanFrontmatter

// parseSkillFile reads a SKILL.md and extracts frontmatter metadata.
// Returns a SkillInfo with Path, DescChars, BodyBytes, LastModified populated.
// Name comes from frontmatter `name:` field if present; otherwise empty
// (caller fills in from directory name).
// Source is left empty here — the walker assigns it based on location.
func parseSkillFile(path string) (skills.SkillInfo, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return skills.SkillInfo{}, fmt.Errorf("stat %s: %w", path, err)
	}
	info := skills.SkillInfo{
		Path:         path,
		BodyBytes:    fi.Size(),
		LastModified: fi.ModTime(),
	}

	f, err := os.Open(path)
	if err != nil {
		return info, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	name, descChars, err := scanFrontmatterFn(f, path)
	if err != nil {
		return info, err
	}
	info.Name = name
	info.DescChars = descChars
	return info, nil
}

// scanFrontmatter parses YAML frontmatter (between "---" delimiters) from r.
// Returns name, description char count, and any scanner error.
// label is used only for error message formatting (typically the file path).
func scanFrontmatter(r io.Reader, label string) (name string, descChars int, err error) {
	sc := bufio.NewScanner(r)
	if !sc.Scan() || strings.TrimSpace(sc.Text()) != "---" {
		return "", 0, nil // no frontmatter
	}

	var (
		currentField string
		descLines    []string
	)
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "---" {
			break
		}
		// top-level field detection: "key: value" or "key: |"
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			if i := strings.Index(line, ":"); i > 0 {
				key := strings.TrimSpace(line[:i])
				val := strings.TrimSpace(line[i+1:])
				currentField = key
				switch key {
				case "name":
					name = stripQuotes(val)
				case "description":
					if val == "|" || val == ">" {
						// multiline; collect indented lines below
						continue
					}
					descLines = []string{stripQuotes(val)}
				}
				continue
			}
		}
		// continuation of multiline description
		if currentField == "description" {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				descLines = append(descLines, trimmed)
			}
		}
	}
	if scanErr := sc.Err(); scanErr != nil {
		return name, 0, fmt.Errorf("scan %s: %w", label, scanErr)
	}
	return name, len(strings.Join(descLines, " ")), nil
}

func stripQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// Options configures the discovery walk.
type Options struct {
	Roots          []string
	IncludeGlobals bool
	GlobalPaths    GlobalPaths // overridable for tests; zero value uses defaults via Resolve
}

// GlobalPaths holds the three Claude-Code-convention global skill locations.
// Zero values mean "use Resolve() defaults" (i.e. expand from $HOME).
type GlobalPaths struct {
	ClaudeUser  string // ~/.claude/skills
	AgentsUser  string // ~/.agents/skills
	PluginsRoot string // ~/.claude/plugins (walked recursively for any */skills/)
}

// Resolve fills in any zero-valued GlobalPaths from $HOME.
func (g GlobalPaths) Resolve() (GlobalPaths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return g, err
	}
	if g.ClaudeUser == "" {
		g.ClaudeUser = filepath.Join(home, ".claude", "skills")
	}
	if g.AgentsUser == "" {
		g.AgentsUser = filepath.Join(home, ".agents", "skills")
	}
	if g.PluginsRoot == "" {
		g.PluginsRoot = filepath.Join(home, ".claude", "plugins")
	}
	return g, nil
}

// excludeSegments are path segments that always cause the walker to skip a directory.
var excludeSegments = map[string]struct{}{
	".worktrees":   {},
	"worktrees":    {},
	"node_modules": {},
	".venv":        {},
	".git":         {},
	"dist":         {},
	"build":        {},
	".scratch":     {},
}

// Walk discovers SKILL.md files across roots and (optionally) globals.
// Returns deduplicated SkillInfo slice. Dedup rule: if the same skill name
// appears more than once, the entry with the most-specific source wins
// (project beats global; first-seen wins on tie).
func Walk(opts Options) ([]skills.SkillInfo, error) {
	var out []skills.SkillInfo
	seen := map[string]int{} // name → index in out

	add := func(info skills.SkillInfo) {
		if info.Name == "" {
			// Use directory name as fallback
			info.Name = filepath.Base(filepath.Dir(info.Path))
		}
		if existing, ok := seen[info.Name]; ok {
			// collision: keep the more-specific source (project > global)
			if sourceRank(info.Source) > sourceRank(out[existing].Source) {
				out[existing] = info
			}
			return
		}
		seen[info.Name] = len(out)
		out = append(out, info)
	}

	// Walk roots
	for _, root := range opts.Roots {
		if err := walkRoot(root, add); err != nil {
			return nil, err
		}
	}

	// Walk globals
	if opts.IncludeGlobals {
		gp, err := opts.GlobalPaths.Resolve()
		if err != nil {
			return nil, err
		}
		if err := walkGlobalDir(gp.ClaudeUser, skills.SourceClaudeGlobal, add); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		if err := walkGlobalDir(gp.AgentsUser, skills.SourceAgentsGlobal, add); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		if err := walkPluginRoot(gp.PluginsRoot, add); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}
	return out, nil
}

func walkRoot(root string, add func(skills.SkillInfo)) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			if _, excluded := excludeSegments[d.Name()]; excluded {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "SKILL.md" {
			return nil
		}
		info, err := parseSkillFile(path)
		if err != nil {
			return err
		}
		info.Source = classifyProjectSource(path)
		add(info)
		return nil
	})
}

func walkGlobalDir(dir string, src skills.Source, add func(skills.SkillInfo)) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillFile := filepath.Join(dir, e.Name(), "SKILL.md")
		if _, err := os.Stat(skillFile); err != nil {
			continue
		}
		info, err := parseSkillFile(skillFile)
		if err != nil {
			return err
		}
		info.Source = src
		add(info)
	}
	return nil
}

func walkPluginRoot(dir string, add func(skills.SkillInfo)) error {
	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if !d.IsDir() && d.Name() == "SKILL.md" && strings.Contains(path, "/skills/") {
			info, err := parseSkillFile(path)
			if err != nil {
				return err
			}
			info.Source = skills.SourcePlugin
			add(info)
		}
		return nil
	})
}

// classifyProjectSource inspects an absolute path and returns the appropriate Source.
func classifyProjectSource(path string) skills.Source {
	switch {
	case strings.Contains(path, "/.claude/skills/"):
		return skills.SourceProjectClaude
	case strings.Contains(path, "/.agents/skills/"):
		return skills.SourceProjectAgents
	case strings.Contains(path, "/.codex/skills/"):
		return skills.SourceCodexProject
	default:
		return skills.SourceProjectClaude // fallback (unusual layout)
	}
}

// sourceRank returns higher numbers for more-specific sources.
// Used to resolve name collisions during the walk.
func sourceRank(s skills.Source) int {
	switch s {
	case skills.SourceProjectClaude, skills.SourceProjectAgents, skills.SourceCodexProject:
		return 3
	case skills.SourcePlugin:
		return 2
	case skills.SourceClaudeGlobal, skills.SourceAgentsGlobal, skills.SourceCodexGlobal:
		return 1
	}
	return 0
}
