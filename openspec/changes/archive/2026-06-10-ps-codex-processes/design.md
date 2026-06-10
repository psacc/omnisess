# Design: ps-codex-processes

## Context

`omnisess ps` (macOS only) enumerates live Claude Code sessions via the `~/.claude/sessions/<PID>.json` registry, walks ancestor chains from one `ps -Ao pid=,ppid=,comm=,args=` snapshot, and renders a merged process tree. Codex has no PID registry. Empirical findings on a live machine (verified against two running codex TUIs and one Codex.app app-server):

- A codex TUI process (`comm == codex`) holds its rollout JSONL (`~/.codex/sessions/YYYY/MM/DD/rollout-<ts>-<uuid>.jsonl`) **open for writing** for the session's lifetime — `lsof -p <pid>` shows it.
- The Codex.app app-server (`comm == /Applications/Codex.app/Contents/Resources/codex`, basename `codex`) holds rollout files open only while a thread is live; idle app-servers hold none.
- The rollout's first line is `{"type":"session_meta","payload":{"id","timestamp","cwd","originator","cli_version",...}}`. The line can be tens of KB (embedded base instructions).
- `~/.codex/session_index.jsonl` maps id→thread_name but is only written on rename/save; live sessions are usually absent.

## Goals / Non-Goals

**Goals:**
- `omnisess ps` lists live codex sessions alongside claude ones, in the same merged ancestor tree.
- Exact PID↔session correlation (no mtime heuristics).
- 100% per-package test coverage maintained; all externals injectable.
- Graceful degradation: codex enumeration failure never breaks claude output.

**Non-Goals:**
- Session names for codex (session_index.jsonl lookup) — deferred; leaf shows short id.
- TUI live-badge support for codex (`ApplySnapshot` stays claude-only).
- `active` command changes (tracked separately as issue #74).
- Linux/Windows support (procsnap is darwin-only by design).

## Decisions

1. **lsof-held rollout file as the correlation primitive** (over cwd+mtime matching or `~/.codex/process_manager/chat_processes.json`).
   - lsof gives an exact pid→session-file edge; cwd/mtime is ambiguous with multiple sessions per directory; `chat_processes.json` was empty for live TUI sessions on the reference machine.
   - Cost: one extra subprocess (`lsof -a -p <pid,...> -F pfn`) only when codex candidates exist.

2. **Candidate filtering from the existing ps snapshot** — PIDs whose `comm` basename is `codex`. Catches both the TUI binary and Codex.app's app-server. No candidates → no lsof call.

3. **One `lsof` invocation for all candidates** (`-n -P -a -p p1,p2 -F pfn`; `-n -P` skip DNS/port-name resolution since codex holds open sockets), machine-readable `-F` output parsed into pid→{cwd, open rollout paths}. Each open rollout = one session (an app-server with N live threads yields N sessions sharing a PID).

4. **session_meta first-line parse with filename fallback.** Read the rollout's first line (bounded scanner, 4 MiB cap) for id, cwd, started-at, originator→Entrypoint, cli_version→Version. If unreadable/malformed, fall back to id + start time parsed from the filename and cwd from lsof, warning on stderr. A held `.jsonl` is dropped (with a warning) only when meta AND filename parses both fail — so a future rollout-naming change degrades loudly, not silently.

5. **`Tool` field on `procsnap.Session`** (`"claude"`/`"codex"`), set for both enumerations. `cmd/ps.go` renders `s.Tool` in the leaf label. `ps --json` gains the field (additive). `internal/tui.ApplySnapshot` filters the snapshot map to claude entries to preserve its documented claude-only contract.

6. **Injectables for testability**: `codexLsofFn(pids []int) ([]byte, error)` and `codexSessionsDirFn() (string, error)` mirror the existing `psRunnerFn`/`sessionsDirFn` pattern, enabling fixture-driven tests and 100% coverage without touching real `~/.codex`.

## Risks / Trade-offs

- [lsof latency] lsof scoped with `-a -p <pids>` is fast (tens of ms); only runs when codex processes exist. Mitigation: measure in QA against the 5s budget (TESTING.md §4.3).
- [Transient readers misdetected] `codex resume` picker briefly opens rollouts read-only. Mitigation: restrict matches to files under the codex sessions dir matching the rollout name pattern; transient opens are a narrow race and yield at worst a short-lived extra row. Accepted.
- [Codex format drift] `session_meta` shape or rollout path scheme changes upstream. Mitigation: filename fallback keeps id/started-at; parse failures degrade to fallback, never error out the command.
- [lsof unavailable/sandboxed] Mitigation: warn to stderr, return claude-only sessions (same best-effort posture as the existing `ps` failure path).
- [PID reuse between ps and lsof] Negligible window; lsof `-a` conjunction means a reused PID would have to be another codex process holding a rollout open.
- [Shared rollout fd across PIDs] A forked codex child inheriting the rollout fd would yield duplicate rows for one session. Not observed in practice (verified live: N rollouts, N distinct holders); no dedup in v1.
- [Binary-name edge] Candidate matching is comm-basename == `codex`; running the raw arch-suffixed binary (e.g. `codex-aarch64-apple-darwin`) directly would be missed. Accepted: every supported install path (brew cask shim, Codex.app) exposes comm `codex`.
- [Codex UUIDv7 short-id ambiguity] The first 8 displayed chars of codex ids are mostly timestamp bits, so near-simultaneous sessions can render the same short id. Accepted: display stays consistent with every other command (first 8, prefix-resolvable by `show`); full ids are in `--json`.

## Migration Plan

Additive feature, no migration. Rollback = revert the PR. `ps --json` consumers see a new `Tool` field only.

## Open Questions

(none)
