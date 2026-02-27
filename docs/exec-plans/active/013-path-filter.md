# 013 — Path-Based Session Filtering

**Status:** Proposal
**Created:** 2026-02-26
**Scope:** Small (~1–2 days)

---

## Problem Statement

Every `omnisess` command today shows sessions across all projects. The only
existing filter is `--project <substring>`, a global persistent flag backed by
`ListOptions.Project`. This is usable but manual — you must remember to pass
the flag and know the correct substring every time.

When working inside a specific repo you nearly always want to see only that
repo's sessions. The current UX makes the common case require extra flags; it
should be zero-effort.

---

## Proposed CLI UX

### 1. Positional path argument (explicit, per-command)

Add an optional trailing positional argument to `list`, `active`, and `search`
that specifies a project path:

```
omnisess list [path]
omnisess active [path]
omnisess search <query> [path]
```

Examples:

```bash
omnisess list /Users/paolo/prj/foo          # absolute path
omnisess list ./prj/foo                     # relative (resolved to absolute)
omnisess list .                             # current directory
omnisess search "sqlite" /Users/paolo/prj/foo
```

`show` is excluded: it takes a qualified ID, not a project query. Path
filtering on a `show` call is semantically meaningless (the session is already
identified).

### 2. `--here` flag (implicit cwd, explicit opt-in)

Add a global persistent boolean flag `--here` that tells omnisess to use
`os.Getwd()` as the project filter:

```bash
omnisess list --here
omnisess active --here
omnisess search "sqlite" --here
```

`--here` and a positional path argument are mutually exclusive — the command
should error if both are provided.

### 3. Auto-detect (deferred — see Open Questions)

Automatic cwd injection without any flag is intentionally **not** included in
this plan. See Open Questions.

---

## Mapping to `ListOptions.Project`

`ListOptions.Project` is already a `string` used as a substring match against
`Session.Project`. No changes to `ListOptions` or the `Source` interface are
required.

The resolved project path (from positional arg or `--here`) feeds directly
into `ListOptions.Project`. Sources already do substring matching, so passing
an absolute path like `/Users/paolo/prj/foo` will match any session whose
`Project` field contains that string.

**No changes needed in:**
- `internal/source/source.go`
- `internal/model/session.go`
- Any source implementation (`claude/`, `cursor/`, `codex/`, `gemini/`)

---

## What Needs to Change

### A. `cmd/root.go`

1. Add `flagHere bool` to the package-level flag vars.
2. Register `--here` as a persistent flag on `rootCmd`.
3. Update `getListOptions()` to resolve project path:
   - If positional path arg is set → `filepath.Abs(arg)` → set `opts.Project`
   - Else if `--here` → `os.Getwd()` → set `opts.Project`
   - Else if `--project` is set → use as-is (existing behaviour, unchanged)
   - If more than one of the above is non-empty → return an error

### B. `cmd/list.go`

1. Change `Use` to `"list [path]"`.
2. Change `Args` validator from implicit (none) to `cobra.MaximumNArgs(1)`.
3. Extract positional arg (if present) and pass to `getListOptions()` — or use
   a helper `resolveProjectFilter(cmd, args)`.

### C. `cmd/active.go`

Same changes as `cmd/list.go`.

### D. `cmd/search.go`

1. Change `Use` to `"search <query> [path]"`.
2. Update `Args` from `cobra.MinimumNArgs(1)` to accept 1 or 2 args.
3. `args[0]` remains the query; `args[1]` (optional) is the path.

### E. `cmd/show.go`

No changes. Path filtering is meaningless for a by-ID lookup.

### F. Helper: `resolveProjectFilter`

Extract into a shared function (in `cmd/root.go` or a new `cmd/flags.go`):

```go
// resolveProjectFilter returns the project path to filter on, or "" for no filter.
// Priority: positional arg > --here > --project flag.
// Returns an error if conflicting inputs are provided.
func resolveProjectFilter(positionalPath string) (string, error) {
    if positionalPath != "" && flagHere {
        return "", fmt.Errorf("cannot combine a path argument with --here")
    }
    if positionalPath != "" {
        return filepath.Abs(positionalPath)
    }
    if flagHere {
        return os.Getwd()
    }
    return flagProject, nil // existing --project behaviour
}
```

This keeps `getListOptions()` clean and makes the resolution testable.

---

## Open Questions

### Q1: Should `--here` be a global flag or per-command?

**Leaning: global persistent flag on `rootCmd`.**

- Rationale: cwd context is orthogonal to which subcommand you run. Making it
  per-command duplicates the flag and diverges help text.
- Risk: `show` inherits the flag but ignores it — slightly noisy in `--help`.
  Acceptable since `show` already ignores `--project`.

### Q2: Should cwd auto-detection be opt-in (`--here`) or default?

**Decision: opt-in (`--here`) for now. Do not auto-detect.**

Arguments against auto-detect:
- Surprising behaviour: running `omnisess list` from your home directory would
  silently return 0 results (no sessions for `~`).
- Hard to override: users would need an escape hatch (`--no-here`?) to see all
  sessions from inside a project dir.
- Violates principle of least surprise for a CLI tool — explicit is better.

Revisit if usage data shows `--here` is used in >80% of invocations (signals
the default should flip).

### Q3: Should path matching be exact prefix vs. substring?

**Current behaviour: substring.** No change proposed here.

Substring is more forgiving (works with partial paths like `prj/foo`) but can
over-match (a project named `foo` would match `foobar`). An exact prefix match
(`strings.HasPrefix(session.Project, filterPath)`) would be more correct when
passing an absolute path. This is a separate cleanup — file as follow-up if
substring causes false positives in practice.

### Q4: What if the resolved path does not exist on disk?

Emit a warning to stderr and proceed (return 0 results rather than error out).
The path is a filter hint, not a requirement.

---

## Rough Scope

| Area | Effort |
|------|--------|
| `cmd/root.go` flag + helper | ~30 min |
| `cmd/list.go`, `cmd/active.go` arg parsing | ~30 min |
| `cmd/search.go` arg parsing | ~20 min |
| Tests (table-driven, `cmd` package) | ~2 h |
| Manual smoke test | ~20 min |
| **Total** | **~3–4 h** |

No new external dependencies. No changes to the `Source` interface or
`model.*` types. This is a **two-way door** change — reversible by removing
the flag and positional arg with no data migration.

---

## Non-Goals

- Watching for cwd changes at runtime
- Fuzzy/regex path matching
- Shell completions for the path argument (nice-to-have, separate PR)
