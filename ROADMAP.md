## Strategic context
Priority: medium — v0.9.0 shipped; next focus is correctness of "active" (#74), which downstream automation depends on
Current phase: Active-definition unification (#74) — route `active` through the same live-process correlation as `ps`
Blocks: downstream automation that consumes the active-session count; worktree-aware session listing (Phase 2.5)
Blocked by: nothing
Last updated: 2026-06-10

---

# omnisess Roadmap

CLI that aggregates AI coding sessions across Claude Code, Cursor, Codex, and GitHub Copilot CLI. See `CLAUDE.md` for invariants and the dev loop, `ARCHITECTURE.md` for the codemap.

## Shipped (summary)

- **v0.1–v0.7** — Source interface + Claude/Cursor/Codex/Copilot parsers (Gemini stub), `list`/`search`/`active`/`show`/`digest`, multi-resume, TUI dashboard, `ps` process tree for live Claude sessions, skills audit.
- **v0.8.x** — Opt-in SQLite transcript index (OTel GenAI–aligned), `stats`/`index` commands, 5–11× perf on JSONL-scan paths via bounded parallelization.
- **v0.9.0** — `ps` detects live **Codex** sessions (CLI TUI + Codex.app) by correlating codex PIDs with lsof-held rollout files; `Tool` field on `procsnap.Session`.

## Priorities

### 1. Unify the definition of "active" — #74 (top priority)

`active` and `ps` disagree today: `ps` reads the `~/.claude/sessions/<PID>.json` registry (pid liveness, per-session status), while `active` uses transcript mtime + a global `pgrep` probe — weak heuristics that miscount, especially with an always-on agent fleet.

- [ ] Investigate Claude Code session-registry internals (status semantics: busy/idle/waiting; registry GC behavior) — prerequisite noted on #74
- [ ] Route claude `active` detection through the registry (share the `procsnap` correlation; one implementation of "active", not two)
- [ ] Reuse the v0.9.0 codex lsof correlation for codex `active`
- [ ] Fix doc drift: `ActiveThreshold` is 10m in code, glossary says 2m
- [ ] Update TESTING.md §4.1 expectations for `active`

One shared implementation of "active", not two. Unblocks downstream automation that consumes the active-session count.

### 2. Correctness bugs (pre-existing, found by v0.9.0 QA)

- [ ] #77 — `search` table output invalid UTF-8: rune-align snippet truncation in `extractSnippet` (same fix class as FUP-B)
- [ ] #78 — `search` silently skips sessions with over-long lines (bufio token too long): grow buffer or switch reader, and warn loudly

### 3. Phase 2.5 — session lifecycle & worktrees

- [ ] `stop`/`restart`/`kill` for running sessions (exec plan `012-lifecycle-store.md`)
- [ ] Model git worktrees as first-class session location

Informs resume/kill semantics for scheduled agents and worktree-aware session listing in companion tooling. Lifecycle commands write to processes (signals), not source files — invariant #7 (read-only local data) still holds for transcripts.

### 4. Performance (deferred pending evidence)

- [ ] #71 — parallelize `Search` in cursor/codex/copilot sources. Per invariant #4, index-routing stays deferred unless parallelization proves insufficient.

### 5. Process & CI hygiene

- [ ] #66 — branch protection: require `cover-check` job in addition to `check`
- [ ] #67 — recalibrate TESTING.md §4.3 perf thresholds (corpus has grown; 5 commands sit in the 5–30 s band)
- [ ] #68 — TESTING.md §8 formal override path for pre-existing non-regression FAILs

### 6. Exploratory backlog (unscheduled)

- [ ] Antigravity CLI source — on hold, deliberately unscheduled (successor to the dropped Gemini stub)
- [ ] `recap` consuming `digest` output as a richer LLM source than list previews
- [ ] Copilot digest noise: strip verbose VS Code `command:…openSymbolFromReferences` URLs (no issue filed yet)
