# 014 — `omnisess skills audit`

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Status:** Proposal — design locked, ready to implement
**Created:** 2026-05-07
**Scope:** Medium (~2–3 days)
**Goal:** Add a read-only `omnisess skills audit` subcommand that classifies every discoverable agent skill as Keep / Borderline / Archive / Unknown based on observed invocation counts in Claude Code session transcripts.
**Architecture:** New `internal/skills/` package (discovery, usage, allowlist, audit, report) consumed by a new `cmd/skills.go` Cobra subcommand. No changes to existing Sources or models.
**Tech Stack:** Go 1.22+, Cobra, encoding/json, regexp, text/template. No new external dependencies.

---

## Required reading before starting

A fresh agent should read these in order before touching code:

1. `CLAUDE.md` — omnisess invariants (read-only, no CGO, source isolation, Go 1.22+)
2. `ARCHITECTURE.md` — package layout
3. `docs/process/git-workflow.md` — branch + merge rules
4. `docs/dev-harness.md` — `make check` workflow
5. `internal/source/claude/parser.go` — reference for how Claude JSONL is parsed today (we will NOT reuse it, but the structure is analogous)
6. This plan, end to end, before writing any code.

---

## Why this exists

**Problem.** A typical user session on Paolo's machine loads ~168 skills into the Claude Code system reminder. The `gws-*` (42), `recipe-*` (41), and `persona-*` (10) clusters alone account for 93 skills (≈55%). Each skill name + description costs tokens at every session start. Cost is fixed at load time, benefit only accrues on invocation. There is no native Anthropic command to measure which skills are actually used (open issue [anthropics/claude-code#35319](https://github.com/anthropics/claude-code/issues/35319) — confirmed gap as of 2026-05).

**Decision framing.** Skills are workflow-shaped (invocation-scoped), not rules-shaped (always-on). So "did you actually use it in the last 90 days" is a meaningful question. We classify and report; the human acts.

**Cross-tool prior art.** None of the major harnesses ship a usage-audit feature:

| Tool | Skill abstraction | Audit / usage telemetry |
|---|---|---|
| Anthropic Claude Code | `SKILL.md` + frontmatter | None (issue #35319 open). JSONL transcripts are parseable. |
| OpenAI Codex | Same `SKILL.md` format | None. JSONL at `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl`. Skill invocations show as `Read` tool calls into `**/skills/*/SKILL.md`. |
| Cursor | `.cursor/rules/*.mdc` | None (community hack: self-counting rule writes to `.cursor/analytics.md`). |
| Aider | Conventions file | None. Markdown chat history only. |
| Continue.dev | Prompt files w/ frontmatter | PostHog telemetry, not user-exposed. |
| Cline | Rules + Workflows split | PostHog telemetry, not user-exposed. |
| Factory.ai (Droid) | `SKILL.md` + frontmatter | OpenTelemetry export — closest peer, compliance-shaped. |
| Antigravity (Gemini) | `SKILL.md` + frontmatter | None. |

omnisess is therefore filling a real, multi-vendor gap. The first version targets Claude Code only; Codex/Cursor/Gemini are v0.5 follow-ups (parsers needed; same data model).

---

## Locked design decisions

These were resolved in a `/grill-me` session on 2026-05-07. **Do not reopen without explicit user direction.**

| # | Decision | Rationale |
|---|---|---|
| 1 | **Hosting:** integrate into omnisess as a subcommand. Not a standalone script. | Reuses Claude JSONL parsing posture; matches omnisess read-only invariant; analogous to `recap`. |
| 2 | **Counting:** parse both model-invoked (`tool_use name=Skill, input.skill=X`) and user-invoked (`<command-name>/X</command-name>`) signals. Keep the two counts as **distinct columns** in output; sum for tiering. | Skill-tool-only undercounts by a known-large factor (probed: 9 slash vs 1 Skill-tool in one real session). |
| 3 | **Discovery interface:** `--root <path>` repeatable flag, no defaults for personal paths. Globals on by default (`~/.claude/skills/`, `~/.agents/skills/`, `~/.claude/plugins/**/skills/`). `--no-globals` to opt out. Always-on excludes: `**/.worktrees/**`, `**/worktrees/**`, `**/node_modules/**`, `**/.venv/**`, `**/.git/**`, `**/dist/**`, `**/build/**`, `**/.scratch/**`. | omnisess is a public tool — no hardcoded user paths. Globals are documented Claude Code conventions, not personal paths. Worktree clones explain finn's 1009-vs-150 SKILL.md inflation. |
| 4 | **Subcommand shape:** namespace-style `omnisess skills audit`. | Future-proofs `omnisess skills list`, `omnisess skills usage`, etc. (v0.5+). |
| 5 | **Output format:** markdown to stdout by default; `--format json` for machines; `-o <path>` to redirect. No TSV. | Markdown is the human-read deliverable; JSON is the machine-read substrate; `jq -r` covers any tabular need. |
| 6 | **Tiering:** pure usage-count tiers, 90-day window default, `--window 30d\|90d\|180d` configurable. **Cost is informational only**, not a tier driver. Tiers: `Keep` (`total_90d ≥ 3` OR allowlisted), `Borderline` (`1–2`), `Archive` (`0` AND not allowlisted), `Unknown` (telemetry-incomplete sources, see #9). | Usage variance dominates cost variance; allowlist absorbs the long-tail-but-critical case (e.g. `security-review`). |
| 7 | **Allowlist:** plain text, exact match, `--allowlist <path>` flag, no default location. Format: one skill name per line, `#` for comments, blank lines ignored. Unmatched entries emitted as warnings in the report. | Public tool: no surprise default paths. Plain text plays well with version control; richer formats are YAGNI for v0. |
| 8 | **Read-only:** v0 never modifies skills, session files, allowlist, git, or anything else. The Archive tier is a recommendation, not an action. | Matches omnisess invariant. One-way doors are off the table for a quarterly audit. |
| 9 | **Telemetry scope:** Claude Code sessions only in v0 (`~/.claude/projects/**/*.jsonl`). Discovered Codex/Cursor/Gemini/Antigravity skills appear in inventory but force-tier as `Unknown`, never `Archive`. | Avoids the worst failure mode (recommending deletion of a skill used in another harness). v0.5 adds Codex; depends on omnisess Codex Source landing first. |
| 10 | **Process:** TDD per `superpowers:test-driven-development` skill; exec plan (this file); branch `feature/skills-audit-v0`; `make check` must pass; PR review per omnisess `git-workflow.md`. Validation via three parallel subagents (see Validation section). | Matches omnisess engineering culture. |

---

## Architecture

### File structure

```
omnisess/
├── cmd/
│   ├── skills.go                # parent `skills` Cobra command (registers subcommands)
│   └── skills_audit.go          # `audit` subcommand wiring (flags → audit pipeline)
├── internal/
│   └── skills/
│       ├── types.go             # SkillInfo, Invocation, Tier, Report, AuditResult
│       ├── discovery/
│       │   ├── walker.go        # walks --root + globals, returns []SkillInfo
│       │   ├── walker_test.go
│       │   └── testdata/
│       │       └── tree/        # synthetic skill-tree fixture
│       ├── usage/
│       │   ├── scanner.go       # scans ~/.claude/projects/**/*.jsonl, returns []Invocation
│       │   ├── scanner_test.go
│       │   └── testdata/
│       │       └── *.jsonl      # synthetic session fixtures (model + user invocations)
│       ├── allowlist/
│       │   ├── loader.go        # parses plain-text allowlist file
│       │   └── loader_test.go
│       ├── audit/
│       │   ├── tier.go          # joins inventory + usage + allowlist → AuditResult
│       │   └── tier_test.go
│       └── report/
│           ├── markdown.go      # AuditResult → markdown
│           ├── markdown_test.go
│           ├── json.go          # AuditResult → JSON
│           └── json_test.go
└── docs/
    └── skills-audit.md          # user-facing doc (cross-tool comparison + usage)
```

### Why a new package, not reusing `internal/source/claude`

- `Source` interface returns hydrated `Session` objects (full message bodies). Audit only needs invocation events with timestamps — different shape, different perf profile.
- `Source.List()` is meant for interactive `list/search/show`. Audit is bulk aggregation across all sessions.
- omnisess invariant: "Source packages NEVER import each other." `internal/skills/` is **not** a Source — it's analytics. It imports `internal/model` for shared timestamp/role types (if needed) but not any Source package.
- Existing `extractToolCalls` in `internal/source/claude/parser.go` truncates `Input` at 200 chars. Fine for our use (skill names are short), but coupling audit to that limit is fragile. Independent scanner is cleaner.

### Data flow

```
[--root paths + globals]                       [~/.claude/projects/**/*.jsonl]
            │                                                │
            ▼                                                ▼
  discovery.Walk()                                  usage.Scan()
  → []SkillInfo                                     → []Invocation
            │                                                │
            └──────────────┬─────────────────────────────────┘
                           ▼
            [optional: allowlist.Load(--allowlist)]
                           │
                           ▼
                   audit.Tier(skills, invs, allowlist, window)
                   → AuditResult
                           │
              ┌────────────┴────────────┐
              ▼                         ▼
     report.Markdown()          report.JSON()
        → io.Writer                → io.Writer
```

### Core types (locked)

```go
// internal/skills/types.go

package skills

import "time"

type Source string

const (
    SourceClaudeGlobal     Source = "claude-global"      // ~/.claude/skills/
    SourceAgentsGlobal     Source = "agents-global"      // ~/.agents/skills/
    SourcePlugin           Source = "plugin"             // ~/.claude/plugins/**/skills/
    SourceProjectClaude    Source = "project-claude"     // <root>/**/.claude/skills/
    SourceProjectAgents    Source = "project-agents"     // <root>/**/.agents/skills/
    SourceCodexProject     Source = "codex-project"      // <root>/**/.codex/skills/
    SourceCodexGlobal      Source = "codex-global"       // ~/.codex/skills/
)

type SkillInfo struct {
    Name         string    // skill name (directory name)
    Path         string    // absolute path to SKILL.md
    Source       Source
    DescChars    int       // length of `description` frontmatter field (0 if missing)
    BodyBytes    int64     // size of SKILL.md in bytes
    LastModified time.Time // file mtime
}

type Invocation struct {
    SkillName string
    Timestamp time.Time
    Kind      InvocationKind
    SessionID string
}

type InvocationKind string

const (
    InvocationModel InvocationKind = "model" // tool_use name=Skill, input.skill=X
    InvocationUser  InvocationKind = "user"  // <command-name>/X</command-name>
)

type Tier string

const (
    TierKeep       Tier = "Keep"
    TierBorderline Tier = "Borderline"
    TierArchive    Tier = "Archive"
    TierUnknown    Tier = "Unknown" // telemetry-incomplete (Codex/etc.)
)

type SkillAudit struct {
    Skill           SkillInfo
    Tier            Tier
    ModelInvoked    int       // count in window
    UserInvoked     int       // count in window
    Total           int       // = ModelInvoked + UserInvoked
    LastUsed        time.Time // zero value if never used
    Allowlisted     bool
}

type AuditResult struct {
    Window           time.Duration
    GeneratedAt      time.Time
    Roots            []string
    GlobalsIncluded  bool
    SessionsParsed   int
    Skills           []SkillAudit  // sorted by Tier then descending Total
    GhostUsage       []string      // skill names invoked but not discovered
    Collisions       map[string][]string // skill name → list of paths
    UnmatchedAllow   []string      // allowlist entries not matching any skill
    OmnisessVersion  string
}
```

---

## Implementation tasks (TDD checkbox format)

> Each task: write failing test, run it (expect FAIL), implement, run test (expect PASS), commit. Use `superpowers:test-driven-development` discipline. Commit after each green task — small commits, not bundled.

### Task 0: Branch and exec plan check-in

- [ ] **Step 1: Create feature branch from main**

```bash
cd ~/prj/psacc/omnisess
git checkout main && git pull
git checkout -b feature/skills-audit-v0
```

- [ ] **Step 2: Verify exec plan exists**

This file (`docs/exec-plans/active/014-skills-audit.md`) is already on main. Confirm:

```bash
ls docs/exec-plans/active/014-skills-audit.md
```

Expected: file present.

- [ ] **Step 3: Create empty package directories with `.keep` placeholders so the layout is reviewable in early commits**

Files to create as empty (zero-byte) `.keep` markers:

```
internal/skills/.keep
internal/skills/discovery/.keep
internal/skills/usage/.keep
internal/skills/usage/testdata/.keep
internal/skills/allowlist/.keep
internal/skills/audit/.keep
internal/skills/report/.keep
```

```bash
mkdir -p internal/skills/{discovery,usage/testdata,allowlist,audit,report}
touch internal/skills/{,discovery/,usage/,usage/testdata/,allowlist/,audit/,report/}.keep
git add internal/skills/
git commit -m "chore(skills): scaffold internal/skills package layout"
```

---

### Task 1: Core types

**Files:**
- Create: `internal/skills/types.go`

- [ ] **Step 1: Write the failing test**

Create `internal/skills/types_test.go`:

```go
package skills

import (
    "encoding/json"
    "testing"
    "time"
)

func TestTierStringRoundTrip(t *testing.T) {
    cases := []Tier{TierKeep, TierBorderline, TierArchive, TierUnknown}
    for _, c := range cases {
        b, err := json.Marshal(c)
        if err != nil {
            t.Fatalf("marshal %v: %v", c, err)
        }
        var got Tier
        if err := json.Unmarshal(b, &got); err != nil {
            t.Fatalf("unmarshal %v: %v", c, err)
        }
        if got != c {
            t.Errorf("round trip: got %v want %v", got, c)
        }
    }
}

func TestSkillAuditTotalIsSum(t *testing.T) {
    sa := SkillAudit{ModelInvoked: 3, UserInvoked: 5}
    sa.Total = sa.ModelInvoked + sa.UserInvoked
    if sa.Total != 8 {
        t.Errorf("Total: got %d want 8", sa.Total)
    }
}

func TestInvocationKindValues(t *testing.T) {
    if InvocationModel == InvocationUser {
        t.Fatal("kinds must be distinct")
    }
    if string(InvocationModel) != "model" || string(InvocationUser) != "user" {
        t.Errorf("InvocationKind values changed; check JSON contracts")
    }
}

func TestAuditResultZeroValueIsValid(t *testing.T) {
    var r AuditResult
    if r.GeneratedAt != (time.Time{}) {
        t.Errorf("zero AuditResult should have zero time")
    }
    if len(r.Skills) != 0 {
        t.Errorf("zero AuditResult should have empty Skills")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/skills/ -run 'TestTier|TestSkill|TestInvocation|TestAuditResult'
```

Expected: FAIL — types not defined.

- [ ] **Step 3: Write the types**

Create `internal/skills/types.go` with the exact contents specified in the **Core types (locked)** section above.

- [ ] **Step 4: Run tests**

```bash
go test ./internal/skills/...
```

Expected: PASS.

- [ ] **Step 5: `make check` and commit**

```bash
make check
git add internal/skills/types.go internal/skills/types_test.go
git rm internal/skills/.keep
git commit -m "feat(skills): add core types (SkillInfo, Invocation, Tier, AuditResult)"
```

---

### Task 2: Allowlist loader

**Files:**
- Create: `internal/skills/allowlist/loader.go`
- Create: `internal/skills/allowlist/loader_test.go`

- [ ] **Step 1: Write the failing test**

```go
package allowlist

import (
    "os"
    "path/filepath"
    "testing"
)

func TestLoadParsesPlainText(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "allow.txt")
    content := `# top comment
security-review        # inline comment
debugging

# blank line above
incident-response
`
    if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
        t.Fatal(err)
    }
    got, err := Load(path)
    if err != nil {
        t.Fatalf("Load: %v", err)
    }
    want := []string{"security-review", "debugging", "incident-response"}
    if len(got) != len(want) {
        t.Fatalf("got %v want %v", got, want)
    }
    for i := range want {
        if got[i] != want[i] {
            t.Errorf("entry %d: got %q want %q", i, got[i], want[i])
        }
    }
}

func TestLoadEmptyPathReturnsEmpty(t *testing.T) {
    got, err := Load("")
    if err != nil {
        t.Fatalf("Load empty path: %v", err)
    }
    if len(got) != 0 {
        t.Errorf("expected empty, got %v", got)
    }
}

func TestLoadMissingFileReturnsError(t *testing.T) {
    _, err := Load("/nonexistent/path/allow.txt")
    if err == nil {
        t.Error("expected error for missing file")
    }
}

func TestLoadDuplicatesDeduped(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "allow.txt")
    os.WriteFile(path, []byte("foo\nbar\nfoo\n"), 0o644)
    got, _ := Load(path)
    if len(got) != 2 {
        t.Errorf("expected 2 deduped entries, got %v", got)
    }
}
```

- [ ] **Step 2: Run test, verify FAIL**

```bash
go test ./internal/skills/allowlist/
```

Expected: FAIL — `Load` undefined.

- [ ] **Step 3: Implement loader**

Create `internal/skills/allowlist/loader.go`:

```go
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
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/skills/allowlist/
```

Expected: PASS.

- [ ] **Step 5: `make check` and commit**

```bash
make check
git add internal/skills/allowlist/
git rm internal/skills/allowlist/.keep
git commit -m "feat(skills): add allowlist loader (plain text, dedup)"
```

---

### Task 3: Discovery walker — frontmatter parsing

**Files:**
- Create: `internal/skills/discovery/walker.go`
- Create: `internal/skills/discovery/walker_test.go`
- Create: `internal/skills/discovery/testdata/tree/<...>` (fixture)

This task is split into two TDD cycles — first frontmatter parsing of a single SKILL.md, then full directory walking. Keep them as separate commits.

- [ ] **Step 1: Write the failing test for frontmatter parsing**

```go
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
```

- [ ] **Step 2: Run test, verify FAIL**

```bash
go test ./internal/skills/discovery/
```

Expected: FAIL — `parseSkillFile` undefined.

- [ ] **Step 3: Implement frontmatter parsing**

Create `internal/skills/discovery/walker.go` with parsing-only logic first:

```go
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
```

- [ ] **Step 4: Run tests, verify PASS**

```bash
go test ./internal/skills/discovery/
```

Expected: PASS.

- [ ] **Step 5: `make check` and commit**

```bash
make check
git add internal/skills/discovery/
git commit -m "feat(skills): parse SKILL.md frontmatter (name, description)"
```

---

### Task 4: Discovery walker — directory traversal + classification

**Files:**
- Modify: `internal/skills/discovery/walker.go`
- Modify: `internal/skills/discovery/walker_test.go`
- Create: `internal/skills/discovery/testdata/tree/...` fixture

- [ ] **Step 1: Build the synthetic fixture tree**

Create the fixture by running:

```bash
cd internal/skills/discovery/testdata/tree
mkdir -p \
  proj-a/.claude/skills/alpha \
  proj-a/.agents/skills/beta \
  proj-a/.worktrees/wt1/.claude/skills/alpha \
  proj-a/node_modules/something/.claude/skills/should-be-skipped \
  proj-b/.claude/skills/gamma \
  proj-b/.codex/skills/delta \
  globals/claude/skills/epsilon \
  globals/agents/skills/zeta \
  globals/plugins/myplugin/skills/eta

for d in proj-a/.claude/skills/alpha proj-a/.agents/skills/beta proj-b/.claude/skills/gamma proj-b/.codex/skills/delta globals/claude/skills/epsilon globals/agents/skills/zeta globals/plugins/myplugin/skills/eta; do
  cat > "$d/SKILL.md" <<EOF
---
name: $(basename $d)
description: Test skill $(basename $d)
---
body
EOF
done
# Worktree clone (should be excluded by walker)
cp proj-a/.claude/skills/alpha/SKILL.md proj-a/.worktrees/wt1/.claude/skills/alpha/SKILL.md
# node_modules fixture (should be excluded)
cat > proj-a/node_modules/something/.claude/skills/should-be-skipped/SKILL.md <<'EOF'
---
name: should-be-skipped
description: vendored, do not include
---
EOF
```

- [ ] **Step 2: Write the failing walker test**

Append to `internal/skills/discovery/walker_test.go`:

```go
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
            ClaudeUser:    "testdata/tree/globals/claude/skills",
            AgentsUser:    "testdata/tree/globals/agents/skills",
            PluginsRoot:   "testdata/tree/globals/plugins",
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
```

- [ ] **Step 3: Run test, verify FAIL**

```bash
go test ./internal/skills/discovery/ -run TestWalk
```

Expected: FAIL — `Walk`, `Options`, `GlobalPaths` undefined.

- [ ] **Step 4: Implement walker**

Append to `internal/skills/discovery/walker.go`:

```go
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
```

Remove the `var _ = filepath.Join` placeholder line from the previous task — `filepath` is now used.

- [ ] **Step 5: Run tests**

```bash
go test ./internal/skills/discovery/
```

Expected: PASS for all `TestWalk*` cases.

- [ ] **Step 6: `make check` and commit**

```bash
make check
git add internal/skills/discovery/
git rm internal/skills/discovery/.keep
git commit -m "feat(skills): walk roots + globals, classify source, dedup collisions"
```

---

### Task 5: Usage scanner — JSONL parsing

**Files:**
- Create: `internal/skills/usage/scanner.go`
- Create: `internal/skills/usage/scanner_test.go`
- Create: `internal/skills/usage/testdata/sample.jsonl`

**Reference shapes** (confirmed against real session JSONL on 2026-05-07):

```jsonc
// Model invocation (assistant message, tool_use)
{"type":"assistant","message":{"role":"assistant","content":[
  {"type":"tool_use","id":"toolu_...","name":"Skill","input":{"skill":"agent-slack"}}
]},"timestamp":"2026-05-07T10:23:11.123Z","sessionId":"abc-123",...}

// User invocation (user message, command-name marker in content)
{"type":"user","message":{"role":"user","content":"<command-name>/grill-me</command-name>\n            <command-message>grill-me</command-message>\n            <command-args></command-args>"},"timestamp":"2026-05-07T10:24:00.000Z","sessionId":"abc-123",...}
```

- [ ] **Step 1: Create the JSONL fixture**

Create `internal/skills/usage/testdata/sample.jsonl`:

```jsonl
{"type":"user","message":{"role":"user","content":"<command-name>/grill-me</command-name>\n<command-message>grill-me</command-message>"},"timestamp":"2026-05-01T08:00:00.000Z","sessionId":"sess-1"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Skill","input":{"skill":"agent-slack"}}]},"timestamp":"2026-05-01T08:00:05.000Z","sessionId":"sess-1"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Read","input":{"file_path":"/etc/passwd"}}]},"timestamp":"2026-05-01T08:00:10.000Z","sessionId":"sess-1"}
{"type":"user","message":{"role":"user","content":"<command-name>/figma:figma-use</command-name>"},"timestamp":"2026-05-01T08:01:00.000Z","sessionId":"sess-1"}
{"type":"user","message":{"role":"user","content":"plain text, no command"},"timestamp":"2026-05-01T08:02:00.000Z","sessionId":"sess-1"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Skill","input":{"skill":"calendar"}}]},"timestamp":"2026-05-02T09:00:00.000Z","sessionId":"sess-2"}
```

- [ ] **Step 2: Write the failing test**

```go
package usage

import (
    "testing"
    "time"

    "github.com/psacc/omnisess/internal/skills"
)

func TestScanFileExtractsBothInvocationKinds(t *testing.T) {
    got, err := scanFile("testdata/sample.jsonl")
    if err != nil {
        t.Fatal(err)
    }
    var model, user int
    for _, inv := range got {
        switch inv.Kind {
        case skills.InvocationModel:
            model++
        case skills.InvocationUser:
            user++
        }
    }
    if model != 2 {
        t.Errorf("model invocations: got %d want 2", model)
    }
    if user != 2 {
        t.Errorf("user invocations: got %d want 2", user)
    }
}

func TestScanFileSkillNames(t *testing.T) {
    got, _ := scanFile("testdata/sample.jsonl")
    names := map[string]int{}
    for _, inv := range got {
        names[inv.SkillName]++
    }
    expected := map[string]int{
        "grill-me":      1,
        "agent-slack":   1,
        "figma:figma-use": 1,
        "calendar":      1,
    }
    for name, want := range expected {
        if names[name] != want {
            t.Errorf("skill %q count: got %d want %d (all: %v)", name, names[name], want, names)
        }
    }
    if _, leaked := names["Read"]; leaked {
        t.Error("Read tool_use should not be counted as a skill invocation")
    }
}

func TestScanFileTimestampsParse(t *testing.T) {
    got, _ := scanFile("testdata/sample.jsonl")
    if len(got) == 0 {
        t.Fatal("no invocations")
    }
    for _, inv := range got {
        if inv.Timestamp.IsZero() {
            t.Errorf("zero timestamp on %+v", inv)
        }
    }
}

func TestScanWindowFiltersBefore(t *testing.T) {
    cutoff := time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC)
    got, err := Scan(ScanOptions{
        Files:  []string{"testdata/sample.jsonl"},
        Since:  cutoff,
    })
    if err != nil {
        t.Fatal(err)
    }
    for _, inv := range got {
        if inv.Timestamp.Before(cutoff) {
            t.Errorf("invocation %+v before cutoff %v", inv, cutoff)
        }
    }
    // Only the 2026-05-02 calendar invocation should remain.
    if len(got) != 1 {
        t.Errorf("got %d invocations, want 1", len(got))
    }
}
```

- [ ] **Step 3: Run test, verify FAIL**

```bash
go test ./internal/skills/usage/
```

Expected: FAIL — `scanFile`, `Scan`, `ScanOptions` undefined.

- [ ] **Step 4: Implement scanner**

Create `internal/skills/usage/scanner.go`:

```go
// Package usage scans Claude Code session JSONL transcripts for skill invocations.
package usage

import (
    "bufio"
    "encoding/json"
    "fmt"
    "os"
    "regexp"
    "time"

    "github.com/psacc/omnisess/internal/skills"
)

// commandNameRE matches a slash-command invocation. Skill names may contain
// alphanumerics, hyphens, underscores, and one optional plugin colon prefix.
var commandNameRE = regexp.MustCompile(`<command-name>/([A-Za-z0-9_:-]+)</command-name>`)

// ScanOptions configures Scan.
type ScanOptions struct {
    Files []string  // explicit list of JSONL files to scan
    Since time.Time // discard invocations strictly before this timestamp; zero = no filter
}

// Scan reads all JSONL files and returns merged invocations matching options.
func Scan(opts ScanOptions) ([]skills.Invocation, error) {
    var out []skills.Invocation
    for _, f := range opts.Files {
        invs, err := scanFile(f)
        if err != nil {
            return nil, err
        }
        for _, inv := range invs {
            if !opts.Since.IsZero() && inv.Timestamp.Before(opts.Since) {
                continue
            }
            out = append(out, inv)
        }
    }
    return out, nil
}

type rawLine struct {
    Type      string          `json:"type"`
    Message   json.RawMessage `json:"message"`
    Timestamp string          `json:"timestamp"`
    SessionID string          `json:"sessionId"`
}

type rawAssistantMsg struct {
    Content []rawContentBlock `json:"content"`
}

type rawContentBlock struct {
    Type  string          `json:"type"`
    Name  string          `json:"name"`
    Input json.RawMessage `json:"input"`
}

type rawUserMsg struct {
    Content interface{} `json:"content"` // may be string or array
}

type skillInput struct {
    Skill string `json:"skill"`
}

func scanFile(path string) ([]skills.Invocation, error) {
    f, err := os.Open(path)
    if err != nil {
        return nil, fmt.Errorf("open %s: %w", path, err)
    }
    defer f.Close()

    var out []skills.Invocation
    sc := bufio.NewScanner(f)
    sc.Buffer(make([]byte, 64*1024), 16*1024*1024) // sessions can have large lines
    for sc.Scan() {
        var line rawLine
        if err := json.Unmarshal(sc.Bytes(), &line); err != nil {
            continue // skip malformed lines silently (real sessions sometimes have control chars)
        }
        ts := parseTimestamp(line.Timestamp)
        switch line.Type {
        case "assistant":
            var msg rawAssistantMsg
            if err := json.Unmarshal(line.Message, &msg); err != nil {
                continue
            }
            for _, b := range msg.Content {
                if b.Type != "tool_use" || b.Name != "Skill" {
                    continue
                }
                var inp skillInput
                if err := json.Unmarshal(b.Input, &inp); err != nil {
                    continue
                }
                if inp.Skill == "" {
                    continue
                }
                out = append(out, skills.Invocation{
                    SkillName: inp.Skill,
                    Timestamp: ts,
                    Kind:      skills.InvocationModel,
                    SessionID: line.SessionID,
                })
            }
        case "user":
            var msg rawUserMsg
            if err := json.Unmarshal(line.Message, &msg); err != nil {
                continue
            }
            content, ok := msg.Content.(string)
            if !ok {
                continue
            }
            for _, m := range commandNameRE.FindAllStringSubmatch(content, -1) {
                out = append(out, skills.Invocation{
                    SkillName: m[1],
                    Timestamp: ts,
                    Kind:      skills.InvocationUser,
                    SessionID: line.SessionID,
                })
            }
        }
    }
    if err := sc.Err(); err != nil {
        return nil, fmt.Errorf("scan %s: %w", path, err)
    }
    return out, nil
}

func parseTimestamp(s string) time.Time {
    if s == "" {
        return time.Time{}
    }
    for _, layout := range []string{
        time.RFC3339Nano,
        time.RFC3339,
        "2006-01-02T15:04:05.000Z",
        "2006-01-02T15:04:05Z",
    } {
        if t, err := time.Parse(layout, s); err == nil {
            return t
        }
    }
    return time.Time{}
}

// FindSessionFiles returns all *.jsonl files under ~/.claude/projects.
func FindSessionFiles(projectsRoot string) ([]string, error) {
    entries, err := os.ReadDir(projectsRoot)
    if err != nil {
        return nil, err
    }
    var out []string
    for _, e := range entries {
        if !e.IsDir() {
            continue
        }
        sub := projectsRoot + "/" + e.Name()
        files, err := os.ReadDir(sub)
        if err != nil {
            continue
        }
        for _, f := range files {
            if !f.IsDir() && len(f.Name()) > 6 && f.Name()[len(f.Name())-6:] == ".jsonl" {
                out = append(out, sub+"/"+f.Name())
            }
        }
    }
    return out, nil
}
```

- [ ] **Step 5: Run tests, verify PASS**

```bash
go test ./internal/skills/usage/
```

Expected: PASS.

- [ ] **Step 6: `make check` and commit**

```bash
make check
git add internal/skills/usage/
git rm internal/skills/usage/.keep internal/skills/usage/testdata/.keep
git commit -m "feat(skills): scan JSONL sessions for Skill tool_use and slash invocations"
```

---

### Task 6: Audit tiering

**Files:**
- Create: `internal/skills/audit/tier.go`
- Create: `internal/skills/audit/tier_test.go`

- [ ] **Step 1: Write the failing test**

```go
package audit

import (
    "testing"
    "time"

    "github.com/psacc/omnisess/internal/skills"
)

func newSkill(name string, src skills.Source) skills.SkillInfo {
    return skills.SkillInfo{Name: name, Source: src, DescChars: 100, BodyBytes: 1000, LastModified: time.Now()}
}

func newInv(name, kind string, t time.Time) skills.Invocation {
    return skills.Invocation{SkillName: name, Timestamp: t, Kind: skills.InvocationKind(kind)}
}

func TestTier_Keep_3OrMore(t *testing.T) {
    now := time.Now()
    inv := []skills.Invocation{
        newInv("foo", "model", now),
        newInv("foo", "user", now),
        newInv("foo", "model", now),
    }
    res := Tier(TierInput{
        Skills:      []skills.SkillInfo{newSkill("foo", skills.SourceClaudeGlobal)},
        Invocations: inv,
        Window:      90 * 24 * time.Hour,
        Now:         now,
    })
    if res.Skills[0].Tier != skills.TierKeep {
        t.Errorf("got %v want Keep", res.Skills[0].Tier)
    }
    if res.Skills[0].Total != 3 {
        t.Errorf("Total: got %d want 3", res.Skills[0].Total)
    }
}

func TestTier_Borderline_1or2(t *testing.T) {
    now := time.Now()
    res := Tier(TierInput{
        Skills:      []skills.SkillInfo{newSkill("foo", skills.SourceClaudeGlobal)},
        Invocations: []skills.Invocation{newInv("foo", "model", now)},
        Window:      90 * 24 * time.Hour,
        Now:         now,
    })
    if res.Skills[0].Tier != skills.TierBorderline {
        t.Errorf("got %v want Borderline", res.Skills[0].Tier)
    }
}

func TestTier_Archive_zero(t *testing.T) {
    now := time.Now()
    res := Tier(TierInput{
        Skills:      []skills.SkillInfo{newSkill("foo", skills.SourceClaudeGlobal)},
        Invocations: nil,
        Window:      90 * 24 * time.Hour,
        Now:         now,
    })
    if res.Skills[0].Tier != skills.TierArchive {
        t.Errorf("got %v want Archive", res.Skills[0].Tier)
    }
}

func TestTier_Allowlist_overrides_archive(t *testing.T) {
    now := time.Now()
    res := Tier(TierInput{
        Skills:      []skills.SkillInfo{newSkill("foo", skills.SourceClaudeGlobal)},
        Invocations: nil,
        Allowlist:   []string{"foo"},
        Window:      90 * 24 * time.Hour,
        Now:         now,
    })
    if res.Skills[0].Tier != skills.TierKeep {
        t.Errorf("got %v want Keep (allowlisted)", res.Skills[0].Tier)
    }
    if !res.Skills[0].Allowlisted {
        t.Error("Allowlisted should be true")
    }
}

func TestTier_Codex_forced_unknown(t *testing.T) {
    now := time.Now()
    res := Tier(TierInput{
        Skills:      []skills.SkillInfo{newSkill("foo", skills.SourceCodexProject)},
        Invocations: nil,
        Window:      90 * 24 * time.Hour,
        Now:         now,
    })
    if res.Skills[0].Tier != skills.TierUnknown {
        t.Errorf("got %v want Unknown for codex source", res.Skills[0].Tier)
    }
}

func TestTier_OutsideWindow_notCounted(t *testing.T) {
    now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
    old := now.Add(-200 * 24 * time.Hour)
    res := Tier(TierInput{
        Skills:      []skills.SkillInfo{newSkill("foo", skills.SourceClaudeGlobal)},
        Invocations: []skills.Invocation{newInv("foo", "model", old)},
        Window:      90 * 24 * time.Hour,
        Now:         now,
    })
    if res.Skills[0].Tier != skills.TierArchive {
        t.Errorf("got %v want Archive (old invocation outside window)", res.Skills[0].Tier)
    }
}

func TestTier_GhostUsage_recorded(t *testing.T) {
    now := time.Now()
    res := Tier(TierInput{
        Skills:      []skills.SkillInfo{newSkill("foo", skills.SourceClaudeGlobal)},
        Invocations: []skills.Invocation{newInv("missing-skill", "model", now)},
        Window:      90 * 24 * time.Hour,
        Now:         now,
    })
    found := false
    for _, g := range res.GhostUsage {
        if g == "missing-skill" {
            found = true
        }
    }
    if !found {
        t.Errorf("missing-skill should appear in GhostUsage; got %v", res.GhostUsage)
    }
}

func TestTier_UnmatchedAllowlist_recorded(t *testing.T) {
    now := time.Now()
    res := Tier(TierInput{
        Skills:    []skills.SkillInfo{newSkill("foo", skills.SourceClaudeGlobal)},
        Allowlist: []string{"foo", "ghost-allow"},
        Window:    90 * 24 * time.Hour,
        Now:       now,
    })
    if len(res.UnmatchedAllow) != 1 || res.UnmatchedAllow[0] != "ghost-allow" {
        t.Errorf("UnmatchedAllow: got %v want [ghost-allow]", res.UnmatchedAllow)
    }
}
```

- [ ] **Step 2: Run test, verify FAIL**

```bash
go test ./internal/skills/audit/
```

Expected: FAIL.

- [ ] **Step 3: Implement Tier**

Create `internal/skills/audit/tier.go`:

```go
// Package audit joins skill inventory + usage + allowlist into a tiered audit result.
package audit

import (
    "sort"
    "time"

    "github.com/psacc/omnisess/internal/skills"
)

// TierInput is the input to Tier.
type TierInput struct {
    Skills      []skills.SkillInfo
    Invocations []skills.Invocation
    Allowlist   []string
    Window      time.Duration
    Now         time.Time
}

// Tier classifies each skill and records ghost usage / unmatched allowlist entries.
func Tier(in TierInput) skills.AuditResult {
    cutoff := in.Now.Add(-in.Window)

    // Index skills by name.
    byName := make(map[string]skills.SkillInfo, len(in.Skills))
    for _, s := range in.Skills {
        byName[s.Name] = s
    }

    allowSet := make(map[string]struct{}, len(in.Allowlist))
    for _, a := range in.Allowlist {
        allowSet[a] = struct{}{}
    }

    type counts struct {
        model, user int
        last        time.Time
    }
    invCounts := make(map[string]*counts)
    var ghosts []string
    seenGhost := map[string]struct{}{}

    for _, inv := range in.Invocations {
        if inv.Timestamp.Before(cutoff) {
            continue
        }
        if _, known := byName[inv.SkillName]; !known {
            if _, dup := seenGhost[inv.SkillName]; !dup {
                ghosts = append(ghosts, inv.SkillName)
                seenGhost[inv.SkillName] = struct{}{}
            }
            continue
        }
        c, ok := invCounts[inv.SkillName]
        if !ok {
            c = &counts{}
            invCounts[inv.SkillName] = c
        }
        switch inv.Kind {
        case skills.InvocationModel:
            c.model++
        case skills.InvocationUser:
            c.user++
        }
        if inv.Timestamp.After(c.last) {
            c.last = inv.Timestamp
        }
    }

    var auditRows []skills.SkillAudit
    for _, s := range in.Skills {
        c := invCounts[s.Name]
        var row skills.SkillAudit
        row.Skill = s
        if c != nil {
            row.ModelInvoked = c.model
            row.UserInvoked = c.user
            row.LastUsed = c.last
        }
        row.Total = row.ModelInvoked + row.UserInvoked
        _, allowed := allowSet[s.Name]
        row.Allowlisted = allowed

        switch {
        case isUnknownSource(s.Source):
            row.Tier = skills.TierUnknown
        case allowed:
            row.Tier = skills.TierKeep
        case row.Total >= 3:
            row.Tier = skills.TierKeep
        case row.Total >= 1:
            row.Tier = skills.TierBorderline
        default:
            row.Tier = skills.TierArchive
        }
        auditRows = append(auditRows, row)
    }

    sort.SliceStable(auditRows, func(i, j int) bool {
        ti, tj := tierOrder(auditRows[i].Tier), tierOrder(auditRows[j].Tier)
        if ti != tj {
            return ti < tj
        }
        return auditRows[i].Total > auditRows[j].Total
    })

    var unmatched []string
    for _, a := range in.Allowlist {
        if _, ok := byName[a]; !ok {
            unmatched = append(unmatched, a)
        }
    }

    return skills.AuditResult{
        Window:         in.Window,
        GeneratedAt:    in.Now,
        Skills:         auditRows,
        GhostUsage:     ghosts,
        UnmatchedAllow: unmatched,
    }
}

func isUnknownSource(s skills.Source) bool {
    return s == skills.SourceCodexProject || s == skills.SourceCodexGlobal
}

func tierOrder(t skills.Tier) int {
    switch t {
    case skills.TierKeep:
        return 0
    case skills.TierBorderline:
        return 1
    case skills.TierArchive:
        return 2
    case skills.TierUnknown:
        return 3
    }
    return 4
}
```

- [ ] **Step 4: Run tests, verify PASS**

```bash
go test ./internal/skills/audit/
```

Expected: PASS for all `TestTier_*` cases.

- [ ] **Step 5: `make check` and commit**

```bash
make check
git add internal/skills/audit/
git rm internal/skills/audit/.keep
git commit -m "feat(skills): tier classification (keep/borderline/archive/unknown)"
```

---

### Task 7: Markdown report

**Files:**
- Create: `internal/skills/report/markdown.go`
- Create: `internal/skills/report/markdown_test.go`

- [ ] **Step 1: Write failing test**

```go
package report

import (
    "bytes"
    "strings"
    "testing"
    "time"

    "github.com/psacc/omnisess/internal/skills"
)

func sampleResult() skills.AuditResult {
    now := time.Date(2026, 5, 7, 10, 0, 0, 0, time.UTC)
    return skills.AuditResult{
        Window:          90 * 24 * time.Hour,
        GeneratedAt:     now,
        Roots:           []string{"/home/u/repo-a"},
        GlobalsIncluded: true,
        SessionsParsed:  42,
        OmnisessVersion: "0.2.0",
        Skills: []skills.SkillAudit{
            {Skill: skills.SkillInfo{Name: "frequent", Source: skills.SourceClaudeGlobal, DescChars: 80}, Tier: skills.TierKeep, ModelInvoked: 5, UserInvoked: 7, Total: 12, LastUsed: now.Add(-24 * time.Hour)},
            {Skill: skills.SkillInfo{Name: "rare", Source: skills.SourceProjectClaude, DescChars: 200}, Tier: skills.TierBorderline, ModelInvoked: 1, UserInvoked: 1, Total: 2, LastUsed: now.Add(-50 * 24 * time.Hour)},
            {Skill: skills.SkillInfo{Name: "unused-big", Source: skills.SourceClaudeGlobal, DescChars: 500}, Tier: skills.TierArchive, Total: 0},
            {Skill: skills.SkillInfo{Name: "unused-small", Source: skills.SourceClaudeGlobal, DescChars: 60}, Tier: skills.TierArchive, Total: 0},
            {Skill: skills.SkillInfo{Name: "codex-only", Source: skills.SourceCodexProject, DescChars: 100}, Tier: skills.TierUnknown},
        },
        GhostUsage:     []string{"phantom"},
        UnmatchedAllow: []string{"never-existed"},
    }
}

func TestMarkdownContainsSections(t *testing.T) {
    var buf bytes.Buffer
    if err := Markdown(&buf, sampleResult()); err != nil {
        t.Fatal(err)
    }
    out := buf.String()
    for _, want := range []string{
        "# Skills Audit",
        "## Summary",
        "## Top by usage",
        "## Archive candidates",
        "## Borderline",
        "## Unknown",
        "## Ghost usage",
        "## Unmatched allowlist",
        "## Provenance",
        "frequent",
        "unused-big",
        "rare",
        "codex-only",
        "phantom",
        "never-existed",
        "0.2.0",
    } {
        if !strings.Contains(out, want) {
            t.Errorf("output missing %q", want)
        }
    }
}

func TestMarkdownArchiveSortedByCostDesc(t *testing.T) {
    var buf bytes.Buffer
    Markdown(&buf, sampleResult())
    out := buf.String()
    iBig := strings.Index(out, "unused-big")
    iSmall := strings.Index(out, "unused-small")
    if iBig < 0 || iSmall < 0 {
        t.Fatal("expected both archive entries in output")
    }
    if iBig > iSmall {
        t.Errorf("unused-big (DescChars=500) should appear before unused-small (DescChars=60) in archive section")
    }
}

func TestMarkdownDeterministic(t *testing.T) {
    var a, b bytes.Buffer
    res := sampleResult()
    Markdown(&a, res)
    Markdown(&b, res)
    if a.String() != b.String() {
        t.Errorf("markdown output should be deterministic; diff:\n--A--\n%s\n--B--\n%s", a.String(), b.String())
    }
}
```

- [ ] **Step 2: Run test, verify FAIL**

```bash
go test ./internal/skills/report/
```

Expected: FAIL.

- [ ] **Step 3: Implement Markdown**

Create `internal/skills/report/markdown.go`:

```go
// Package report renders AuditResult as markdown or JSON.
package report

import (
    "fmt"
    "io"
    "sort"
    "strings"
    "time"

    "github.com/psacc/omnisess/internal/skills"
)

// Markdown writes the audit result as a markdown report.
func Markdown(w io.Writer, r skills.AuditResult) error {
    f := func(format string, args ...interface{}) {
        fmt.Fprintf(w, format, args...)
    }

    days := int(r.Window.Hours() / 24)
    f("# Skills Audit\n\n")
    f("Window: last %d days · Generated: %s\n\n", days, r.GeneratedAt.UTC().Format(time.RFC3339))

    // Summary
    var nKeep, nBorder, nArch, nUnk int
    var archiveDescChars int
    for _, s := range r.Skills {
        switch s.Tier {
        case skills.TierKeep:
            nKeep++
        case skills.TierBorderline:
            nBorder++
        case skills.TierArchive:
            nArch++
            archiveDescChars += s.Skill.DescChars
        case skills.TierUnknown:
            nUnk++
        }
    }
    f("## Summary\n\n")
    f("| Tier | Count |\n|---|---|\n")
    f("| Keep | %d |\n| Borderline | %d |\n| Archive | %d |\n| Unknown | %d |\n", nKeep, nBorder, nArch, nUnk)
    f("\nEstimated tokens reclaimable if all archive candidates removed: ~%d (≈ desc chars / 4)\n\n", archiveDescChars/4)

    // Top by usage
    f("## Top by usage\n\n")
    top := append([]skills.SkillAudit(nil), r.Skills...)
    sort.SliceStable(top, func(i, j int) bool { return top[i].Total > top[j].Total })
    f("| Skill | Source | Total | Model | User | Last used |\n|---|---|---|---|---|---|\n")
    for i, s := range top {
        if i >= 20 || s.Total == 0 {
            break
        }
        f("| `%s` | %s | %d | %d | %d | %s |\n", s.Skill.Name, s.Skill.Source, s.Total, s.ModelInvoked, s.UserInvoked, fmtTime(s.LastUsed))
    }
    f("\n")

    // Archive candidates (sorted by DescChars descending, biggest wins first)
    archive := filterTier(r.Skills, skills.TierArchive)
    sort.SliceStable(archive, func(i, j int) bool {
        return archive[i].Skill.DescChars > archive[j].Skill.DescChars
    })
    f("## Archive candidates\n\n")
    if len(archive) == 0 {
        f("_None._\n\n")
    } else {
        f("Sorted by description size (biggest token wins first).\n\n")
        f("| Skill | Source | DescChars | BodyBytes | Last modified | Path |\n|---|---|---|---|---|---|\n")
        for _, s := range archive {
            f("| `%s` | %s | %d | %d | %s | `%s` |\n",
                s.Skill.Name, s.Skill.Source, s.Skill.DescChars, s.Skill.BodyBytes,
                fmtTime(s.Skill.LastModified), s.Skill.Path)
        }
        f("\n")
    }

    // Borderline
    border := filterTier(r.Skills, skills.TierBorderline)
    f("## Borderline\n\n")
    if len(border) == 0 {
        f("_None._\n\n")
    } else {
        f("| Skill | Source | Total | Last used |\n|---|---|---|---|\n")
        for _, s := range border {
            f("| `%s` | %s | %d | %s |\n", s.Skill.Name, s.Skill.Source, s.Total, fmtTime(s.LastUsed))
        }
        f("\n")
    }

    // Unknown
    unk := filterTier(r.Skills, skills.TierUnknown)
    f("## Unknown\n\n")
    if len(unk) == 0 {
        f("_None._\n\n")
    } else {
        f("Telemetry-incomplete sources (Codex/etc.); not parsed in v0.\n\n")
        f("| Skill | Source | Path |\n|---|---|---|\n")
        for _, s := range unk {
            f("| `%s` | %s | `%s` |\n", s.Skill.Name, s.Skill.Source, s.Skill.Path)
        }
        f("\n")
    }

    // Ghost usage
    f("## Ghost usage\n\n")
    if len(r.GhostUsage) == 0 {
        f("_None — every invocation matched a discovered skill._\n\n")
    } else {
        f("Skills invoked in JSONL but not discovered. Likely missing `--root` coverage:\n\n")
        for _, g := range r.GhostUsage {
            f("- `%s`\n", g)
        }
        f("\n")
    }

    // Unmatched allowlist
    f("## Unmatched allowlist\n\n")
    if len(r.UnmatchedAllow) == 0 {
        f("_All allowlist entries matched a discovered skill._\n\n")
    } else {
        f("Allowlist entries with no matching skill. Likely renamed or stale:\n\n")
        for _, a := range r.UnmatchedAllow {
            f("- `%s`\n", a)
        }
        f("\n")
    }

    // Provenance
    f("## Provenance\n\n")
    f("- Roots: %s\n", strings.Join(r.Roots, ", "))
    f("- Globals included: %v\n", r.GlobalsIncluded)
    f("- Sessions parsed: %d\n", r.SessionsParsed)
    f("- omnisess version: %s\n", r.OmnisessVersion)
    return nil
}

func filterTier(rows []skills.SkillAudit, t skills.Tier) []skills.SkillAudit {
    var out []skills.SkillAudit
    for _, r := range rows {
        if r.Tier == t {
            out = append(out, r)
        }
    }
    return out
}

func fmtTime(t time.Time) string {
    if t.IsZero() {
        return "—"
    }
    return t.UTC().Format("2006-01-02")
}
```

- [ ] **Step 4: Run tests, verify PASS**

```bash
go test ./internal/skills/report/
```

Expected: PASS.

- [ ] **Step 5: `make check` and commit**

```bash
make check
git add internal/skills/report/markdown.go internal/skills/report/markdown_test.go
git commit -m "feat(skills): markdown reporter (deterministic, sectioned, sorted)"
```

---

### Task 8: JSON report

**Files:**
- Create: `internal/skills/report/json.go`
- Create: `internal/skills/report/json_test.go`

- [ ] **Step 1: Write failing test**

```go
package report

import (
    "bytes"
    "encoding/json"
    "testing"
)

func TestJSONRoundTrip(t *testing.T) {
    var buf bytes.Buffer
    if err := JSON(&buf, sampleResult()); err != nil {
        t.Fatal(err)
    }
    var got map[string]interface{}
    if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
        t.Fatalf("unmarshal: %v\nraw: %s", err, buf.String())
    }
    if _, ok := got["skills"]; !ok {
        t.Errorf("expected skills key in output: %v", got)
    }
    if _, ok := got["metadata"]; !ok {
        t.Errorf("expected metadata key in output: %v", got)
    }
}

func TestJSONIndented(t *testing.T) {
    var buf bytes.Buffer
    JSON(&buf, sampleResult())
    if !bytes.Contains(buf.Bytes(), []byte("\n  ")) {
        t.Errorf("expected indented JSON output")
    }
}
```

- [ ] **Step 2: Run test, verify FAIL**

```bash
go test ./internal/skills/report/ -run TestJSON
```

Expected: FAIL.

- [ ] **Step 3: Implement JSON**

Create `internal/skills/report/json.go`:

```go
package report

import (
    "encoding/json"
    "io"

    "github.com/psacc/omnisess/internal/skills"
)

type jsonOutput struct {
    Metadata jsonMetadata        `json:"metadata"`
    Skills   []skills.SkillAudit `json:"skills"`
    Ghosts   []string            `json:"ghost_usage,omitempty"`
    Unmatched []string           `json:"unmatched_allowlist,omitempty"`
}

type jsonMetadata struct {
    WindowDays      int      `json:"window_days"`
    GeneratedAt     string   `json:"generated_at"`
    Roots           []string `json:"roots"`
    GlobalsIncluded bool     `json:"globals_included"`
    SessionsParsed  int      `json:"sessions_parsed"`
    OmnisessVersion string   `json:"omnisess_version"`
}

// JSON writes the audit result as indented JSON.
func JSON(w io.Writer, r skills.AuditResult) error {
    out := jsonOutput{
        Metadata: jsonMetadata{
            WindowDays:      int(r.Window.Hours() / 24),
            GeneratedAt:     r.GeneratedAt.UTC().Format("2006-01-02T15:04:05Z"),
            Roots:           r.Roots,
            GlobalsIncluded: r.GlobalsIncluded,
            SessionsParsed:  r.SessionsParsed,
            OmnisessVersion: r.OmnisessVersion,
        },
        Skills:    r.Skills,
        Ghosts:    r.GhostUsage,
        Unmatched: r.UnmatchedAllow,
    }
    enc := json.NewEncoder(w)
    enc.SetIndent("", "  ")
    return enc.Encode(out)
}
```

- [ ] **Step 4: Run tests, verify PASS**

```bash
go test ./internal/skills/report/
```

Expected: PASS.

- [ ] **Step 5: `make check` and commit**

```bash
make check
git add internal/skills/report/json.go internal/skills/report/json_test.go
git rm internal/skills/report/.keep
git commit -m "feat(skills): JSON reporter (indented, machine-readable)"
```

---

### Task 9: Cobra wiring (`omnisess skills audit`)

**Files:**
- Create: `cmd/skills.go`
- Create: `cmd/skills_audit.go`
- Create: `cmd/skills_audit_test.go`
- Modify: `cmd/root.go` (register `skillsCmd`)

- [ ] **Step 1: Inspect existing command pattern**

Read `cmd/list.go` and `cmd/active.go` to understand how omnisess wires Cobra commands. Note where `rootCmd.AddCommand(...)` lives (probably in each file's `init()`).

- [ ] **Step 2: Create the parent command**

Create `cmd/skills.go`:

```go
package cmd

import (
    "github.com/spf13/cobra"
)

var skillsCmd = &cobra.Command{
    Use:   "skills",
    Short: "Inspect and audit agent skills",
    Long:  "Subcommands for inspecting agent-skill inventories and usage. v0 ships only `audit`.",
}

func init() {
    rootCmd.AddCommand(skillsCmd)
}
```

- [ ] **Step 3: Create the audit subcommand**

Create `cmd/skills_audit.go`:

```go
package cmd

import (
    "fmt"
    "io"
    "os"
    "path/filepath"
    "strings"
    "time"

    "github.com/spf13/cobra"

    "github.com/psacc/omnisess/internal/skills/allowlist"
    "github.com/psacc/omnisess/internal/skills/audit"
    "github.com/psacc/omnisess/internal/skills/discovery"
    "github.com/psacc/omnisess/internal/skills/report"
    "github.com/psacc/omnisess/internal/skills/usage"
)

var (
    auditRoots       []string
    auditNoGlobals   bool
    auditAllowlist   string
    auditWindow      string
    auditFormat      string
    auditOutput      string
)

var skillsAuditCmd = &cobra.Command{
    Use:   "audit",
    Short: "Classify skills by usage (Keep/Borderline/Archive/Unknown)",
    Long: `Audit agent skills by joining the discovered inventory with invocation
counts from Claude Code session JSONL transcripts.

Read-only. Never modifies skills, sessions, or the allowlist.`,
    RunE: runSkillsAudit,
}

func init() {
    skillsCmd.AddCommand(skillsAuditCmd)
    skillsAuditCmd.Flags().StringSliceVar(&auditRoots, "root", nil, "Project root to walk (repeatable)")
    skillsAuditCmd.Flags().BoolVar(&auditNoGlobals, "no-globals", false, "Skip global skill locations (~/.claude/skills, ~/.agents/skills, ~/.claude/plugins)")
    skillsAuditCmd.Flags().StringVar(&auditAllowlist, "allowlist", "", "Path to plain-text allowlist file (one skill name per line)")
    skillsAuditCmd.Flags().StringVar(&auditWindow, "window", "90d", "Telemetry window: 30d, 90d, 180d")
    skillsAuditCmd.Flags().StringVar(&auditFormat, "format", "markdown", "Output format: markdown or json")
    skillsAuditCmd.Flags().StringVarP(&auditOutput, "output", "o", "", "Output file (default stdout)")
}

func runSkillsAudit(cmd *cobra.Command, args []string) error {
    if len(auditRoots) == 0 && auditNoGlobals {
        return fmt.Errorf("at least one --root is required (or remove --no-globals)")
    }

    window, err := parseWindow(auditWindow)
    if err != nil {
        return err
    }

    // 1. Discover
    discOpts := discovery.Options{
        Roots:          auditRoots,
        IncludeGlobals: !auditNoGlobals,
    }
    skillsList, err := discovery.Walk(discOpts)
    if err != nil {
        return fmt.Errorf("discovery: %w", err)
    }

    // 2. Scan usage
    home, _ := os.UserHomeDir()
    projectsRoot := filepath.Join(home, ".claude", "projects")
    files, err := usage.FindSessionFiles(projectsRoot)
    if err != nil {
        return fmt.Errorf("find sessions: %w", err)
    }
    invocations, err := usage.Scan(usage.ScanOptions{
        Files: files,
        Since: time.Now().Add(-window),
    })
    if err != nil {
        return fmt.Errorf("scan sessions: %w", err)
    }

    // 3. Allowlist
    allow, err := allowlist.Load(auditAllowlist)
    if err != nil {
        return fmt.Errorf("allowlist: %w", err)
    }

    // 4. Tier
    res := audit.Tier(audit.TierInput{
        Skills:      skillsList,
        Invocations: invocations,
        Allowlist:   allow,
        Window:      window,
        Now:         time.Now(),
    })
    res.Roots = auditRoots
    res.GlobalsIncluded = !auditNoGlobals
    res.SessionsParsed = len(files)
    res.OmnisessVersion = resolveVersion()

    // 5. Render
    var w io.Writer = os.Stdout
    if auditOutput != "" {
        f, err := os.Create(auditOutput)
        if err != nil {
            return err
        }
        defer f.Close()
        w = f
    }
    switch strings.ToLower(auditFormat) {
    case "markdown", "md":
        return report.Markdown(w, res)
    case "json":
        return report.JSON(w, res)
    default:
        return fmt.Errorf("unknown format %q (want markdown or json)", auditFormat)
    }
}

func parseWindow(s string) (time.Duration, error) {
    switch s {
    case "30d":
        return 30 * 24 * time.Hour, nil
    case "90d":
        return 90 * 24 * time.Hour, nil
    case "180d":
        return 180 * 24 * time.Hour, nil
    }
    return 0, fmt.Errorf("invalid --window %q (allowed: 30d, 90d, 180d)", s)
}

```

> `resolveVersion()` is defined in `cmd/version.go` and returns the build-info-derived version string, falling back to `"(devel)"`. Reuse it; do not introduce a new version mechanism.

- [ ] **Step 4: Smoke test on real data**

```bash
go build -o omnisess .
./omnisess skills audit --root ~/prj/psacc --root ~/prj/finn -o /tmp/audit.md
head -60 /tmp/audit.md
```

Expected: a populated markdown report with non-zero counts in the "Top by usage" section.

- [ ] **Step 5: Add a smoke test**

Create `cmd/skills_audit_test.go`:

```go
package cmd

import (
    "testing"
    "time"
)

func TestParseWindow(t *testing.T) {
    cases := map[string]time.Duration{
        "30d":  30 * 24 * time.Hour,
        "90d":  90 * 24 * time.Hour,
        "180d": 180 * 24 * time.Hour,
    }
    for in, want := range cases {
        got, err := parseWindow(in)
        if err != nil {
            t.Errorf("parseWindow(%q) error: %v", in, err)
        }
        if got != want {
            t.Errorf("parseWindow(%q) = %v want %v", in, got, want)
        }
    }
    if _, err := parseWindow("60d"); err == nil {
        t.Error("expected error for unsupported window")
    }
}
```

- [ ] **Step 6: `make check` and commit**

```bash
make check
git add cmd/skills.go cmd/skills_audit.go cmd/skills_audit_test.go
git commit -m "feat(cmd): add omnisess skills audit subcommand"
```

---

### Task 10: User-facing documentation

**Files:**
- Create: `docs/skills-audit.md`
- Modify: `README.md` (add line under commands list)

- [ ] **Step 1: Write `docs/skills-audit.md`**

The doc must include:
1. **What it is** — one-paragraph framing matching the "Why this exists" section above.
2. **Cross-tool comparison table** — verbatim from this plan's "Why this exists / Cross-tool prior art" section.
3. **Quickstart** — three commands: minimal run, with allowlist, JSON output.
4. **Read-only guarantee** — explicit statement.
5. **Tier rules** — table from this plan.
6. **Allowlist format** — example file.
7. **Output sections** — what each section means.
8. **v0.5+ roadmap** — Codex parser, Cursor/Gemini parsers, scheduled runs.
9. **Reference: Anthropic issue #35319** as the original gap.

Keep the doc <300 lines.

- [ ] **Step 2: Update README**

Add a line under the commands list:

```markdown
- `omnisess skills audit` — classify skills by usage (Keep/Borderline/Archive); see [docs/skills-audit.md](docs/skills-audit.md)
```

- [ ] **Step 3: Commit**

```bash
git add docs/skills-audit.md README.md
git commit -m "docs(skills): add user-facing audit guide and README link"
```

---

### Task 11: Validation via parallel subagents

> This task uses `superpowers:dispatching-parallel-agents`. Three subagents run in parallel after all code tasks pass `make check`. Do **not** merge to main without their reports.

- [ ] **Step 1: Dispatch Subagent A — parser ground-truth**

Prompt (copy verbatim into Agent tool, `subagent_type: general-purpose`):

> You are validating the parsing layer of `omnisess skills audit`. The audit was just implemented under branch `feature/skills-audit-v0`. Your job: independently verify that the JSONL parser counts invocations correctly.
>
> Steps:
> 1. Pick three real session JSONL files from `~/.claude/projects/`. Choose files modified in the last 7 days, size ≥ 100KB, that contain at least one `"name":"Skill"` line (use `grep -l`).
> 2. For each file, run a ground-truth grep:
>    - `grep -c '"name":"Skill"' <file>` → expected model invocations
>    - `grep -c '<command-name>/' <file>` → expected user invocations (note: this counts ALL slash commands, not just skill-named ones)
> 3. Build omnisess on the feature branch (`go build -o /tmp/omnisess .`) and run `/tmp/omnisess skills audit --root ~/prj/psacc --format json -o /tmp/audit.json`.
> 4. From `/tmp/audit.json`, extract per-skill `model_invoked` and `user_invoked` counts. Sum them. Compare against the ground-truth grep totals.
> 5. Discrepancies are expected for slash commands (we filter to known skills). Document any *unexpected* discrepancy: e.g. a Skill tool_use that ground-truth grep finds but the report omits.
>
> Output a ~150-word report: pass/fail, any discrepancies, root cause if any. Do not modify code.

- [ ] **Step 2: Dispatch Subagent B — sanity check the report**

Prompt:

> You are reviewing the markdown audit report produced by `omnisess skills audit` on branch `feature/skills-audit-v0`. Your job: spot anything that smells wrong before merge.
>
> Steps:
> 1. Build and run: `go build -o /tmp/omnisess . && /tmp/omnisess skills audit --root ~/prj/psacc --root ~/prj/finn -o /tmp/audit.md`
> 2. Read `/tmp/audit.md` end-to-end.
> 3. Check the following:
>    - **Heavily-used skills should be in Keep, not Archive.** Cross-reference against `~/.claude/projects/` JSONL: any skill the user invokes weekly should not land in Archive. Flag if you see one.
>    - **`gws-*`, `recipe-*`, `persona-*` clusters should mostly be in Archive** (the original thesis to validate). If they're in Keep, the parser is undercounting.
>    - **Top by usage section should be plausible.** Skills like `agent-slack`, `omnisess-search`, `recap` are likely top entries on this machine.
>    - **Ghost usage** should be small or empty. A long ghost list means `--root` coverage is incomplete.
>    - **Unmatched allowlist** is empty (no allowlist used in this run, so this section should say "all matched" or be absent).
>    - **Provenance** lists both roots and shows ≥30 sessions parsed.
>    - **No duplicate skill rows.** Each skill name appears at most once.
>    - **Markdown is valid** (run `npx markdownlint /tmp/audit.md` if lint passes; not required, but spot-check headings render).
>
> Output a ~250-word review: top concerns, recommended fixes, overall pass/fail. Do not modify code.

- [ ] **Step 3: Dispatch Subagent C — user-facing doc**

Prompt:

> You are writing the user-facing documentation for the new `omnisess skills audit` feature on branch `feature/skills-audit-v0`. The exec plan at `docs/exec-plans/active/014-skills-audit.md` has all the design context. Your job: produce a clean `docs/skills-audit.md`.
>
> Requirements:
> - Read `docs/exec-plans/active/014-skills-audit.md` end-to-end first.
> - Write `docs/skills-audit.md`, ~250 lines max.
> - Sections (in order): What it is · Why we built it · Cross-tool comparison table (use the table from the exec plan verbatim) · Quickstart (3 commands) · Read-only guarantee · Tier rules table · Allowlist format with example · Output sections explained · v0.5+ roadmap · References (Anthropic issue #35319, mattpocock skills, Factory.ai skills doc).
> - Tone: technical, terse. No marketing.
> - Do NOT modify any code or other docs.
> - Commit your work as `docs(skills): add user-facing audit guide` on the same branch.

- [ ] **Step 4: Wait for all three subagents, review reports**

Read each subagent's output. If A or B reports a failure, fix the underlying issue (a fresh implementation task, not a hack), re-run the subagent. If C produced the doc, replace the placeholder from Task 10 if it diverges.

- [ ] **Step 5: Final `make check` after any fixes**

```bash
make check
```

Expected: PASS.

---

### Task 12: PR

- [ ] **Step 1: Move exec plan to completed**

```bash
git mv docs/exec-plans/active/014-skills-audit.md docs/exec-plans/completed/014-skills-audit.md
git commit -m "chore(plan): mark 014 skills audit as completed"
```

- [ ] **Step 2: Push and open PR**

```bash
git push -u origin feature/skills-audit-v0
gh pr create --title "feat: omnisess skills audit (v0)" --body "$(cat <<'EOF'
## Summary

Adds `omnisess skills audit` — read-only classification of agent skills by observed invocation count in Claude Code session JSONL.

- New package `internal/skills/` (discovery, usage, allowlist, audit, report)
- New Cobra subcommand `omnisess skills audit` with `--root`, `--no-globals`, `--allowlist`, `--window`, `--format`, `-o`
- Markdown + JSON output
- Tiers: Keep / Borderline / Archive / Unknown
- v0 telemetry: Claude Code only; Codex/Cursor/Gemini skills surface as Unknown

## Test plan

- [x] Unit tests across all subpackages (`make check`)
- [x] Smoke test on `~/prj/psacc` and `~/prj/finn`
- [x] Subagent A: parser ground-truth verified against grep
- [x] Subagent B: report sanity-checked against expected usage patterns
- [x] Subagent C: user-facing doc reviewed

## Out of scope

- Codex/Cursor/Gemini telemetry parsing (v0.5)
- Auto-archive / mutating actions (deliberate — read-only invariant)
- Pattern matching in allowlist (v0.5)

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 3: Wait for human review and merge**

Per `psacc/CLAUDE.md`: git push to public repos is visible to others — DO NOT auto-merge. Wait for explicit user approval.

---

## Out of scope (v0.5+ — track in follow-up plans)

1. **Codex telemetry parser.** Parse `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl`, detect skill invocations as `Read` tool calls into `**/skills/*/SKILL.md`. Upgrades Codex skills from `Unknown` to real tiers.
2. **Cursor / Gemini parsers.** Block on omnisess landing those Source implementations.
3. **Allowlist pattern matching.** Glob (`gws-*`) or regex.
4. **Allowlist YAML format** with metadata (reason, expiry, owner).
5. **`--archive-script` flag.** Generate a shell script the user can review and run; still no auto-mutation.
6. **`/loop` integration.** Weekly diff against previous report, Slack DM on changes.
7. **`omnisess skills list`** (inventory only) and **`omnisess skills usage`** (raw counts).
8. **Cost-budget what-if mode.** "If I archive these N skills, how many tokens do I save?"
9. **HTML rendering.** `pandoc` covers it externally.

## Known unknowns

- **Codex skill discovery may pick up Antigravity / Gemini skills** if they share the SKILL.md convention. They'll classify as `SourceCodexProject` since the path heuristic only differentiates `.codex/`. Acceptable v0 imprecision; revisit when adding Codex telemetry.
- **Plugin-bundled skills install/uninstall as a unit.** Pruning recommendations should not suggest deleting individual files inside `~/.claude/plugins/<plugin>/skills/`. The output sources these as `plugin` so a future report can flag them.
- **Worktree exclusion is path-based.** If you have a worktree at a non-standard path (not under `*/worktrees/*`), it won't be excluded. Acceptable — the user can use a different `--root`.

## References

- Anthropic Claude Code skills: https://code.claude.com/docs/en/skills.md
- Open issue (skill telemetry): https://github.com/anthropics/claude-code/issues/35319
- OpenAI Codex skills: https://developers.openai.com/codex/skills
- mattpocock / `npx skills` (vercel-labs/skills): https://github.com/vercel-labs/skills
- Factory.ai skills: https://docs.factory.ai/cli/configuration/skills
- omnisess existing exec plan format: `docs/exec-plans/active/013-path-filter.md`
- omnisess git workflow: `docs/process/git-workflow.md`
