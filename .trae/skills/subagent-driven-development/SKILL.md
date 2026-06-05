---
name: subagent-driven-development
description: Use when executing implementation plans with independent tasks in the current session
---

# Subagent-Driven Development

## Iron Law

**Decompose work into independent, parallelizable tasks. Delegate each task to a focused subagent. Coordinate results — never serialize what can be parallelized.**

## Core Principle

When an implementation plan contains multiple independent tasks, execute them via subagents rather than sequentially. Each subagent owns one task end-to-end: read context, implement, verify. The coordinator (you) handles dependencies, merges results, and enforces quality gates.

## Process

### 1. Analyze the Plan

- Read the full implementation plan (e.g., `tasks.md` from OpenSpec)
- Identify task dependencies (which tasks must complete before others can start)
- Group tasks into dependency tiers:
  - **Tier 0**: No dependencies — can start immediately
  - **Tier N**: Depends on Tier N-1 outputs

### 2. Prepare Subagent Briefings

For each task, prepare a self-contained briefing that includes:
- Task description and acceptance criteria
- Relevant file paths and module boundaries
- Constraints (red lines, coding standards to follow)
- Verification command(s) to run after implementation
- What NOT to change (adjacent modules, shared interfaces)

### 3. Execute by Tier

```
For each tier (0, 1, 2, ...):
  1. Launch all tier tasks as parallel subagents
  2. Wait for all to complete
  3. Run integration verification across tier outputs
  4. Resolve conflicts before advancing to next tier
```

### 4. Merge and Verify

- After all tiers complete, run full project verification:
  - `make build && make test && make lint` (backend)
  - `pnpm lint && pnpm test && pnpm build` (frontend)
- Check for integration issues between subagent outputs
- Resolve any conflicts or inconsistencies

### 5. Report Results

- Summarize what each subagent accomplished
- List any tasks that failed verification and why
- Flag remaining work or follow-up items

## Red Flags — Stop and Re-evaluate

- **Subagent modifies shared interfaces** without coordination — pause and align
- **Circular dependencies** between tasks — restructure the plan before executing
- **Subagent exceeds scope** — it's changing files outside its briefing — kill and re-brief
- **Verification fails after merge** — don't patch blindly; trace which subagent's output caused the failure
- **More than 3 tiers** — plan may be poorly decomposed; consider restructuring

## What NOT to Do

- Don't launch subagents for trivially small tasks (overhead > benefit)
- Don't let subagents share mutable state (files, branches) without coordination
- Don't skip the integration verification between tiers
- Don't assume subagent output is correct — always verify
- Don't serialize tasks that have no dependency relationship

## Integration with Other Skills

| Skill | When to Use Together |
|-------|---------------------|
| `verification-before-completion` | After each tier and after final merge |
| `executing-plans` | When the plan needs review checkpoints between tasks |
| `writing-plans` | Before subagent execution — ensure plan is well-decomposed |
| `test-driven-development` | Each subagent should follow TDD within its task |
| `requesting-code-review` | After all subagents complete, before final integration |

## Decision Guide

```
Is the plan well-decomposed with clear task boundaries?
  YES → Use subagent-driven-development
  NO  → Use executing-plans (sequential with checkpoints)

Are tasks truly independent?
  YES → Parallelize across tiers
  NO  → Serialize along dependency chains, parallelize within tiers
```
