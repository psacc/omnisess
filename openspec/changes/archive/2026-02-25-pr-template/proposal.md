## Why

PRs in this repo lack a consistent structure, leading to reviewers missing context on motivation, missing test evidence, or unclear scope. A standardized template enforces quality at submission time with zero ongoing overhead.

## What Changes

- Add `.github/pull_request_template.md` with structured sections for summary, change type, test plan, OpenSpec traceability, and breaking changes

## Capabilities

### New Capabilities

- `pr-template`: GitHub PR template that standardizes pull request quality by requiring a why-focused summary, change type classification, mandatory test checklist, OpenSpec change name, and breaking change declaration

### Modified Capabilities

_(none)_

## Impact

- `.github/pull_request_template.md`: new file, auto-populated by GitHub when opening a PR
- No code changes, no dependencies, no API changes
