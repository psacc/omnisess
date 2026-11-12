# Test gate failure propagation

## Scope

Preserve failing `go test` status through readable output and correct the slow-test
parser's duration and name fields. No CLI or environment behavior changes.

## Plan

1. Reproduce false success with temporary failing and slow Go tests.
2. Enable pipefail only for the test pipeline and correct the awk fields.
3. Repeat probes, remove them, and run check and coverage gates independently.
4. Review and commit locally; publication and merge remain separate gates.

## Decisions

Two-way door: a pipeline-local Bash invocation avoids changing every recipe's
shell. Parse indented result lines too, so named subtests obey the same budget.

## Validation

Pending local before/after probes and clean gates.

## Completed validation

- Pinned Go 1.25.0 and golangci-lint 2.14.0 through mise; GNU make.
  Unset `OMNISESS_AXI`; enabled CGO and external linking for the local C compiler
  required by the race runtime. These are verification-only environment settings.
- Temporary `TestGateFailureProbe`: baseline `make check` exit 0 with FAIL output;
  fixed exit 2 with the same failing test.
- Temporary `TestGateSlowProbe`: passing test at 2.10s, threshold 2.0s;
  baseline exit 0, fixed exit 2 with correct test name and duration.
- The restored default slow gate exposed three pre-existing 1.1s sleeps in index
  tests. Replaced timestamp waits with explicit old database timestamps; removed
  an unused wait in the downgrade test. Assertions and the default budget remain.
- Final default `make check`: exit 0, no FAIL output.
- Independent `make cover-check`: exit 0, no FAIL output, all enforced packages 100%.
- Built branch binary and ran `list --limit=1` against local data: exit 0.
- Reviewer found no defects, approved per agent-review check #6.
- Temporary probes removed. Detailed logs retained outside the public repository.

Local implementation complete. Publication, CI, owner approval, merge, primary
refresh, warning cleanup and installation are pending their delivery gates.
