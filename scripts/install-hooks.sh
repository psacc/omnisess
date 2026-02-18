#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
HOOK_SRC="$REPO_ROOT/scripts/pre-commit"

# Resolve the actual .git directory (supports worktrees)
GIT_DIR="$(git -C "$REPO_ROOT" rev-parse --git-dir)"

# For worktrees, GIT_DIR is something like /path/to/main/.git/worktrees/<name>
# Hooks should go in the worktree's own git dir
HOOK_DST="$GIT_DIR/hooks/pre-commit"

mkdir -p "$(dirname "$HOOK_DST")"
ln -sf "$HOOK_SRC" "$HOOK_DST"
echo "Installed pre-commit hook -> $HOOK_DST"
