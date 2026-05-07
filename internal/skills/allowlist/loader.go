// Package allowlist loads plain-text allowlist files for the skills audit.
package allowlist

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Load reads an allowlist file. Returns the deduplicated, ordered list of
// skill names. Empty path returns an empty slice (no allowlist).
//
// Format: one skill name per line. `#` starts a comment (anywhere on the line).
// Blank lines are ignored. Whitespace is trimmed.
func Load(path string) ([]string, error) {
	if path == "" {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("allowlist: %w", err)
	}
	defer f.Close()

	seen := make(map[string]struct{})
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if _, dup := seen[line]; dup {
			continue
		}
		seen[line] = struct{}{}
		out = append(out, line)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("allowlist: %w", err)
	}
	return out, nil
}
