---
name: digest
description: |
  Print AI coding sessions for a calendar day as Obsidian-compatible markdown
  with the full Q&A content of every session. Intended for daily-note workflows:
  paste the output into a daily journal to preserve a searchable record of what
  was discussed with which AI tool.
  Use this skill when asked to dump today's sessions as markdown, prepare a
  daily-note entry, archive Q&A for Obsidian, or export a calendar-day's
  coding conversations.
  Trigger phrases: "digest today's sessions", "dump sessions as markdown",
  "obsidian daily note", "archive my coding chats", "export today's Q&A",
  "session log for daily note".
license: MIT
allowed-tools: Bash
argument-hint: "[--date YYYY-MM-DD] [--tool <name>] [--limit N]"
metadata:
  version: 0.9.0
---

# omnisess digest

Print sessions for a calendar day with full Q&A content as Obsidian-compatible markdown.

## Usage

```bash
omnisess digest [--date YYYY-MM-DD] [--tool <name>] [--limit N]
```

- Defaults to **today** when no `--date` is given (daily-note helper).
- Honors all standard filters: `--tool`, `--project`, `--exclude-project`, `--limit`.
- Output is rune-safe UTF-8 (Italian accents, emoji, CJK render correctly).

## Output format

```markdown
## AI sessions — 2026-05-09  (3 sessions)

### First user message (truncated to 80 chars)
_project: `~/dev/myproject` · tool: claude_

**Q [09:00]:** user question...

_[tools: Bash, Read]_

**A [09:01]:**
assistant response...

---

### Next session title
...
```

Features:
- Sessions separated by `---` and sorted by start time.
- Tool-call-only assistant turns collapsed to `_[tools: Name, …]_`.
- Turns truncated at 2000 runes with a `_(truncated)_` marker.
- `$HOME` substituted with `~` (only when project equals or sits below `$HOME`).

## Instructions

1. Check that the `omnisess` binary is available:

```bash
if ! command -v omnisess &>/dev/null; then
  echo "omnisess not found. Install it with:"
  echo "  go install github.com/psacc/omnisess@latest"
  exit 1
fi
```

2. Run the command, passing through all arguments:

```bash
omnisess digest "$@"
```

## Examples

```bash
# Today's sessions across all tools (default)
omnisess digest

# A specific day
omnisess digest --date 2026-05-09

# One tool only
omnisess digest --tool claude

# Cap to the most recent N sessions of the day
omnisess digest --date 2026-05-09 --limit 5
```
