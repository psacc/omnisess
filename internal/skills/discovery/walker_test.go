package discovery

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/psacc/omnisess/internal/skills"
)

// errReader delivers the data in its entirety on the first Read call, then
// returns a hard error (not io.EOF) on subsequent calls. This exercises
// bufio.Scanner's sc.Err() path in scanFrontmatter: the scanner receives the
// opening "---\n" line, then on the next Scan() call gets an error with no
// data, so Scan() returns false and sc.Err() is non-nil.
type errReader struct {
	data []byte
	done bool
}

func (r *errReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, errors.New("simulated read error after first read")
	}
	r.done = true
	n := copy(p, r.data)
	return n, nil
}

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

func TestWalkCollisionProjectWinsOverGlobal(t *testing.T) {
	// Build a temp tree where "alpha" exists both in a project and in a global dir.
	// Project source should win (sourceRank project > global).
	dir := t.TempDir()
	projSkill := "testdata/tree/proj-a/.claude/skills/alpha/SKILL.md" // SourceProjectClaude, rank 3
	globalDir := dir + "/claude/skills"
	globalSkillDir := globalDir + "/alpha"
	if err := os.MkdirAll(globalSkillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Write a global alpha SKILL.md with a distinct name to tell them apart.
	if err := os.WriteFile(globalSkillDir+"/SKILL.md", []byte("---\nname: alpha\ndescription: global copy\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = projSkill // referenced implicitly via rootA below
	rootA := "testdata/tree/proj-a"

	opts := Options{
		Roots:          []string{rootA},
		IncludeGlobals: true,
		GlobalPaths: GlobalPaths{
			ClaudeUser:  globalDir,
			AgentsUser:  dir + "/agents/skills",
			PluginsRoot: dir + "/plugins",
		},
	}
	got, err := Walk(opts)
	if err != nil {
		t.Fatal(err)
	}
	var found []skills.SkillInfo
	for _, s := range got {
		if s.Name == "alpha" {
			found = append(found, s)
		}
	}
	if len(found) != 1 {
		t.Fatalf("expected exactly 1 alpha after dedup, got %d: %+v", len(found), found)
	}
	if found[0].Source != skills.SourceProjectClaude {
		t.Errorf("collision winner: got source %v, want project-claude", found[0].Source)
	}
}

func TestWalkDirNameFallback(t *testing.T) {
	// SKILL.md with frontmatter but no `name:` field; directory name should be used.
	dir := t.TempDir()
	skillDir := dir + "/.claude/skills/my-tool"
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Frontmatter present but no name field.
	content := "---\ndescription: A tool without a name field\n---\nbody\n"
	if err := os.WriteFile(skillDir+"/SKILL.md", []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Walk(Options{Roots: []string{dir}, IncludeGlobals: false})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(got))
	}
	if got[0].Name != "my-tool" {
		t.Errorf("dir-name fallback: got %q, want %q", got[0].Name, "my-tool")
	}
}

func TestGlobalPathsResolve_FillsFromHOME(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	gp := GlobalPaths{} // all zero
	resolved, err := gp.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	wantClaude := fakeHome + "/.claude/skills"
	wantAgents := fakeHome + "/.agents/skills"
	wantPlugins := fakeHome + "/.claude/plugins"
	if resolved.ClaudeUser != wantClaude {
		t.Errorf("ClaudeUser = %q, want %q", resolved.ClaudeUser, wantClaude)
	}
	if resolved.AgentsUser != wantAgents {
		t.Errorf("AgentsUser = %q, want %q", resolved.AgentsUser, wantAgents)
	}
	if resolved.PluginsRoot != wantPlugins {
		t.Errorf("PluginsRoot = %q, want %q", resolved.PluginsRoot, wantPlugins)
	}
}

func TestGlobalPathsResolve_PreservesNonZero(t *testing.T) {
	// Pre-filled paths must not be overwritten.
	gp := GlobalPaths{
		ClaudeUser:  "/custom/claude",
		AgentsUser:  "/custom/agents",
		PluginsRoot: "/custom/plugins",
	}
	resolved, err := gp.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ClaudeUser != "/custom/claude" {
		t.Errorf("ClaudeUser changed: got %q", resolved.ClaudeUser)
	}
	if resolved.AgentsUser != "/custom/agents" {
		t.Errorf("AgentsUser changed: got %q", resolved.AgentsUser)
	}
	if resolved.PluginsRoot != "/custom/plugins" {
		t.Errorf("PluginsRoot changed: got %q", resolved.PluginsRoot)
	}
}

func TestWalkPluginRoot_FindsSkills(t *testing.T) {
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
	var found bool
	for _, s := range got {
		if s.Name == "eta" && s.Source == skills.SourcePlugin {
			found = true
		}
	}
	if !found {
		t.Error("expected plugin skill 'eta' with source=plugin, not found")
	}
}

func TestWalkRoot_NonExistentRootSkipped(t *testing.T) {
	// walkRoot returns nil (not an error) for a non-existent root.
	got, err := Walk(Options{Roots: []string{"/nonexistent/path/that/does/not/exist"}, IncludeGlobals: false})
	if err != nil {
		t.Errorf("Walk with nonexistent root should not error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty result for nonexistent root, got %v", got)
	}
}

func TestWalkGlobalDir_NonExistentSkipped(t *testing.T) {
	// walkGlobalDir with a nonexistent path should not error (IsNotExist is swallowed).
	opts := Options{
		Roots:          []string{},
		IncludeGlobals: true,
		GlobalPaths: GlobalPaths{
			ClaudeUser:  "/nonexistent/claude/skills",
			AgentsUser:  "/nonexistent/agents/skills",
			PluginsRoot: "/nonexistent/plugins",
		},
	}
	got, err := Walk(opts)
	if err != nil {
		t.Errorf("Walk with nonexistent globals should not error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty result, got %v", got)
	}
}

func TestSourceRank_AllValues(t *testing.T) {
	cases := []struct {
		src  skills.Source
		want int
	}{
		{skills.SourceProjectClaude, 3},
		{skills.SourceProjectAgents, 3},
		{skills.SourceCodexProject, 3},
		{skills.SourcePlugin, 2},
		{skills.SourceClaudeGlobal, 1},
		{skills.SourceAgentsGlobal, 1},
		{skills.SourceCodexGlobal, 1},
		{skills.Source("unknown-source"), 0},
	}
	for _, tc := range cases {
		got := sourceRank(tc.src)
		if got != tc.want {
			t.Errorf("sourceRank(%q) = %d, want %d", tc.src, got, tc.want)
		}
	}
}

func TestClassifyProjectSource(t *testing.T) {
	cases := []struct {
		path string
		want skills.Source
	}{
		{"/proj/.claude/skills/foo/SKILL.md", skills.SourceProjectClaude},
		{"/proj/.agents/skills/bar/SKILL.md", skills.SourceProjectAgents},
		{"/proj/.codex/skills/baz/SKILL.md", skills.SourceCodexProject},
		{"/proj/skills/qux/SKILL.md", skills.SourceProjectClaude}, // fallback
	}
	for _, tc := range cases {
		got := classifyProjectSource(tc.path)
		if got != tc.want {
			t.Errorf("classifyProjectSource(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestStripQuotes(t *testing.T) {
	cases := []struct{ in, want string }{
		{`"hello"`, "hello"},
		{`'world'`, "world"},
		{"plain", "plain"},
		{`"`, `"`},   // too short
		{"''", ""},   // empty quoted
		{"ab", "ab"}, // no quotes
	}
	for _, tc := range cases {
		got := stripQuotes(tc.in)
		if got != tc.want {
			t.Errorf("stripQuotes(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestScanFrontmatter_ScannerError(t *testing.T) {
	// errReader delivers only "---\n" (the opening delimiter) on the first Read,
	// then returns a hard error on subsequent calls. After scanning "---", the
	// scanner enters the frontmatter loop and calls Scan() again; this triggers
	// another Read which returns the error. sc.Scan() returns false with no data,
	// so sc.Err() is non-nil.
	r := &errReader{data: []byte("---\n")}
	_, _, err := scanFrontmatter(r, "test-label")
	if err == nil {
		t.Error("expected scanFrontmatter to return error from broken reader, got nil")
	}
}

func TestParseSkillFile_ScanFrontmatterError(t *testing.T) {
	// Swap scanFrontmatterFn to inject a scanner error via a real file path,
	// exercising the parseSkillFile error return at the scanFrontmatter call site.
	dir := t.TempDir()
	path := dir + "/SKILL.md"
	if err := os.WriteFile(path, []byte("---\nname: foo\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	orig := scanFrontmatterFn
	scanFrontmatterFn = func(r io.Reader, label string) (string, int, error) {
		return "", 0, errors.New("injected scanner error")
	}
	t.Cleanup(func() { scanFrontmatterFn = orig })
	_, err := parseSkillFile(path)
	if err == nil {
		t.Error("expected parseSkillFile to propagate scanFrontmatter error, got nil")
	}
}

func TestParseSkillFile_StatError(t *testing.T) {
	_, err := parseSkillFile("/nonexistent/path/SKILL.md")
	if err == nil {
		t.Error("expected error for nonexistent SKILL.md, got nil")
	}
}

func TestParseSkillFile_OpenError(t *testing.T) {
	// Create a directory named SKILL.md — os.Open succeeds but reading fails;
	// actually the easier approach: create a file then chmod 000.
	dir := t.TempDir()
	path := dir + "/SKILL.md"
	if err := os.WriteFile(path, []byte("---\nname: foo\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(path, 0o644) })
	_, err := parseSkillFile(path)
	if err == nil {
		t.Error("expected error for unreadable SKILL.md, got nil")
	}
}

func TestWalkRoot_ParseError(t *testing.T) {
	// Place an unreadable SKILL.md inside a project tree so walkRoot hits the
	// parseSkillFile error path.
	dir := t.TempDir()
	skillDir := dir + "/.claude/skills/broken-skill"
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := skillDir + "/SKILL.md"
	if err := os.WriteFile(path, []byte("---\nname: broken\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(path, 0o644) })
	_, err := Walk(Options{Roots: []string{dir}, IncludeGlobals: false})
	if err == nil {
		t.Error("expected Walk to propagate parseSkillFile error, got nil")
	}
}

func TestWalkGlobalDir_ParseError(t *testing.T) {
	// walkGlobalDir should propagate parseSkillFile error.
	dir := t.TempDir()
	skillDir := dir + "/my-skill"
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := skillDir + "/SKILL.md"
	if err := os.WriteFile(path, []byte("---\nname: my-skill\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(path, 0o644) })
	opts := Options{
		Roots:          []string{},
		IncludeGlobals: true,
		GlobalPaths: GlobalPaths{
			ClaudeUser:  dir,
			AgentsUser:  dir + "/agents",  // nonexistent — swallowed
			PluginsRoot: dir + "/plugins", // nonexistent — swallowed
		},
	}
	_, err := Walk(opts)
	if err == nil {
		t.Error("expected Walk to propagate walkGlobalDir parseSkillFile error, got nil")
	}
}

func TestWalkPluginRoot_ParseError(t *testing.T) {
	// walkPluginRoot should propagate parseSkillFile error when SKILL.md is unreadable.
	dir := t.TempDir()
	skillPath := dir + "/myplugin/skills/broken/SKILL.md"
	if err := os.MkdirAll(dir+"/myplugin/skills/broken", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillPath, []byte("---\nname: broken\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(skillPath, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(skillPath, 0o644) })
	opts := Options{
		Roots:          []string{},
		IncludeGlobals: true,
		GlobalPaths: GlobalPaths{
			ClaudeUser:  dir + "/claude", // nonexistent — swallowed
			AgentsUser:  dir + "/agents", // nonexistent — swallowed
			PluginsRoot: dir,
		},
	}
	_, err := Walk(opts)
	if err == nil {
		t.Error("expected Walk to propagate walkPluginRoot parseSkillFile error, got nil")
	}
}

func TestWalkCollisionGlobalWinsOverLowerRankGlobal(t *testing.T) {
	// Two global sources with same name; first-seen wins on tie (same rank).
	// This exercises the collision path when sourceRank(new) <= sourceRank(existing).
	dir := t.TempDir()
	claudeDir := dir + "/claude/skills/shared-skill"
	agentsDir := dir + "/agents/skills/shared-skill"
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(claudeDir+"/SKILL.md", []byte("---\nname: shared-skill\n---\nclaude\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agentsDir+"/SKILL.md", []byte("---\nname: shared-skill\n---\nagents\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := Options{
		Roots:          []string{},
		IncludeGlobals: true,
		GlobalPaths: GlobalPaths{
			ClaudeUser:  dir + "/claude/skills",
			AgentsUser:  dir + "/agents/skills",
			PluginsRoot: dir + "/plugins", // nonexistent
		},
	}
	got, err := Walk(opts)
	if err != nil {
		t.Fatal(err)
	}
	var found []skills.SkillInfo
	for _, s := range got {
		if s.Name == "shared-skill" {
			found = append(found, s)
		}
	}
	if len(found) != 1 {
		t.Fatalf("expected 1 shared-skill after dedup, got %d", len(found))
	}
	// claude-global was seen first; agents-global has same rank → claude-global wins (first-seen).
	if found[0].Source != skills.SourceClaudeGlobal {
		t.Errorf("expected SourceClaudeGlobal (first-seen), got %v", found[0].Source)
	}
}

func TestGlobalPathsResolve_UserHomeDirError(t *testing.T) {
	// Unset HOME so os.UserHomeDir returns an error.
	t.Setenv("HOME", "")
	// Also unset USERPROFILE for Windows compatibility (not needed on macOS but defensive).
	gp := GlobalPaths{} // all zero — will call os.UserHomeDir
	_, err := gp.Resolve()
	if err == nil {
		t.Error("expected error when HOME is unset, got nil")
	}
}

func TestWalk_ResolveError(t *testing.T) {
	// Resolve fails when HOME is unset and no custom GlobalPaths are provided.
	t.Setenv("HOME", "")
	opts := Options{
		Roots:          []string{},
		IncludeGlobals: true,
		GlobalPaths:    GlobalPaths{}, // all zero → Resolve will call UserHomeDir → error
	}
	_, err := Walk(opts)
	if err == nil {
		t.Error("expected Walk to return error when Resolve fails, got nil")
	}
}

func TestWalkGlobalDir_NonExistentSkillMD(t *testing.T) {
	// walkGlobalDir: subdirectory exists but has no SKILL.md → stat returns error, entry skipped.
	dir := t.TempDir()
	// A subdir with no SKILL.md.
	if err := os.MkdirAll(dir+"/empty-skill", 0o755); err != nil {
		t.Fatal(err)
	}
	// A regular file (non-dir) in the skills root — should be skipped by the !e.IsDir() check.
	if err := os.WriteFile(dir+"/README.md", []byte("# readme"), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := Options{
		Roots:          []string{},
		IncludeGlobals: true,
		GlobalPaths: GlobalPaths{
			ClaudeUser:  dir,
			AgentsUser:  dir + "/agents",  // nonexistent
			PluginsRoot: dir + "/plugins", // nonexistent
		},
	}
	got, err := Walk(opts)
	if err != nil {
		t.Errorf("expected no error when SKILL.md is absent, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 skills from empty global dir, got %v", got)
	}
}

func TestWalkGlobalDir_AgentsNonExistError(t *testing.T) {
	// walkGlobalDir for AgentsUser with a path that causes a real non-NotExist error:
	// point at an existing FILE as the dir → ReadDir returns an error that is not IsNotExist.
	dir := t.TempDir()
	// Create a file where a dir is expected.
	fileAsDir := dir + "/not-a-dir"
	if err := os.WriteFile(fileAsDir, []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	// ClaudeUser is nonexistent (IsNotExist, swallowed). AgentsUser points to a file → ReadDir errors.
	opts := Options{
		Roots:          []string{},
		IncludeGlobals: true,
		GlobalPaths: GlobalPaths{
			ClaudeUser:  dir + "/nonexistent", // IsNotExist → swallowed
			AgentsUser:  fileAsDir,            // ReadDir on a file → ENOTDIR → not IsNotExist
			PluginsRoot: dir + "/plugins",     // nonexistent
		},
	}
	_, err := Walk(opts)
	if err == nil {
		t.Error("expected Walk to propagate non-IsNotExist error from walkGlobalDir(AgentsUser), got nil")
	}
}

func TestWalkRoot_NonIsNotExistError(t *testing.T) {
	// WalkDir on an unreadable directory triggers the callback with a non-IsNotExist error.
	dir := t.TempDir()
	unreadable := dir + "/unreadable"
	if err := os.MkdirAll(unreadable, 0o755); err != nil {
		t.Fatal(err)
	}
	// Create a skills path inside for WalkDir to encounter.
	if err := os.MkdirAll(unreadable+"/.claude/skills/foo", 0o755); err != nil {
		t.Fatal(err)
	}
	// Make the inner directory unreadable so WalkDir errors when trying to enumerate it.
	if err := os.Chmod(unreadable, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(unreadable, 0o755) })
	_, err := Walk(Options{Roots: []string{unreadable}, IncludeGlobals: false})
	if err == nil {
		t.Error("expected Walk to propagate non-IsNotExist error from unreadable directory, got nil")
	}
}

func TestWalkPluginRoot_NonIsNotExistError(t *testing.T) {
	// walkPluginRoot: a file inside the plugins dir that is not IsNotExist triggers the error path.
	dir := t.TempDir()
	// Create a plugins structure with an unreadable inner directory.
	innerDir := dir + "/myplugin/skills"
	if err := os.MkdirAll(innerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(innerDir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(innerDir, 0o755) })
	opts := Options{
		Roots:          []string{},
		IncludeGlobals: true,
		GlobalPaths: GlobalPaths{
			ClaudeUser:  dir + "/claude", // nonexistent
			AgentsUser:  dir + "/agents", // nonexistent
			PluginsRoot: dir,
		},
	}
	_, err := Walk(opts)
	if err == nil {
		t.Error("expected Walk to propagate non-IsNotExist error from walkPluginRoot, got nil")
	}
}

func TestWalkCollisionPluginWinsOverGlobal(t *testing.T) {
	// Plugin source (rank 2) should replace a claude-global source (rank 1) for the
	// same skill name. Globals are walked in order: claude, agents, plugins.
	// ClaudeGlobal adds "shared" first (rank 1). Then walkPluginRoot sees "shared" with
	// rank 2 → replacement path is hit.
	dir := t.TempDir()
	claudeSkillDir := dir + "/claude/skills/shared"
	pluginSkillDir := dir + "/plugins/myplugin/skills/shared"
	if err := os.MkdirAll(claudeSkillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pluginSkillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(claudeSkillDir+"/SKILL.md", []byte("---\nname: shared\n---\nclaude-global\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pluginSkillDir+"/SKILL.md", []byte("---\nname: shared\n---\nplugin\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := Options{
		Roots:          []string{},
		IncludeGlobals: true,
		GlobalPaths: GlobalPaths{
			ClaudeUser:  dir + "/claude/skills",
			AgentsUser:  dir + "/agents", // nonexistent
			PluginsRoot: dir + "/plugins",
		},
	}
	got, err := Walk(opts)
	if err != nil {
		t.Fatal(err)
	}
	var found []skills.SkillInfo
	for _, s := range got {
		if s.Name == "shared" {
			found = append(found, s)
		}
	}
	if len(found) != 1 {
		t.Fatalf("expected 1 'shared' skill after dedup, got %d: %+v", len(found), found)
	}
	// Plugin (rank 2) beats ClaudeGlobal (rank 1) → replacement should have occurred.
	if found[0].Source != skills.SourcePlugin {
		t.Errorf("expected SourcePlugin (higher rank), got %v", found[0].Source)
	}
}
