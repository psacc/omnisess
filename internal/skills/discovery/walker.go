// Package discovery walks filesystem trees to find agent skills.
package discovery

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/psacc/omnisess/internal/skills"
)

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

	sc := bufio.NewScanner(f)
	if !sc.Scan() || strings.TrimSpace(sc.Text()) != "---" {
		return info, nil // no frontmatter
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
					info.Name = stripQuotes(val)
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
	if err := sc.Err(); err != nil {
		return info, fmt.Errorf("scan %s: %w", path, err)
	}
	info.DescChars = len(strings.Join(descLines, " "))
	return info, nil
}

func stripQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// _ silences unused import warnings until walker uses filepath/strings.
var _ = filepath.Join
