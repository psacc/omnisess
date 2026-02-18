# Agent-to-Agent Review: Decision Rules

Rules for subagents to autonomously resolve design decisions. Follow mechanically.

## 1. Decision Classification

### Two-way door (agent decides)

ALL of these must be true:

- Revertable with a single `git revert` of one commit
- Does NOT change: `Source` interface, CLI flag names/behavior, file format parsing semantics
- Does NOT add external dependencies (new `go.mod` entries)
- Blast radius is one package or less
- A similar pattern already exists in the codebase, OR the solution is ≤30 lines

Examples: adding a fallback path, changing a threshold, adding an internal helper function, refactoring internal logic, choosing between two approaches when one is already used in the codebase.

### One-way door (escalate to human)

ANY of these is true:

- Changes the `Source` interface or `model.*` types
- Changes CLI flag names, subcommand behavior, or output format contracts
- Changes file format parsing in ways that break existing data
- Adds a new external dependency
- Affects more than two packages
- Cannot be reverted without data migration or user communication

Examples: changing `source.Source` method signatures, renaming CLI flags, changing JSONL parsing to reject previously-accepted input, adding a new third-party library.

## 2. Review Protocol

When a subagent encounters a design decision during implementation:

1. **Classify** using the criteria above. Check every bullet — not a gut feeling.
2. **Two-way door** → Implement it. In the commit message, add: `Decision (agent-decided): <what and why>`.
3. **One-way door** → Stop. Write up using the escalation format (section 4). Tag `needs-human` in the exec plan or investigation report.
4. **Uncertain** → Request a reviewer agent. Provide: the decision, the two-way/one-way checklist results, and which bullets you're unsure about.

## 3. Reviewer Agent Checklist

When reviewing a decision another agent is unsure about, check in order. First match wins.

| # | Check | Result |
|---|-------|--------|
| 1 | Does the proposed change modify any public interface (`Source`, `model.*` types, CLI flags)? | Yes → **escalate** |
| 2 | Does a similar pattern already exist in the codebase? (`grep` for it) | Yes → **approve** |
| 3 | Is it revertable with a single `git revert`? | No → **escalate** |
| 4 | If we're wrong, is rework > 1 hour? | Yes → **escalate** |
| 5 | Does it add an external dependency? | Yes → **escalate** |
| 6 | None of the above triggered | → **approve** |

After deciding, the reviewer writes a one-line rationale citing the check number: `Approved per check #2: pattern exists in internal/source/claude/parser.go`.

## 4. Escalation Format

When escalating to human, use exactly this format:

```
**Decision needed**: <one-line summary>
**Classification**: One-way door
**Options**:
  A) <option> — <tradeoff>
  B) <option> — <tradeoff>
**Recommendation**: <which option and why>
**Reversibility**: <what happens if we pick wrong>
```

Place this in the exec plan under a `## Decisions` section, or in the investigation report if no exec plan exists.

## 5. Post-Decision Tracking

- Two-way agent decisions: recorded in commit messages (searchable via `git log --grep="agent-decided"`)
- One-way escalations: recorded in exec plans or investigation reports under `## Decisions`
- Reviewer approvals: recorded in commit messages (`Reviewed (agent-review): <rationale>`)
