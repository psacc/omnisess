---
name: skills-audit
description: |
  Classify agent skills by usage tier (Keep / Borderline / Archive / Unknown) using
  Claude Code session JSONL transcripts as the signal source. Read-only — never
  modifies skills, sessions, or any file on disk.
  Use this skill when asked to audit skills, find unused skills, classify skills,
  check which skills get used, produce a skill usage report, or prune skills.
  Trigger phrases: "audit skills", "find unused skills", "classify skills",
  "which skills do I use", "skill usage report", "prune skills",
  "what skills are unused", "skills I can remove", "trim my skill list".
  Output is a recommendation (Keep / Borderline / Archive) — acting on it is always
  a manual step.
license: MIT
allowed-tools: Bash
argument-hint: "[--root <path>...] [--window 30d|90d|180d] [--allowlist <path>] [--format markdown|json] [-o <file>]"
metadata:
  version: 0.8.0
---

# omnisess skills audit

Classify agent skills by usage tier by joining the discovered skill inventory
against invocation counts scraped from Claude Code session JSONL transcripts.

## Usage

```
/omnisess:skills-audit [--root <path>...] [--window 30d|90d|180d] [--allowlist <path>] [--format markdown|json] [-o <file>]
```

## Instructions

### Step 1: Binary guard

```bash
if ! command -v omnisess &>/dev/null; then
  echo "omnisess not found. Install it with:"
  echo "  go install github.com/psacc/omnisess@latest"
  exit 1
fi
```

### Step 2: Build the command

Assemble the `omnisess skills audit` invocation from the arguments provided:

- `--root <path>` (repeatable) — project root(s) to walk for SKILL.md files.
  If none are supplied and the user mentioned a directory, use that. If still
  none, omit the flag — globals-only mode (all of `~/.claude/skills/`,
  `~/.agents/skills/`, `~/.claude/plugins/**/skills/`).
- `--no-globals` — add only if the user explicitly wants to skip global locations.
- `--allowlist <path>` — add only if the user supplied an allowlist file.
- `--window <value>` — one of `30d`, `90d`, `180d`. Default `90d` if not stated.
- `--format <value>` — `markdown` (default, human-readable) or `json` (machine).
- `-o <file>` — output file; omit to write to stdout.

### Step 3: Run the audit

```bash
omnisess skills audit [assembled flags]
```

If the command exits non-zero, print the error and stop.

### Step 4: Present results

- If `--format markdown` (or default): render the output directly — it is already
  structured markdown with tier sections.
- If `--format json`: summarise the tier counts from the JSON and offer to render
  specific sections the user asks for.

After presenting, add a one-line reminder:

> _Archive is a recommendation. Review paths before removing any skill._

## Arguments

| Flag | Default | Description |
|---|---|---|
| `--root <path>` | (none) | Project root to walk (repeatable). Omit to use globals only. |
| `--no-globals` | off | Skip `~/.claude/skills`, `~/.agents/skills`, `~/.claude/plugins/**/skills/` |
| `--allowlist <path>` | (none) | Plain-text file: one skill name per line, `#` for comments |
| `--window 30d\|90d\|180d` | `90d` | Telemetry look-back window |
| `--format markdown\|json` | `markdown` | Output format |
| `-o <file>` | stdout | Write output to file instead of stdout |

## Tier rules (summary)

| Tier | Criteria |
|---|---|
| **Keep** | ≥ 3 invocations in window, or skill is in the allowlist |
| **Borderline** | 1–2 invocations in window |
| **Archive** | 0 invocations and not allowlisted |
| **Unknown** | Skill from a non-Claude source (Codex, Cursor, Gemini); telemetry not parsed in v0 |

## Examples

```bash
# Scan one project root, 90-day window (default)
omnisess skills audit --root ~/prj/myapp

# Multiple roots + custom window
omnisess skills audit --root ~/prj/myapp --root ~/prj/shared --window 30d

# Force-keep specific skills, output to file
omnisess skills audit --root ~/prj --allowlist ~/.config/omnisess/allowlist.txt --format json -o audit.json
```

## When NOT to use

- **Plugin-bundled skills**: skills inside a `.claude-plugin/` bundle cannot be
  deleted individually — removing one requires editing `plugin.json` and
  re-packaging. The audit will correctly classify them, but acting on Archive
  suggestions for bundled skills is a separate workflow.
- **Non-Claude sources (v0)**: Codex, Cursor, and Gemini skills appear as
  `Unknown` — invocation telemetry for those tools is not parsed in v0.
  Do not use the Archive tier for Unknown skills.
- **Window too narrow**: a 30-day window will mark quarterly or incident-response
  skills as Archive. Use `--allowlist` or widen `--window` for infrequently-used
  but intentional skills.

## Notes

- Read-only. The audit never modifies skills, sessions, the allowlist, or any
  other file on disk.
- Ghost usage entries (skill names seen in JSONL but not discovered on disk) mean
  `--root` coverage is incomplete — add more `--root` flags.
- Unmatched allowlist entries indicate renamed or deleted skills — clean them up.
- Full guide: `docs/skills-audit.md` in the omnisess repo.
