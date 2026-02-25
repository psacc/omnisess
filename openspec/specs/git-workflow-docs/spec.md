## ADDED Requirements

### Requirement: Document branch protection enforcement
`docs/process/git-workflow.md` SHALL state that direct push to `main` is blocked by GitHub branch protection, that all changes MUST go through a PR, and SHALL reference `make repo-setup` as the way to apply or refresh the protection settings.

#### Scenario: Developer reads workflow doc
- **WHEN** a developer reads `docs/process/git-workflow.md`
- **THEN** they find an explicit statement that direct push to `main` is prohibited and is enforced at the GitHub level

#### Scenario: New collaborator needs to set up protection
- **WHEN** a new collaborator forks or re-configures the repo
- **THEN** `docs/process/git-workflow.md` directs them to run `make repo-setup` to apply branch protection
