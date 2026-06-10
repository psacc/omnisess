# Testing — Agentic QA Runbook

How omnisess uses an LLM subagent for pre-release QA. This document is the runbook the maintainer follows on every release-prep PR.

## 1. Philosophy

`make check` and `make cover-check` enforce correctness (types, behavior, 100% line coverage). They cannot verify **product-level sanity**: does `omnisess list` actually display sessions in a way a human would call right? Does `digest` look correct rendered as markdown? Did a refactor silently change the shape of error messages?

Agentic QA closes that gap. A subagent installs the pre-release binary, walks through every user-facing command, scores the output, and reports findings. It catches the class of bug PR #44 shipped — code that compiled, passed CI, and still produced wrong output (invalid UTF-8 from byte-slice truncation; bogus `~bar/proj` paths; hardcoded allow-list missing a registered tool).

The agent does not replace human review. It is one more gate, cheap enough to run every release.

## 2. When to Run

- **Mandatory:** on every release-prep PR, after CI is green, before merging.
- **Optional:** any time you want a smoke check (e.g., after a large refactor on `main`).

The release-prep PR is the gate. See [`docs/process/release.md`](docs/process/release.md) for where this step sits in the release flow.

## 3. Data Policy (PII Constraint)

The agent runs on your local machine and has read access to real `~/.claude`, `~/.cursor`, `~/.codex`, `~/.copilot`, and VS Code workspaceStorage. This is intentional — synthetic fixtures cannot reproduce the scale or edge-case content that surfaces real bugs (the UTF-8 bug needed multi-byte runes from real Italian content; the HOME-prefix bug needed a real `$HOME` layout).

**Hard constraint on the agent's report content:**

| Allowed | Forbidden |
|---|---|
| Counts (e.g., "list returned N sessions") | Session previews, Q&A content, tool-call arguments |
| Exit codes | Project paths past the basename (e.g., `myapp` ✅, `/Users/foo/prj/internal-customer-data` ❌) |
| Error class / first line of `stderr` (if structural) | Full session IDs (use first 6 chars + `…`) |
| Wallclock time | Branch names, email addresses, hostnames |
| Format observations ("header missing date", "table column misaligned") | Verbatim output excerpts longer than one line |
| Boolean assertions ("digest output is valid UTF-8: yes") | Anything that quotes user content |

**Synthetic fallback:** if a source has zero local data (e.g., you don't use Codex), the agent generates a minimal fixture under `$TMPDIR/omnisess-qa/<source>/` and re-runs that source's command against it. The fallback is documented per-source in §4.

## 4. Coverage Scope

Two layers run on every release:

### 4.1 Baseline — golden + error path, every command

For each command, the agent runs one **golden** invocation and one **designed-to-fail** invocation. Findings go in the report.

| Command | Golden | Error path | Notes |
|---|---|---|---|
| `version` | `omnisess version` | — | Must print a version matching the prep PR's SKILL.md bump. |
| `list` | `omnisess list --limit 5` | `omnisess list --since not-a-duration` | Synthetic fallback: no fixture, the command should just produce "0 sessions". |
| `search` | `omnisess search "the"` (very common word, will match) | `omnisess search ""` | Empty query should error cleanly, not hang. |
| `active` | `omnisess active` | — | Output may legitimately be empty; check for clean exit. On macOS, every claude/codex row must correspond to a live process in `omnisess ps` (one shared snapshot — #74); rows show ID, UPDATED, and `ACTIVE (<status>)`. |
| `show` | `omnisess show <tool>:<id>` for a real session (or synthetic) | `omnisess show unknown:abc` | Error message must list every registered source (drift guard from FUP-D). |
| `digest` | `omnisess digest --date <today>` | `omnisess digest --date not-a-date` | Output must be valid UTF-8 (regression guard for FUP-B). Synthetic fallback: write one fake session under `$TMPDIR/omnisess-qa/claude/`. |
| `ps` | `omnisess ps` (macOS) | `omnisess ps` (non-macOS) | Covers claude + codex sessions when present (codex rows require a live codex process). Linux/Windows: SKIPPED — platform. |
| `tui` | `timeout 2 ./omnisess-prerelease tui </dev/null 2>tui.stderr; check tui.stderr` for `panic:` / `fatal:` | — | 2-second timed launch — catches init-panic class. Interactive flow is OUT OF SCOPE (manual eyes only). |
| `skills audit` | `omnisess skills audit --window 90d` | — | If `~/.claude/skills` doesn't exist: SKIPPED — no skills directory. |

### 4.2 Risk overlay — "what changed since last release"

The agent computes `git log <last-tag>..HEAD --oneline` and the file diff for the same range, then:

- For each `feat:` commit: identifies the command/area affected, adds one deeper exercise (more flags, more `--tool` values).
- For each `fix:` commit: re-runs the buggy scenario from the commit message / PR body to confirm the fix is live.
- For each `refactor:` commit: runs the regression suite for the touched commands.

The agent's report has a `## Risk overlay (what changed since vX.Y.Z)` section listing each exercise + outcome.

### 4.3 Performance budgets

The agent measures wallclock for every command (via `/usr/bin/time -p` or `time` in the runbook).

| Threshold | Outcome |
|---|---|
| `< 5 s` | Silent (no note) |
| `5 s ≤ t < 30 s` | PASS-WITH-NOTES — flag in report |
| `t ≥ 30 s` | FAIL — likely hung |

Budgets are calibrated empirically. If a command consistently exceeds 5 s on your machine, either (a) the data has grown — raise the threshold here, or (b) there's a regression — investigate.

### 4.4 Platform scope (v1)

QA runs on **macOS only** for v1. `ps` (macOS-only) is fully exercised; other commands work cross-platform but the agent's runbook assumes macOS tooling (`/usr/bin/time`, BSD-style flags). Linux/Windows QA is future scope — see §9.

## 5. Agent Prompt Template

Copy-paste this into your Claude Code session when you spawn the QA subagent. Fill in `{{VERSION}}` and `{{PR}}`.

```text
You are the omnisess release QA agent. Your job: walk through every
user-facing omnisess command and produce a structured QA report for
release {{VERSION}}, prep PR #{{PR}}.

## Setup (do this once, at the start)

1. cd to the repo root.
2. Build the pre-release binary:
     go build -o ./omnisess-prerelease .
3. Verify it runs:
     ./omnisess-prerelease version
   Confirm the version matches {{VERSION}}.
4. Determine the last release tag:
     last_tag=$(git tag --sort=-v:refname | head -1)
5. Compute the changes since last release:
     git log "$last_tag"..HEAD --oneline
     git diff --stat "$last_tag"..HEAD

## Tests to run

Read TESTING.md §4 in full. Run every test it specifies in 4.1
(baseline) and 4.2 (risk overlay based on the changes you just
computed). For each command, capture: command, wallclock, exit code,
outcome (PASS / PASS-WITH-NOTES / FAIL / SKIPPED), and a one-line
structural observation.

## Data policy

Read TESTING.md §3. Your report MUST NOT contain:
- Session previews, Q&A content, tool-call arguments
- Project paths past the basename
- Full session IDs (use first 6 chars + "…")
- Branch names, email addresses, hostnames
- Any verbatim output excerpt longer than one line

You MAY include: counts, exit codes, wallclock times, structural
observations, boolean assertions, the first line of stderr if
structural (not content-bearing).

## Output

Write the full report to:
   qa-reports/{{VERSION}}.md
following the format in TESTING.md §6.

Emit (do NOT post) the proposed PR-comment status line at the end of
your response, on its own line, in this exact format:
   QA: pass
   QA: pass with N notes
   QA: fail — <one-clause summary>

Do NOT call `gh pr comment` yourself. Return the report path and the
status line; the maintainer will post the comment.

## Cleanup

When done:
   rm ./omnisess-prerelease

Return to the maintainer:
1. Path to the local report.
2. The one-line status string.
3. Nothing else.
```

## 6. Output Format (`qa-reports/<VERSION>.md`)

```markdown
# QA Report — vX.Y.Z (prep PR #NN)

_Generated: <UTC timestamp>_
_Last release: vX.Y-1.Z_
_Host platform: darwin_

## Outcome: <PASS | PASS-WITH-NOTES | FAIL>

<one-paragraph summary>

## Baseline

| Command | Wallclock | Exit | Outcome | Observation |
|---|---|---|---|---|
| version | 0.01s | 0 | PASS | Version reported: vX.Y.Z (matches). |
| list | 0.34s | 0 | PASS | N sessions across M projects. |
| list (error path) | 0.01s | 1 | PASS | Stderr first line: `Invalid --since value: …`. |
| ... | | | | |

## Risk overlay (what changed since vX.Y-1.Z)

`<commit subject>` (fix/feat/refactor) — exercise: `<command>` — outcome: `<PASS|...>` — `<observation>`

## Performance notes

<empty if all < 5s>

## Skipped

- `<command>` — SKIPPED — `<reason>`

## Methodology

This report was generated per TESTING.md §4 by the omnisess QA subagent.
```

## 7. Status-Comment Rubric

The subagent emits a status line as text; the maintainer posts it to the prep PR.

| Outcome | Status line | Action |
|---|---|---|
| **PASS** | `QA: pass` | Merge once human review is also OK. |
| **PASS-WITH-NOTES** | `QA: pass with N notes` | Read the notes. Decide if they're worth blocking on or can land. |
| **FAIL** | `QA: fail — <one-clause summary>` | Do NOT merge. Fix, re-run QA, re-status. |
| **SKIPPED-ONLY** (every command SKIPPED) | `QA: skipped` | Invalid release-prep state — investigate. |

The PR comment is one line. The full report stays at `qa-reports/<VERSION>.md` on your machine.

## 8. Failure Handling

- **FAIL blocks the merge.** No exceptions. If the agent's call is wrong, file an issue + add the false-positive case to §4 + re-run.
- **PASS-WITH-NOTES does NOT block.** You decide whether to merge or fix-first. Document the call in the prep PR's body if you ship anyway ("Accepted note: …").
- **Repeat runs.** If you fix and re-run, the new report overwrites `qa-reports/<VERSION>.md`. Only the final version is the audit record.

## 9. Updating This Doc

Revise §4 / §5 when:

- A new command lands (add it to §4.1 + the prompt's runbook).
- A new source lands (add the synthetic fallback under §3 + §4.1).
- A performance budget consistently lies (raise or lower the threshold in §4.3).
- The agent produces noisy / unhelpful reports (tighten the prompt in §5).

### v1 limitations & future scope

- **macOS only.** Linux/Windows QA needs platform-specific tooling adjustments (different `time` flags, `ps` SKIP becomes `ps` PASS on Linux when we add a Linux source for it, etc.).
- **CLI surface only.** The plugin slash-command files (`commands/omnisess/*.md`) and SKILL.md files are NOT exercised. Future scope: static cross-reference check (every `cmd/*.go` has a matching SKILL.md + plugin.json entry + slash-command file) and optional dynamic invocation via Claude Code's plugin system.
- **Manual invocation.** The agent is invoked by hand via the §5 prompt. If the prompt stabilizes over 3+ releases, promote to a `/qa-release` skill (see §5 of the release process doc once that exists).
- **No agent posting to GitHub.** The subagent emits the status line as text; the maintainer posts. This is a deliberate autonomy boundary; if you trust the agent enough to remove this gate, document it here first.
