# TUI Quality Gates: Lessons Learned

Agent-facing guidance for quality-gating TUI features. Derived from the first TUI session picker iteration where the babysitter process scored 94.5/100 but shipped three bugs caught by manual user testing.

## Bugs That Escaped

### 1. Off-by-one in column layout math

**Symptom**: "STATUS" header truncated to "STATU" at terminal edge.

**Root cause**: `previewWidth()` computed fixed column widths but miscounted inter-column spaces (3 instead of 4). The dynamic PREVIEW column was 1 char too wide, pushing STATUS off-screen.

**Why the quality gate missed it**: The scorer verified "View() renders all 5 columns" by checking substring presence in the output string. It did not verify that the rendered width fit within the terminal width. The smoke test ran in a wide terminal where 1 extra char didn't clip.

**Fix for future processes**:
- Add a **width budget test**: assert that `renderRow()` output width <= terminal width for common widths (80, 120).
- Test column header and row alignment at exactly 80 columns — the width where off-by-one errors become visible.
- Quality scorer acceptance criteria should include: "rendered output fits within `m.width` without truncation."

### 2. No fallback for empty Preview field

**Symptom**: Sessions with no preview showed blank space, making them look identical to each other.

**Root cause**: `renderRow()` used `s.Preview` directly without a fallback. Many sessions (especially short or aborted ones) have `Preview == ""`.

**Why the quality gate missed it**: Test fixtures and real smoke-test data happened to have previews populated. No test case exercised `Preview == ""`. The scorer's check for "columns rendered" didn't test empty-field edge cases.

**Fix for future processes**:
- **Always test with empty/zero values** for every displayed field. Add explicit test cases: `Preview: ""`, `Project: ""`, `Tool: ""`.
- Quality scorer should require: "test cases include empty/missing values for all displayed fields."

### 3. Resume fails when CWD differs from session's project

**Symptom**: `claude --resume <id>` returns "No conversation found" because Claude Code scopes session lookup to the current working directory's project.

**Root cause**: `resumeClaude()` called `syscall.Exec` without first `os.Chdir`-ing to the session's project directory.

**Why the quality gate missed it**: The scorer verified that `resumeClaude()` calls `syscall.Exec` with correct arguments (static analysis). It couldn't actually test the exec (process replacement). The external system constraint — Claude Code resolving sessions relative to CWD — was not in the acceptance criteria.

**Fix for future processes**:
- When integrating with external CLIs via `exec`, **document CWD and environment requirements** in acceptance criteria.
- Add a section to the design review breakpoint: "What assumptions does the external tool make about working directory, environment, or filesystem state?"
- Quality scorer should verify: "exec calls set up the expected working directory and environment."

## General Process Improvements

### Acceptance criteria gaps

The babysitter process used a quality-convergence loop (target 85/100) with 9 criteria. The criteria focused on **structural correctness** (files exist, tests pass, columns render) but not **behavioral correctness at boundaries** (terminal width, empty data, external tool constraints).

**Recommendation**: Future TUI quality gates should include:
1. **Boundary rendering tests**: Render at width=80 and assert no truncation.
2. **Empty/missing data tests**: Every displayed field tested with zero value.
3. **External integration tests**: Document and verify CWD, env, and filesystem assumptions for all exec/system calls.
4. **Manual test script**: Generate a checklist of manual checks that cannot be automated (e.g., "resize terminal to 80 cols and verify STATUS column is visible").

### Smoke test limitations

The smoke test ran against real session data, which is good for validating parsing and sorting. But real data is self-selected — it tends to have populated fields and doesn't exercise edge cases. Future smoke tests should inject at least one synthetic session with minimal/empty fields.
