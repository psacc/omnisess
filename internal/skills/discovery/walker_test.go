package discovery

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/psacc/omnisess/internal/skills"
)

func TestParseSkillFileExtractsFrontmatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	content := `---
name: foo
description: A short description of foo
---

# foo

Body content here.
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := parseSkillFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "foo" {
		t.Errorf("Name: got %q want %q", info.Name, "foo")
	}
	if info.DescChars != len("A short description of foo") {
		t.Errorf("DescChars: got %d want %d", info.DescChars, len("A short description of foo"))
	}
	if info.BodyBytes != int64(len(content)) {
		t.Errorf("BodyBytes: got %d want %d", info.BodyBytes, len(content))
	}
}

func TestParseSkillFileMissingFrontmatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	os.WriteFile(path, []byte("# just a body\n"), 0o644)
	info, err := parseSkillFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "" || info.DescChars != 0 {
		t.Errorf("expected empty Name/DescChars, got %+v", info)
	}
	if info.BodyBytes == 0 {
		t.Error("BodyBytes should still be set even without frontmatter")
	}
}

func TestParseSkillFileMultilineDescription(t *testing.T) {
	// YAML supports folded scalars; we treat description as the literal
	// characters between the field name and the next field/end-of-frontmatter.
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	content := `---
name: foo
description: |
  Line one
  Line two
---
body
`
	os.WriteFile(path, []byte(content), 0o644)
	info, err := parseSkillFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Implementation detail: count actual chars of the joined description.
	if info.DescChars == 0 {
		t.Error("DescChars should not be zero for multiline description")
	}
}

func TestWalkRoots_FindsProjectSkills(t *testing.T) {
	rootA := "testdata/tree/proj-a"
	rootB := "testdata/tree/proj-b"
	opts := Options{Roots: []string{rootA, rootB}, IncludeGlobals: false}
	got, err := Walk(opts)
	if err != nil {
		t.Fatal(err)
	}
	names := make(map[string]int)
	for _, s := range got {
		names[s.Name]++
	}
	// Expect alpha, beta, gamma, delta. NOT should-be-skipped, NOT
	// worktree-cloned-alpha (alpha appears once, not twice).
	for _, want := range []string{"alpha", "beta", "gamma", "delta"} {
		if names[want] == 0 {
			t.Errorf("expected to find %q, missing", want)
		}
	}
	if names["alpha"] > 1 {
		t.Errorf("worktree clone leaked: alpha appeared %d times", names["alpha"])
	}
	if names["should-be-skipped"] > 0 {
		t.Errorf("node_modules skill leaked: %v", names)
	}
}

func TestWalkClassifiesSources(t *testing.T) {
	opts := Options{Roots: []string{"testdata/tree/proj-a", "testdata/tree/proj-b"}}
	got, _ := Walk(opts)
	sourceByName := make(map[string]skills.Source)
	for _, s := range got {
		sourceByName[s.Name] = s.Source
	}
	if sourceByName["alpha"] != skills.SourceProjectClaude {
		t.Errorf("alpha source: got %v want project-claude", sourceByName["alpha"])
	}
	if sourceByName["beta"] != skills.SourceProjectAgents {
		t.Errorf("beta source: got %v", sourceByName["beta"])
	}
	if sourceByName["delta"] != skills.SourceCodexProject {
		t.Errorf("delta source: got %v", sourceByName["delta"])
	}
}

func TestWalkIncludesGlobals(t *testing.T) {
	// Use the testdata/tree/globals dir as a fake $HOME by passing custom global paths.
	opts := Options{
		Roots:          []string{},
		IncludeGlobals: true,
		GlobalPaths: GlobalPaths{
			ClaudeUser:  "testdata/tree/globals/claude/skills",
			AgentsUser:  "testdata/tree/globals/agents/skills",
			PluginsRoot: "testdata/tree/globals/plugins",
		},
	}
	got, err := Walk(opts)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, s := range got {
		names[s.Name] = true
	}
	for _, want := range []string{"epsilon", "zeta", "eta"} {
		if !names[want] {
			t.Errorf("missing global skill %q", want)
		}
	}
}
