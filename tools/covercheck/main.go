// covercheck enforces a per-package statement coverage threshold.
//
// Usage:
//
//	go run ./tools/covercheck [flags] coverage.out
//
// Flags:
//
//	-threshold int    Minimum coverage percentage per package (default 100)
//	-exempt string    Comma-separated package path substrings to skip
//
// Coverage is computed as statement coverage: (covered statements / total statements)
// per package, parsed directly from the Go coverage profile.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

func main() {
	threshold := flag.Int("threshold", 100, "minimum coverage percentage per package (0-100)")
	exempt := flag.String("exempt", "", "comma-separated package path substrings to exempt")
	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: covercheck [flags] coverage.out")
		os.Exit(1)
	}

	exemptions := parseExemptions(*exempt)
	results, err := parse(flag.Arg(0), exemptions)
	if err != nil {
		fmt.Fprintf(os.Stderr, "covercheck: %v\n", err)
		os.Exit(1)
	}

	failed := report(results, *threshold)
	if failed {
		os.Exit(1)
	}
}

// pkgCoverage holds statement counts for one package.
type pkgCoverage struct {
	total   int
	covered int
}

// pct returns the coverage percentage (0–100).
func (p pkgCoverage) pct() float64 {
	if p.total == 0 {
		return 100.0 // no statements → vacuously covered
	}
	return float64(p.covered) / float64(p.total) * 100
}

// parse reads a Go coverage profile and returns per-package statement coverage,
// skipping any package whose import path contains an exemption substring.
func parse(path string, exemptions []string) (map[string]pkgCoverage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	pkgs := make(map[string]pkgCoverage)
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "mode:") {
			continue
		}

		// Format: github.com/foo/bar/file.go:1.2,3.4 numStmts count
		colonIdx := strings.LastIndex(line, ":")
		if colonIdx < 0 {
			continue
		}
		pkgFile := line[:colonIdx]
		rest := line[colonIdx+1:]

		// Extract package path: everything up to the last slash before the filename.
		slashIdx := strings.LastIndex(pkgFile, "/")
		if slashIdx < 0 {
			continue
		}
		pkg := pkgFile[:slashIdx]

		if isExempt(pkg, exemptions) {
			continue
		}

		// rest: "startline.col,endline.col numStmts count"
		fields := strings.Fields(rest)
		if len(fields) < 3 {
			continue
		}
		// fields[0] = "1.2,3.4", fields[1] = numStmts, fields[2] = count
		numStmts, err := strconv.Atoi(fields[len(fields)-2])
		if err != nil {
			continue
		}
		count, err := strconv.Atoi(fields[len(fields)-1])
		if err != nil {
			continue
		}

		c := pkgs[pkg]
		c.total += numStmts
		if count > 0 {
			c.covered += numStmts
		}
		pkgs[pkg] = c
	}

	return pkgs, scanner.Err()
}

// report prints coverage for all packages and returns true if any failed.
func report(pkgs map[string]pkgCoverage, threshold int) bool {
	names := make([]string, 0, len(pkgs))
	for pkg := range pkgs {
		names = append(names, pkg)
	}
	sort.Strings(names)

	failed := false
	for _, pkg := range names {
		c := pkgs[pkg]
		pct := c.pct()
		if pct < float64(threshold) {
			fmt.Printf("FAIL  %-70s %5.1f%% (need %d%%)\n", pkg, pct, threshold)
			failed = true
		} else {
			fmt.Printf("ok    %-70s %5.1f%%\n", pkg, pct)
		}
	}
	return failed
}

func parseExemptions(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// isExempt reports whether pkg should be excluded from threshold enforcement.
// An exemption matches if:
//   - pkg equals the exemption exactly (e.g. "github.com/psacc/omnisess" matches
//     only the root package, not its subpackages), OR
//   - pkg contains "/"+exemption as a path component (e.g. "gemini" matches
//     any package whose path contains a "gemini" segment).
func isExempt(pkg string, exemptions []string) bool {
	for _, e := range exemptions {
		if pkg == e {
			return true
		}
		if strings.Contains(pkg, "/"+e+"/") || strings.HasSuffix(pkg, "/"+e) {
			return true
		}
	}
	return false
}
