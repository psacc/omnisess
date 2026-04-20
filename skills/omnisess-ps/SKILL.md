---
name: ps
description: |
  Show live Claude Code sessions as a process tree, grouped by shared ancestor
  chain up to launchd. Covers both CLI sessions and Claude Desktop agent mode.
  Use this skill when asked about the process tree, ancestor lineage, which
  terminal or tmux window a Claude session is running in, or what's currently
  running.
  Trigger phrases: "process tree", "what's running", "lineage", "ancestor chain",
  "which terminal is my claude in", "tmux tree", "claude process tree",
  "session lineage".
  macOS-only: on other platforms the command prints "unsupported" and exits 0.
license: MIT
allowed-tools: Bash
argument-hint: ""
metadata:
  version: 0.4.1
---

# omnisess ps

Show live Claude Code sessions as a process tree with ancestor lineage.

## Usage

```bash
omnisess ps
```

JSON output (machine-readable, includes full ancestor chain with command args):

```bash
omnisess ps --json
```

## Instructions

1. Check that the `omnisess` binary is available:

```bash
if ! command -v omnisess &>/dev/null; then
  echo "omnisess not found. Install it with:"
  echo "  go install github.com/psacc/omnisess@latest"
  exit 1
fi
```

2. Run the command:

```bash
omnisess ps "$@"
```

## Notes

- **macOS-only.** On other platforms the command prints
  `omnisess ps: unsupported on this platform (macOS only)` and exits 0.
- Detects CLI Claude sessions (launched from a terminal, tmux, etc.) and
  Claude Desktop agent-mode sessions.
- Tree is rooted at shared ancestors (typically `/sbin/launchd` and `tmux`)
  so you can see which terminal, tmux pane, or desktop app hosts each session.

## Example output

```
/sbin/launchd (1)
├─ /Applications/Cl (24125)
│  ├─ /Applications/Cl (24780)
│  │  └─ claude  c5b1bd94  my-project (c5b1bd94)  desktop  27s
│  └─ /Applications/Cl (44287)
│     └─ claude  12f1b011  my-app (12f1b011)  desktop  4h
└─ tmux (9050)
   ├─ /bin/bash (23512)
   │  └─ claude  b9f52b71  my-app (b9f52b71)  cli  58s
   └─ claude  232347cd  ai-assistant-pyra (232347cd)  cli  12m
```
