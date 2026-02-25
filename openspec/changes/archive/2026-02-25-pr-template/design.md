## Context

GitHub uses `.github/pull_request_template.md` to auto-populate the PR body when a contributor opens a PR. This repo has no such file. PRs arrive with ad-hoc or empty bodies, making review harder and causing test evidence to be omitted.

## Goals / Non-Goals

**Goals:**
- Add a single PR template file that GitHub uses automatically
- Enforce a why-focused summary (3 bullets max)
- Require test evidence via mandatory checkboxes
- Surface OpenSpec change name for traceability
- Require explicit breaking-change declaration

**Non-Goals:**
- Multiple templates (feature / bugfix split)
- CI enforcement of template completeness
- Changing any existing workflow tooling

## Decisions

**Single template, not per-type templates**
GitHub supports multiple templates via a `PULL_REQUEST_TEMPLATE/` directory, but that requires contributors to manually select one. A single template with a type-of-change checkbox section achieves the same classification with less friction.

**Checkboxes for test plan, not free text**
Free text test plans are routinely skipped or written as noise. Checkboxes create a binary contract: you check them or reviewers see unchecked boxes. `make check` and `make smoke` are the two mandatory items because they cover lint/test and binary validation respectively.

**OpenSpec section included**
Since this repo uses OpenSpec, including a "Change name" field in the template keeps traceability without requiring a separate process.

## Risks / Trade-offs

- [Risk] Contributors ignore the template → Mitigation: checkboxes are visible and unchecked state is obvious to reviewers; no CI enforcement needed at this stage
- [Trade-off] Template adds ~20 lines to PR body → Acceptable; GitHub collapses the template into the editor and reviewers are familiar with the pattern

## Open Questions

_(none)_
