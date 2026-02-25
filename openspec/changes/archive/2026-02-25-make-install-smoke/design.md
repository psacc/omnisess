## Context

The omnisess Makefile provides build, test, lint, and coverage targets but has no target to install the binary locally or verify it after installation. Developers must manually run `go install .` and then test the binary by hand. The pre-merge checklist in `git-workflow.md` references a smoke test but provides no automation for it.

## Goals / Non-Goals

**Goals:**
- Add `make install` to install omnisess to `~/go/bin` from local source
- Add `make smoke` to run `omnisess list --limit=1` and verify the installed binary is reachable and functional, failing with a clear PATH hint if not found
- Reference `make smoke` in `docs/process/git-workflow.md` pre-merge checklist
- Include `make install && make smoke` in `README.md` getting-started flow

**Non-Goals:**
- System-wide installation (e.g., `/usr/local/bin`)
- Cross-platform packaging or release automation
- Changing the build or test pipeline
- Any changes to application code

## Decisions

**`make install` runs `go install .`, not `go build` + `cp`**
`go install` is the idiomatic Go way to install a binary to `$GOPATH/bin`. It handles the destination path correctly across environments and is what Go developers expect. Using `go build` + manual `cp` adds complexity with no benefit.

**`make smoke` checks PATH before running**
The smoke target guards against the case where `~/go/bin` is not on PATH by checking `command -v omnisess` first and printing a clear error with the fix (`export PATH="$PATH:$HOME/go/bin"`). Silent failures would be confusing.

**`make smoke` uses `omnisess list --limit=1`**
This is the simplest command that exercises the full binary: argument parsing, source loading, and output formatting. It does not require any specific session data to succeed.

**No new `.PHONY` ordering constraints**
`install` and `smoke` are independent targets. `smoke` intentionally does NOT depend on `install` — they represent separate steps so developers can re-run smoke without reinstalling.

## Risks / Trade-offs

- [Risk] `~/go/bin` not on PATH → Mitigation: `make smoke` detects this and prints the fix explicitly
- [Risk] `go install` silently uses a stale module cache → Mitigation: `go install .` always builds from the local working tree (`.` means current module)
- [Trade-off] `make smoke` requires an actual omnisess session to exist for meaningful output, but `--limit=1` returns gracefully with an empty table if no sessions are found, so no sessions = still a passing smoke test

## Open Questions

_(none)_
