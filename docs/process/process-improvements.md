# Process Improvements: Landing Gap Analysis

Agent-facing guidance. Derived from the resume-support babysitter run that completed with success status but left all code uncommitted on the working tree.

## 1. Gap Analysis

### What happened

The `resume-support` process ran to completion: research synthesis, exec plan creation, human review breakpoint, four implementation phases each with quality gates, and returned `{ success: true }`. The babysitter marked the run complete.

Meanwhile, `git status` shows:

```
 M cmd/tui.go
 M internal/tui/model.go
 M internal/tui/model_test.go
?? internal/resume/           (entire new package, never committed)
?? docs/design-docs/resume-modes.md
?? docs/exec-plans/completed/007-multi-resume.md
```

No branch was created. No commits were made. No review happened. No merge to main.

### Why it happened

The process definition (`resume-support.js`) has four stages:

1. Synthesize research -> agent task
2. Create exec plan -> agent task
3. Human review breakpoint
4. Implementation loop (implement + quality gate per phase)

It returns `{ success: true }` after step 4 completes. There is no step 5 for git operations. The process equates "code written + tests pass" with "done."

The same gap exists in `tui-session-picker.js` (five phases ending at smoke test) and `competitive-analysis.js` (though that one produces docs/artifacts, not code, so the gap is less critical).

### Root cause

The process template treats the implementation lifecycle as: plan -> implement -> verify. The actual lifecycle is: plan -> implement -> verify -> **land**. The landing workflow (branch, commit, review, classify, merge) is documented in `git-workflow.md` but is not encoded in any process definition.

This is a **systematic gap**, not a one-off mistake. Every implementation process will have this gap unless the template includes landing as a required terminal phase.

### Contributing factors

1. **Quality gates create false confidence.** A quality gate that says "score: 94/100, tests pass" feels like completion. But it only validates code correctness, not code persistence.
2. **The process return value is the completion signal.** When `process()` returns, the babysitter considers the run done. There is no post-return validation that code was actually landed.
3. **Agent tasks operate on the working tree.** The agents write files and run tests, but no agent is instructed to run git commands. Each agent's prompt says "write code" and "run tests" but never "commit" or "branch."
4. **Breakpoints are mid-process, not end-of-process.** The human review breakpoint approves the plan, not the final code. There is no terminal breakpoint asking "has this been merged?"

## 2. Recommended Process Template Changes

### A. Add a mandatory landing phase to every implementation process

Every process that modifies Go source files, test files, or go.mod MUST include a landing phase as its final stage before returning. This phase runs after all implementation and quality gates pass.

The landing phase is a single agent task that:

1. Creates a branch (`feat/`, `fix/`, or `chore/` per `git-workflow.md`)
2. Stages and commits all changes (conventional commit message)
3. Runs `make check` on the branch
4. Spawns a reviewer subagent against the branch diff
5. Addresses review findings
6. Classifies the change (two-way door vs one-way door per `agent-review.md`)
7. If two-way door: runs `make merge` (squash-merge into main)
8. If one-way door: pushes branch, writes escalation summary, does NOT merge
9. Verifies main is clean after merge

### B. Change the process return contract

The return value should include a `landed` field:

```js
return {
  success: true,
  landed: {
    merged: true,          // or false if escalated
    branch: 'feat/multi-resume',
    commitHash: 'abc1234',
    mergeMethod: 'squash', // or 'escalated'
    escalation: null       // or escalation summary string
  }
};
```

A process that returns `success: true` but `landed.merged: false` without an escalation reason should be flagged as incomplete by the babysitter.

### C. Add a post-return verification hook

After `process()` returns, the babysitter should verify:

- If the process modified Go files: `git status` on main shows no uncommitted changes related to the process.
- If `landed.merged: true`: the commit hash exists on main.
- If `landed.merged: false`: a branch exists with a push, and an escalation summary is present.

### D. Add terminal breakpoint for one-way door landings

When the landing phase classifies a change as one-way door:

```js
await ctx.breakpoint({
  question: 'This change was classified as one-way door. Branch pushed but NOT merged. Review the escalation summary and decide whether to merge.',
  title: 'One-Way Door: Merge Decision',
  context: { ... }
});
```

## 3. Reusable Landing Phase Specification

### Task definition

```js
export const landingTask = defineTask('landing', (args, taskCtx) => ({
  kind: 'agent',
  title: `Land: ${args.featureName}`,
  agent: {
    name: 'general-purpose',
    prompt: {
      role: 'Release engineer for a Go CLI project',
      task: `Land the completed feature "${args.featureName}" into main following the project git workflow.`,
      context: {
        featureName: args.featureName,
        branchPrefix: args.branchPrefix || 'feat',
        filesModified: args.filesModified,
        filesCreated: args.filesCreated,
        commitMessage: args.commitMessage,
        execPlanPath: args.execPlanPath || null,
        classificationHints: args.classificationHints || {}
      },
      instructions: [
        'Read docs/process/git-workflow.md for the full merge workflow',
        'Read docs/process/agent-review.md for decision classification rules',

        '--- BRANCH ---',
        `Create branch: git checkout -b ${args.branchPrefix}/${args.slug}`,

        '--- COMMIT ---',
        'Stage all relevant files (source, tests, docs). Do NOT stage .a5c/ or artifacts/',
        `Commit with conventional message: ${args.commitMessage}`,

        '--- VERIFY ---',
        'Run: make check (must be clean)',
        'If make check fails, fix and re-commit',

        '--- CLASSIFY ---',
        'Check the change against agent-review.md Section 1:',
        '  - Does it change Source interface or model.* types? -> one-way door',
        '  - Does it add external dependencies (go.mod changes)? -> one-way door',
        '  - Does it affect 3+ packages? -> one-way door',
        '  - Otherwise -> two-way door',

        '--- TWO-WAY DOOR ---',
        'If two-way door: run make merge (squash-merge into main)',
        'Verify make check passes on main after merge',
        'Delete the feature branch',
        'If exec plan exists, move it to docs/exec-plans/completed/',

        '--- ONE-WAY DOOR ---',
        'If one-way door: push the branch (git push -u origin <branch>)',
        'Do NOT merge',
        'Write escalation summary per agent-review.md Section 4'
      ],
      outputFormat: 'JSON with merged (boolean), branch (string), commitHash (string), classification (string: two-way|one-way), escalation (string|null), makeCheckPassed (boolean)'
    },
    outputSchema: {
      type: 'object',
      required: ['merged', 'branch', 'classification', 'makeCheckPassed'],
      properties: {
        merged: { type: 'boolean' },
        branch: { type: 'string' },
        commitHash: { type: 'string' },
        classification: { type: 'string' },
        escalation: { type: 'string' },
        makeCheckPassed: { type: 'boolean' }
      }
    }
  },
  io: {
    inputJsonPath: `tasks/${taskCtx.effectId}/input.json`,
    outputJsonPath: `tasks/${taskCtx.effectId}/result.json`
  }
}));
```

### Usage in a process

```js
// After all implementation phases and quality gates pass:

const landingResult = await ctx.task(landingTask, {
  featureName: 'Multi-mode resume support',
  branchPrefix: 'feat',
  slug: 'multi-resume',
  filesModified: allModifiedFiles,
  filesCreated: allCreatedFiles,
  commitMessage: 'feat: add multi-mode resume support with strategy pattern',
  execPlanPath: 'docs/exec-plans/active/008-multi-resume.md',
  classificationHints: {
    changesModelTypes: true,   // added ResumeMode to model
    newDependencies: false,
    packageCount: 3            // resume, tui, cmd
  }
});

if (!landingResult.merged) {
  await ctx.breakpoint({
    question: `Feature classified as one-way door and was NOT merged. Branch: ${landingResult.branch}. Review escalation and decide.`,
    title: 'One-Way Door: Merge Decision',
    context: {
      escalation: landingResult.escalation,
      branch: landingResult.branch
    }
  });
}

return {
  success: true,
  landed: landingResult,
  // ... other results
};
```

### Pre-conditions for the landing task

The landing task should refuse to run (return error) if:

- `make check` fails before branching (code is not in a landable state)
- No files were modified or created (nothing to land)
- Working tree has unrelated uncommitted changes (dirty state from previous work)

## 4. Checklist for Future Process Creation

Use this checklist when defining a new babysitter process that produces code changes.

### Process structure

- [ ] Process has a planning/design phase
- [ ] Process has a human review breakpoint before implementation begins
- [ ] Process has implementation phases with quality gates
- [ ] Process has a **landing phase** as its final stage (uses `landingTask`)
- [ ] Process return value includes `landed` field with merge status
- [ ] Process handles one-way door escalation with a terminal breakpoint

### Landing phase

- [ ] Landing task receives the complete list of modified/created files
- [ ] Landing task receives a conventional commit message
- [ ] Landing task receives classification hints (model changes, dependency changes, package count)
- [ ] Landing task runs `make check` before and after merge
- [ ] Landing task handles both two-way door (self-merge) and one-way door (push + escalate)

### Quality gate criteria

- [ ] Quality gates verify code correctness (tests pass, build succeeds)
- [ ] Quality gates verify boundary conditions (empty data, edge cases) -- per `tui-quality-gates.md`
- [ ] Quality gates verify external integration assumptions (CWD, env) -- per `tui-quality-gates.md`
- [ ] Quality gates do NOT constitute completion -- they gate the landing phase

### Agent task instructions

- [ ] Implementation agents are told to write code and run tests, NOT to commit
- [ ] Only the landing agent runs git commands
- [ ] No agent modifies files outside the project directory
- [ ] Agents do not stage `.a5c/`, `artifacts/`, or `sessions-bin/` directories

### Process exit criteria

- [ ] `success: true` requires either `landed.merged: true` or a valid escalation
- [ ] If code was written but not landed, the process returns `success: false` with a reason
- [ ] Process does not return success based solely on quality gate scores

## 5. Other Process Gaps (from tui-quality-gates.md)

The TUI post-mortem identified three bug classes that escaped quality gates. These remain relevant:

1. **Boundary rendering tests are not in default quality criteria.** Quality gates check "columns render" but not "columns fit within terminal width." Add width-budget assertions to TUI quality gate templates.

2. **Empty/zero-value test cases are not required.** Test fixtures use populated data. Add an explicit criterion: "test cases include empty/missing values for all displayed fields."

3. **External tool assumptions are not documented in acceptance criteria.** When integrating via `exec` or `os/exec`, the acceptance criteria must include CWD, environment, and filesystem requirements of the external tool.

These are orthogonal to the landing gap but compound with it: code can pass quality gates with latent bugs AND fail to land. Both gaps should be closed.
