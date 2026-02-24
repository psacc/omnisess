## Why

Developers working on omnisess have no standardized way to install the binary locally or verify it works after a build. The existing Makefile covers build, test, and lint, but the workflow gap between `go build` and validating the installed binary leaves the smoke-test step manual and undocumented.

## What Changes

- Add `make install` target: runs `go install .` to install omnisess from local source into `~/go/bin/omnisess`
- Add `make smoke` target: runs `omnisess list --limit=1` to verify the installed binary is reachable and functional; fails with a clear hint if `omnisess` is not on PATH
- Update `docs/process/git-workflow.md` pre-merge checklist smoke test step to reference `make smoke`
- Update `README.md` getting-started section to include `make install && make smoke` in the setup flow

## Capabilities

### New Capabilities

- `makefile-install-smoke`: Makefile targets for local installation (`make install`) and smoke testing (`make smoke`) of the omnisess binary

### Modified Capabilities

_(none — no existing spec-level requirements are changing)_

## Impact

- `Makefile`: two new targets added (`install`, `smoke`)
- `docs/process/git-workflow.md`: pre-merge checklist updated
- `README.md`: getting-started section updated
- No code changes, no API changes, no new dependencies
