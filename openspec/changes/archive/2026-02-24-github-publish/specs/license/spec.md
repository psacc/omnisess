## ADDED Requirements

### Requirement: MIT LICENSE file exists at repo root
A file named `LICENSE` SHALL exist at the repository root. It SHALL contain the standard MIT License text with the copyright year set to the year of initial public release and the copyright holder set to `psacc`.

#### Scenario: File present after change
- **WHEN** the change is applied
- **THEN** a file named `LICENSE` exists at the repository root

#### Scenario: Content is MIT License
- **WHEN** the LICENSE file is read
- **THEN** the text contains the phrase "MIT License"
- **THEN** the text contains a copyright line with a year and "psacc"
- **THEN** the text contains the standard MIT permission notice (permission is hereby granted, free of charge...)

---

### Requirement: LICENSE file is recognized by GitHub as MIT
GitHub's license detection SHALL identify the LICENSE file as MIT when the repository is viewed.

#### Scenario: GitHub displays MIT badge on repo page
- **WHEN** the repository is pushed to GitHub as a public repo
- **THEN** the repository page displays "MIT license" in the About section or sidebar

---

### Requirement: go.mod does not declare a license
No license field or directive SHALL be added to `go.mod`. License is declared only via the `LICENSE` file at the root, per Go module conventions.

#### Scenario: go.mod unchanged by license addition
- **WHEN** the change is applied
- **THEN** `go.mod` does not contain any license-related directive
- **THEN** `go.mod` is otherwise identical to its pre-change state (no unrelated modifications)
