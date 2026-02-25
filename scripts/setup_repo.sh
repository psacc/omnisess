#!/usr/bin/env bash
# setup_repo.sh — Idempotent repository settings bootstrap for omnisess.
#
# Applies GitHub repository settings and branch protection on main.
# Safe to re-run at any time — does not weaken existing stricter settings
# unless --force is passed.
#
# Prerequisites:
#   - gh CLI installed (https://cli.github.com/)
#   - gh CLI authenticated with repo admin access (gh auth login)
#
# Usage:
#   ./scripts/setup_repo.sh [owner/repo] [--force]
#
# Arguments:
#   owner/repo   Optional. Defaults to the repo inferred from `gh repo view`.
#   --force      Overwrite branch protection even if existing settings are
#                more restrictive than what this script applies.

set -euo pipefail

# ── Argument parsing ──────────────────────────────────────────────────────────
FORCE=false
REPO=""
for arg in "$@"; do
  case "$arg" in
    --force|-f) FORCE=true ;;
    *) REPO="$arg" ;;
  esac
done

# ── Prerequisite checks ───────────────────────────────────────────────────────
if ! command -v gh >/dev/null 2>&1; then
  echo "Error: gh CLI is required. Install: https://cli.github.com/"
  exit 1
fi

if ! gh auth status >/dev/null 2>&1; then
  echo "Error: gh CLI is not authenticated. Run: gh auth login"
  exit 1
fi

# ── Resolve owner/repo ────────────────────────────────────────────────────────
REPO="${REPO:-$(gh repo view --json nameWithOwner -q .nameWithOwner)}"
echo "Configuring: $REPO"

# ── 1. Repository settings ────────────────────────────────────────────────────
echo "→ Repo settings (squash-only, auto-merge, auto-delete branch)..."
gh api "repos/$REPO" --method PATCH \
  -f allow_merge_commit=false \
  -f allow_rebase_merge=false \
  -f allow_squash_merge=true \
  -f allow_auto_merge=true \
  -f delete_branch_on_merge=true \
  --silent

# ── 2. Branch protection on main ──────────────────────────────────────────────
echo "→ Branch protection on main (require PRs + linear history)..."

# Guard: if protection already exists, check for settings we'd weaken.
# The GitHub API PUT replaces the full config, so we must be explicit.
existing_protection=$(gh api "repos/$REPO/branches/main/protection" 2>/dev/null || true)

apply_protection() {
  gh api "repos/$REPO/branches/main/protection" \
    --method PUT \
    --silent \
    --input - <<'EOF'
{
  "required_status_checks": null,
  "enforce_admins": false,
  "required_pull_request_reviews": {
    "dismiss_stale_reviews": false,
    "require_code_owner_reviews": false,
    "required_approving_review_count": 0
  },
  "restrictions": null,
  "required_linear_history": true
}
EOF
}

if [[ -n "$existing_protection" ]]; then
  enforce_admins=$(echo "$existing_protection" | python3 -c \
    "import json,sys; d=json.load(sys.stdin); print(d.get('enforce_admins',{}).get('enabled',False))" \
    2>/dev/null || echo "false")
  required_reviews=$(echo "$existing_protection" | python3 -c \
    "import json,sys; d=json.load(sys.stdin); r=d.get('required_pull_request_reviews'); print(r.get('required_approving_review_count',0) if r else 0)" \
    2>/dev/null || echo "0")
  restrictions=$(echo "$existing_protection" | python3 -c \
    "import json,sys; d=json.load(sys.stdin); print('yes' if d.get('restrictions') else 'no')" \
    2>/dev/null || echo "no")

  if [[ "$enforce_admins" == "True" || "$required_reviews" -gt 0 || "$restrictions" == "yes" ]] && [[ "$FORCE" == "false" ]]; then
    echo "  Warning: existing branch protection has stricter settings than this script applies:"
    echo "    enforce_admins=$enforce_admins  required_reviews=$required_reviews  restrictions=$restrictions"
    echo "  Skipping branch protection update to avoid weakening it."
    echo "  Run with --force to overwrite."
  else
    apply_protection
    echo "  Branch protection applied."
  fi
else
  apply_protection
  echo "  Branch protection applied."
fi

# ── 3. Labels ─────────────────────────────────────────────────────────────────
echo "→ Labels..."
gh label create "ai-consensus" \
  --color "0e8a16" \
  --description "Both AI providers approved — auto-merge armed" \
  --force
echo "  ai-consensus"

gh label create "human-review-required" \
  --color "d93f0b" \
  --description "AI review did not reach consensus — human approval needed" \
  --force
echo "  human-review-required"

echo ""
echo "Done."
