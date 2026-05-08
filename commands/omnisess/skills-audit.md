---
description: Classify skills by usage (Keep / Borderline / Archive). Read-only audit driven by Claude Code session JSONL.
argument-hint: "[--root <path>...] [--window 30d|90d|180d] [--allowlist <path>] [--format markdown|json] [-o <file>]"
allowed-tools: Bash
---

Invoke the omnisess:skills-audit skill (using the Skill tool) and follow its instructions.
