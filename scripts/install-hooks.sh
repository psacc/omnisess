#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# Resolve the actual .git directory (supports worktrees). For a worktree this
# is something like /path/to/main/.git/worktrees/<name> — hooks installed here
# apply to the worktree only, which is the expected behavior.
GIT_DIR="$(git -C "$REPO_ROOT" rev-parse --git-dir)"
HOOK_DIR="$GIT_DIR/hooks"
mkdir -p "$HOOK_DIR"

for hook in pre-commit pre-push; do
  src="$REPO_ROOT/scripts/$hook"
  dst="$HOOK_DIR/$hook"
  ln -sf "$src" "$dst"
  echo "Installed $hook hook -> $dst"
done
