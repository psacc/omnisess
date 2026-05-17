## Summary

<!-- Up to 3 bullets. Focus on WHY, not what files changed. -->

-
-
-

## Type of change

- [ ] Bug fix
- [ ] New feature
- [ ] Chore
- [ ] Docs

## Test plan

- [ ] `make check` passes
- [ ] `make cover-check` passes (100% per-package)
- [ ] `make smoke` passes
- [ ] Relevant `omnisess` subcommand tested against real data
- [ ] If touching string-slicing or path-prefix logic: includes a non-ASCII or prefix-colliding fixture (the `utf8.ValidString` tripwire in `cmd/cmd_test.go` catches the former)

## Docs sweep — for new commands / new sources / user-visible flag changes

When a PR adds a new `omnisess <command>`, a new source, or a user-visible flag,
**every** item below applies. Tick each one (or `N/A` it with a one-line reason).

- [ ] `README.md` — CLI commands table (`## Commands`) updated
- [ ] `README.md` — plugin commands table (`### Usage`) updated, and the "N slash commands are available" count is correct
- [ ] `SKILL.md` (repo root) — skills table updated and intro `description:` mentions the new capability
- [ ] `skills/omnisess-<name>/SKILL.md` — new skill file added (front-matter + Instructions + Examples)
- [ ] `commands/omnisess/<name>.md` — slash-command stub added (front-matter + Skill invocation line)
- [ ] `.claude-plugin/plugin.json` — both `commands` and `skills` arrays include the new entry
- [ ] `ARCHITECTURE.md` — codemap (`cmd/*` line and per-file Package Map entry) updated
- [ ] `--help` text reads cleanly and matches the command's actual behavior

For a NEW SOURCE additionally:
- [ ] `internal/source/<name>/` self-contained, no cross-source imports
- [ ] `source.Register()` called in source `init()`; blank import added in `cmd/root.go`
- [ ] `model.Tool<Name>` constant in `internal/model/session.go`
- [ ] `docs/references/<name>-format.md` file-format spec added
- [ ] Source's name appears in the registry-driven tests in `cmd/cmd_test.go` (drift guards run automatically — confirm they pass)

## OpenSpec

<!-- Change name if this implements an OpenSpec change, otherwise "N/A" -->

## Breaking changes

- [ ] Yes — describe below
- [ ] No

<!-- If yes, describe the breaking change and migration path -->
