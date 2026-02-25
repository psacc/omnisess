## 1. Fix Makefile pr target

- [ ] 1.1 Replace `gh pr create --fill` with `gh pr create --title "$(git log -1 --format='%s')" --body-file .github/pull_request_template.md` in the `pr` target

## 2. Update git-workflow.md

- [ ] 2.1 Add rule to §5 Review and Merge Rules: "PR body must populate all sections of `.github/pull_request_template.md` before merge"
- [ ] 2.2 Fix the manual `gh pr create` example in §5 to remove `--fill`

## 3. Update CLAUDE.md session loop

- [ ] 3.1 Add to step 7 (Commit & merge): instruction to compose the PR body from `.github/pull_request_template.md`, filling Summary, Type, Test plan, OpenSpec, and Breaking changes sections

## 4. Verification

- [ ] 4.1 Run `make check` and confirm it passes
- [ ] 4.2 Confirm all three files are changed correctly
