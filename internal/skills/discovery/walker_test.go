package discovery

import (
	"os"
	"path/filepath"
	"testing"
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
