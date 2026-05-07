# Skills Audit

## What it is

`omnisess skills audit` walks a set of project roots and global skill locations,
joins the discovered inventory against Claude Code session JSONL transcripts, and
classifies every skill as **Keep**, **Borderline**, **Archive**, or **Unknown**.
A typical Claude Code session loads ~168 skills into the system reminder; each
skill's `description` frontmatter costs tokens at every session start, regardless
of whether the skill is ever invoked. The audit surfaces which skills are genuinely
used (last 90 days, configurable) so you can decide which to remove without guessing.

---

## Why we built it

No harness ships a first-class usage-audit command. Anthropic confirmed the gap in
[issue #35319](https://github.com/anthropics/claude-code/issues/35319) (open as of
2026-05). Skills are workflow-shaped (invocation-scoped), not rules-shaped
(always-on): "did you actually use it in the last 90 days" is a meaningful question.
omnisess is uniquely positioned to answer it — it already parses Claude Code JSONL
transcripts for `list`, `search`, and `active`. The audit reuses that posture without
coupling to the existing `Source` interface.

---

## Cross-tool comparison

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

omnisess fills a real, multi-vendor gap. v0 targets Claude Code only; Codex,
Cursor, and Gemini are v0.5 follow-ups.

---

## Quickstart

```bash
# Minimal: scan project roots, globals on by default
omnisess skills audit --root ~/prj

# With allowlist (force-Keep named skills regardless of usage)
omnisess skills audit --root ~/prj --allowlist ~/.config/omnisess/allowlist.txt

# JSON output to a file (machine-readable, pipe to jq)
omnisess skills audit --root ~/prj --format json -o audit.json
```

Global skill locations scanned by default (disable with `--no-globals`):

- `~/.claude/skills/`
- `~/.agents/skills/`
- `~/.claude/plugins/**/skills/`

Always excluded: `.worktrees/`, `worktrees/`, `node_modules/`, `.venv/`, `.git/`,
`dist/`, `build/`, `.scratch/`.

---

## Read-only guarantee

The audit **never modifies** skills, session files, the allowlist, or anything else.
The Archive tier is a recommendation — acting on it is always a manual step.

---

## Tier rules

Tiering uses a 90-day window by default (`--window 30d|90d|180d`). Both
model-invoked and user-invoked counts contribute to the total.

| Tier | Criteria |
|---|---|
| **Keep** | `total_90d >= 3` OR skill name is in the allowlist |
| **Borderline** | `total_90d` is 1 or 2 (used, but rarely) |
| **Archive** | `total_90d == 0` AND not allowlisted (candidate for removal) |
| **Unknown** | Skill discovered in a non-Claude source (Codex, Cursor, Gemini); telemetry not parsed in v0 |

Cost (description token count) is shown as informational only — it is not a tier
driver. A large-description skill with 0 invocations is Archive; a tiny skill used
daily is Keep. The Archive candidates section sorts by description size so the
highest-value removals appear first.

---

## Allowlist format

Plain text. One skill name per line. `#` starts a comment. Blank lines ignored.
Inline comments (after the skill name) are stripped.

```
# Skills always kept regardless of usage count

security-review        # quarterly, not in 90d window
incident-response
debugging
```

Pass the file with `--allowlist <path>`. There is no default location; the flag
must be explicit. Allowlist entries that do not match any discovered skill are
reported under **Unmatched allowlist** — useful for catching stale entries after
a rename.

---

## Output sections explained

Default output is Markdown to stdout. Use `--format json -o <file>` for machines.

| Section | What it shows |
|---|---|
| **Summary** | Tier counts (Keep / Borderline / Archive / Unknown) and estimated reclaimable tokens if all Archive candidates are removed |
| **Top by usage** | Up to 20 skills sorted by total invocations; model and user counts in separate columns |
| **Archive candidates** | Zero-invocation, non-allowlisted skills; sorted by description size (biggest token wins first); includes path |
| **Borderline** | Skills invoked 1–2 times in the window; review manually |
| **Unknown** | Skills from non-Claude sources; telemetry not available in v0 |
| **Ghost usage** | Skill names found in JSONL invocations but not in the discovered inventory; indicates missing `--root` coverage |
| **Unmatched allowlist** | Allowlist entries with no matching discovered skill; likely renamed or deleted |
| **Provenance** | Roots scanned, globals flag, sessions parsed, omnisess version |

---

## Invocation counting

Two distinct signals are counted and shown as separate columns:

- **Model** — assistant message with `tool_use name=Skill, input.skill=<name>` (the
  harness invokes the skill on the model's behalf)
- **User** — user message containing `<command-name>/<name></command-name>` (slash
  command typed directly)

The two are summed for tiering. Keeping them separate surfaces skills that are
always slash-invoked vs. always model-invoked — useful context when deciding whether
to remove a skill.

---

## v0.5+ roadmap

- **Collision reporting** — when the same skill name is discovered in multiple locations (e.g., both global and project), report the collision in `AuditResult` rather than silently deduplicating. The dedup rule (project beats global) is already implemented in the walker; surfacing the list is deferred to v0.5+.
- **Codex parser** — scan `~/.codex/sessions/**/*.jsonl` once the omnisess Codex
  `Source` lands; promote Codex-discovered skills from Unknown to a real tier
- **Cursor / Gemini parsers** — same; depends on those Sources shipping
- **Scheduled runs** — integrate with omnisess cron/recap loop for weekly drift reports
- **Allowlist patterns / YAML** — glob patterns (`gws-*: keep`) and YAML format for
  richer annotations (owner, last-reviewed date, reason)
- **`omnisess skills list`** — inventory-only subcommand (no telemetry); useful
  without session data

---

## References

- [anthropics/claude-code#35319](https://github.com/anthropics/claude-code/issues/35319) — confirmed telemetry gap, open as of 2026-05
- [mattpocock/vercel-labs skills](https://github.com/vercel/vercel/tree/main/.claude/skills) — reference implementation for `SKILL.md` frontmatter conventions
- [Factory.ai Droid skills doc](https://docs.factory.ai/droids/skills) — closest peer with real telemetry (OTel export, compliance-shaped)
- exec plan: `docs/exec-plans/active/014-skills-audit.md`
