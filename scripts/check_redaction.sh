#!/usr/bin/env bash
# check_redaction.sh — block private references from entering this public repo.
#
# Generic rules (run everywhere, including CI):
#   1. /Users/<name> and /home/<name> paths must use a placeholder name
#   2. Claude-style encoded paths (Users-<name>-...) must use a placeholder
#   3. Email addresses must be on the domain allowlist
#   4. UUIDv7-looking session IDs must use a synthetic allowed prefix
#
# Machine-local rule (developer machines only, never committed):
#   5. If $GIT_DIR/info/redaction-denylist exists, each non-empty line is a
#      case-insensitive literal that must not appear in any tracked file.
#      This lets a developer enforce terms that cannot themselves be named
#      in a public script.
#
# Usage:
#   scripts/check_redaction.sh            # scan all tracked files
#   scripts/check_redaction.sh --msg FILE # scan a commit-message file instead
set -euo pipefail

# Placeholder usernames allowed in example paths (docs, comments, fixtures).
# Extend only in a reviewed PR — never add a real username.
PLACEHOLDERS='example|foo|bar|foobar|foo\.bar|me|x|u|testuser|user|\.\.\.|\.claude'
# Email domains that may appear in tracked content.
EMAIL_ALLOW='@example\.com|@example\.org|@anthropic\.com|@users\.noreply\.github\.com|@sacconier\.net|@github\.com'
# Synthetic UUIDv7 prefixes allowed in fixtures and docs examples.
UUID_ALLOW='01900000|019eb000|019e0000'

MSGFILE=""
if [ "${1:-}" = "--msg" ]; then
  MSGFILE="$2"
fi

# hits <extended-regex> — print every match of the pattern in the scan scope.
hits() {
  if [ -n "$MSGFILE" ]; then
    grep -ohE "$1" "$MSGFILE" || true
  else
    # -I skips binaries; the rules file itself is excluded.
    git grep -IohE "$1" -- . ':!scripts/check_redaction.sh' || true
  fi
}

fail=0
report() { # report <description> <matches>
  if [ -n "$2" ]; then
    echo "redaction-check: $1:"
    echo "$2" | sed 's/^/  /'
    fail=1
  fi
}

report "non-placeholder /Users or /home path" \
  "$(hits '/(Users|home)/[A-Za-z0-9._-]+' | grep -vE "^/(Users|home)/(${PLACEHOLDERS})$" | sort -u || true)"

report "non-placeholder encoded path (Users-<name>)" \
  "$(hits 'Users-[A-Za-z0-9.]+' | grep -vE "^Users-(${PLACEHOLDERS})$" | sort -u || true)"

report "email address outside allowlist" \
  "$(hits '[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[a-z]{2,}' | grep -viE "(${EMAIL_ALLOW})$" | sort -u || true)"

report "UUIDv7-looking ID with non-synthetic prefix" \
  "$(hits '01[0-9a-f]{6}-[0-9a-f]{4}-7[0-9a-f]{3}-[0-9a-f]{4}-[0-9a-f]{12}' | grep -vE "^(${UUID_ALLOW})" | sort -u || true)"

# Machine-local denylist (one case-insensitive literal per line; lives under
# $GIT_DIR/info/ so it is never tracked or pushed). Matched terms are not
# echoed — they are private by definition.
DENYLIST="$(git rev-parse --git-dir)/info/redaction-denylist"
if [ -f "$DENYLIST" ]; then
  while IFS= read -r term; do
    [ -z "$term" ] && continue
    found=""
    if [ -n "$MSGFILE" ]; then
      grep -iqF "$term" "$MSGFILE" && found=yes || true
    else
      git grep -IiqF "$term" -- . ':!scripts/check_redaction.sh' && found=yes || true
    fi
    if [ -n "$found" ]; then
      echo "redaction-check: machine-local denylist term found (term not echoed; see $DENYLIST)"
      fail=1
    fi
  done < "$DENYLIST"
fi

if [ "$fail" -ne 0 ]; then
  echo "redaction-check: FAILED — remove private references before committing."
  exit 1
fi
echo "redaction-check: OK"
