# Release Process

How to cut a versioned omnisess release.

## When to Cut a Release

Cut a release when `main` contains a coherent, user-visible improvement:

- A new source is promoted from stub to working (e.g., Codex)
- A new CLI command or flag reaches stable behavior
- A bug fix that meaningfully improves reliability for existing users
- The plugin or module interface changes in a way users should pin against

Do not release for doc-only changes, internal refactors, or chore commits unless they accompany a user-visible change.

## Semver Rules

omnisess follows [Semantic Versioning](https://semver.org/): `vMAJOR.MINOR.PATCH`.

| Change type | Bump |
|---|---|
| Backwards-compatible bug fix | PATCH (`v0.1.0` → `v0.1.1`) |
| New feature, backwards-compatible | MINOR (`v0.1.1` → `v0.2.0`) |
| Breaking CLI flag, interface, or module path change | MAJOR (`v0.x` → `v1.0.0`) |

While the version is `v0.x.y`, minor bumps may include breaking changes — this signals an unstable API period. Move to `v1.0.0` when the Source interface and CLI surface are stable.

## Step-by-Step Release Instructions

The release flow is **two stages**:

1. **Release-prep PR** — bump `SKILL.md` versions, run agentic QA, get human review, merge.
2. **Tag + publish** — create the git tag and the GitHub release from `main`.

The two stages are separate because branch protection blocks committing at release time; the SKILL.md version bump and the QA gate must happen on a PR that merges into `main` before tagging.

### Prerequisites

- `gh` CLI installed and authenticated (`gh auth status`)
- `main` is clean and `make check` passes
- You are on `main` with the commit you want to release checked out
- Go 1.22+ installed (needed for `go build` during the QA step)

### Stage 1 — Release-prep PR

1. Confirm `main` is ready:

   ```bash
   git checkout main
   git pull origin main
   make check
   ```

2. Decide the version number using the semver rules above. Use the format `vX.Y.Z`.

3. Create the release-prep branch and bump SKILL.md versions:

   ```bash
   git checkout -b release-prep-v0.2.0
   make bump-skills VERSION=v0.2.0
   git commit -am "chore(release): bump SKILL.md versions to v0.2.0"
   ```

   `make bump-skills` rewrites the `metadata.version` field in every `SKILL.md` (root + per-skill) to match the tag. The `make release` target hard-fails if any `SKILL.md` is out of sync, so this step is mandatory.

4. Open the prep PR using the standard template:

   ```bash
   git push -u origin release-prep-v0.2.0
   gh pr create --title "chore(release): bump SKILL.md versions to v0.2.0" --body-file .github/pull_request_template.md
   ```

   Fill in the PR template body. Wait for CI green.

5. **QA gate — run the agentic QA per [`../../TESTING.md`](../../TESTING.md).**

   The agent installs the pre-release binary, walks every user-facing command (baseline + risk overlay based on what changed since the last tag), and writes a structured report to `qa-reports/v0.2.0.md` (local, gitignored). It emits a one-line status (`QA: pass` / `QA: pass with N notes` / `QA: fail — <reason>`) which you post as a comment on the prep PR.

   **`QA: fail` blocks the merge.** Fix, re-run the agent, re-post the status.

6. Get the prep PR approved and merge it.

### Stage 2 — Tag + publish

7. Update `main` and confirm it is clean:

   ```bash
   git checkout main
   git pull origin main
   make check
   ```

8. Create the annotated tag and push it:

   ```bash
   make tag VERSION=v0.2.0
   ```

   This creates an annotated git tag on the current commit and pushes it to origin. Verify the commit SHA before proceeding.

9. Publish the GitHub Release:

   ```bash
   make release VERSION=v0.2.0
   ```

   This runs `make tag` (idempotent if already done) then calls `gh release create v0.2.0 --generate-notes`. GitHub auto-generates release notes (a flat PR list) from commits since the previous tag.

10. **Replace the auto-generated notes with a hand-written body.** The flat PR list is a starting point, not the deliverable. Use the [Release-notes template](#release-notes-template) below.

    ```bash
    gh release edit v0.2.0 --notes-file /tmp/v0.2.0-notes.md
    ```

11. Confirm the release is live:

    ```bash
    git tag --list | sort -V   # confirms local tag
    gh release view v0.2.0     # confirms GitHub release, hand-written notes visible
    ```

## Release-notes template

Every release notes body MUST follow this structure. Skip a section only if it has no entries.

```markdown
# v<MAJOR.MINOR.PATCH>

<One-paragraph headline — the 1–2 things a reader cares about most.>

## ✨ New features

- One bullet per user-visible addition. Lead with the command/flag/feature, then a one-line "what it does". Link the PR (#NN).

## 🚀 Perf

- One bullet per measurable speedup or memory reduction. Include before/after numbers in qualitative form (e.g. "~50s → single-digit seconds"), no raw corpus aggregates. Link the PR.

## 🐛 Bug fixes

- One bullet per fixed bug. Reference issue + PR. (#issue, #PR)

## Behind the scenes

- Refactors, parser enrichments, CI/coverage improvements that don't change user-visible CLI but matter to contributors. Optional section.

## Known issues

- Issues we're shipping with and tracking as follow-ups. One bullet each, link the GH issue (#NN). Be honest — concealing known issues damages trust more than acknowledging them.

## Upgrade notes

- Backwards-compat statement (default: CLI surface and module API are backwards-compatible).
- New on-disk artifacts (cache files, config defaults).
- Anything a user has to do manually (migrations, env vars).

**Full changelog**: https://github.com/psacc/omnisess/compare/v<previous>...v<this>
```

### Style rules

- **Privacy first.** No real session UUIDs, real corpus aggregates (use "~1k sessions" or "multi-thousand-session corpus", not exact counts), real `/Users/<name>/` paths. The release notes are the most-public artifact of the repo.
- **Action over implementation.** "Stats command shows tool counts and file activity per session" beats "added new internal/index package and cobra wiring".
- **Numbers, not adjectives.** "5–11× speedup" beats "much faster". But qualitative ranges, not specific user data.
- **Link everything.** Every bullet that references work should link the PR and/or issue.
- **One-paragraph headline.** The summary at the top is the thing many users read and nothing else. Make it count.
- **Known issues belong in release notes**, not just in the tracker. Surfacing them up-front is part of the contract with users.

### Past releases as examples

- [v0.8.0](https://github.com/psacc/omnisess/releases/tag/v0.8.0) — first release using this template
- Earlier releases (v0.4.x–v0.7.x) used auto-generated notes only; they remain as-is.

## Version History

| Version | Date | Notes |
|---|---|---|
| v0.1.0 | 2026-02-24 | First release: working Claude and Cursor sources, public module, Claude Code plugin |
| v0.2.0 | 2026-02-25 | (backfill) Early plugin / source iterations |
| v0.3.0 | 2026-03-14 | (backfill) — see GitHub release notes |
| v0.3.1 | 2026-03-18 | SKILL.md frontmatter fix for skills.sh publishing; .gitignore expansion |
| v0.4.0 | 2026-04-20 | `omnisess ps` (macOS) — live Claude process tree with ancestor lineage; procsnap package |
| v0.4.1 | 2026-04-20 | `--version` flag, `ps` registered in plugin.json, automated release SKILL bumps |
| v0.5.0 | 2026-05-08 | `omnisess skills audit`; coverage gate raised to 100%; ci pin golangci-lint v2.12.1 |
