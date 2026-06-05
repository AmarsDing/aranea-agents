---
name: executing-plans
description: Use when you have a written implementation plan to execute in a separate session with review checkpoints
---

# Executing Plans

## Iron Law

**Follow the plan. Execute tasks in order. Stop at checkpoints. Never skip ahead or improvise beyond the plan's scope.**

## Core Principle

When a written implementation plan exists (from `writing-plans`, OpenSpec `tasks.md`, or similar), execute it methodically with review checkpoints. The plan is the contract — deviating from it without explicit approval introduces risk and confusion.

## Process

### 1. Load and Validate the Plan

- Read the full plan document
- Confirm all tasks are well-defined with:
  - Clear description
  - Acceptance criteria
  - Affected files/modules
  - Verification steps
- If any task is ambiguous, clarify with the user before starting

### 2. Establish Checkpoints

Define review points based on plan structure:
- After every task that modifies shared interfaces
- After every task that changes database schema
- After every task that affects cross-module boundaries
- At minimum: after every 3rd task or at natural phase boundaries

### 3. Execute Task by Task

For each task in order:

```
1. Read the task description carefully
2. Identify affected files and modules
3. Read existing code in those files (don't assume)
4. Implement the change
5. Run task-specific verification
6. Mark task as complete in the plan
7. If checkpoint reached → pause for review
```

### 4. Checkpoint Protocol

At each checkpoint:
- [ ] Summarize what was accomplished since last checkpoint
- [ ] List any deviations from the plan (and why)
- [ ] Run full verification suite
- [ ] Present status to user for review
- [ ] Wait for approval before continuing

### 5. Handle Deviations

If you discover the plan needs adjustment:
- **Minor deviation** (different variable name, equivalent approach): proceed, document deviation
- **Moderate deviation** (different module affected, extra files changed): pause, explain to user, get approval
- **Major deviation** (plan is fundamentally wrong, requirements changed): stop, propose plan revision

### 6. Final Verification

After all tasks complete:
- Run full project verification (build + test + lint)
- Cross-reference completed work against original acceptance criteria
- Document any remaining gaps or follow-up items

## Red Flags — Stop and Re-evaluate

- **Task seems much harder than the plan anticipated** — the plan may be wrong
- **Task requires changes outside the planned scope** — scope creep, get approval
- **Verification fails and the fix requires plan changes** — revise plan, don't hack
- **You're "improving" code that the plan didn't touch** — stop, stay in scope
- **You've completed 3+ tasks without a checkpoint** — pause and review

## What NOT to Do

- Don't skip tasks because they seem trivial
- Don't reorder tasks without understanding dependencies
- Don't combine tasks unless they're truly inseparable
- Don't add "while I'm here" improvements
- Don't proceed past a checkpoint without review

## Integration with Other Skills

| Skill | When to Use Together |
|-------|---------------------|
| `writing-plans` | The plan being executed was created by this skill |
| `verification-before-completion` | After each task and at each checkpoint |
| `test-driven-development` | Each task should follow TDD within its scope |
| `subagent-driven-development` | When tasks are independent and can be parallelized |
| `requesting-code-review` | At checkpoints or after final verification |
| `systematic-debugging` | When a task's verification fails unexpectedly |

## Session Continuity

If execution spans multiple sessions:
- Always mark completed tasks in the plan document
- Note any deviations or decisions made
- Start the next session by re-reading the plan and last checkpoint status
- Re-run verification to confirm nothing regressed between sessions
