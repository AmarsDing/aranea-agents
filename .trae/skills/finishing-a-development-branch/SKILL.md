---
name: finishing-a-development-branch
description: Use when implementation is complete, all tests pass, and you need to decide how to integrate the work - guides completion of development work by presenting structured options for merge, PR, or cleanup
---

# Finishing a Development Branch

## Iron Law

**Never leave a development branch in an ambiguous state. When work is complete, make a deliberate decision about how to integrate it — and communicate that decision clearly.**

## Core Principle

Completing implementation is not the end. The work must be integrated into the project's main line of development. This skill ensures you make an explicit, informed choice about how to do that, rather than leaving branches dangling or merging without review.

## Prerequisites

Before using this skill, confirm ALL of the following:

- [ ] All implementation tasks are complete
- [ ] All tests pass (verified via `verification-before-completion`)
- [ ] Lint is clean
- [ ] Build succeeds
- [ ] No unintended changes in working tree
- [ ] Commit history is clean and follows conventions

**If any prerequisite fails, stop and fix it first.**

## Process

### 1. Assess the Change

Determine the nature and risk of the change:

| Factor | Low Risk | High Risk |
|--------|----------|-----------|
| Scope | Single module | Cross-module |
| Size | < 200 lines | > 200 lines |
| Impact | Internal only | API / DB / UX changes |
| Confidence | Thoroughly tested | Edge cases uncertain |
| Reversibility | Easy to revert | Difficult to revert |

### 2. Present Integration Options

Based on the risk assessment, present the user with structured options:

#### Option A: Direct Merge (Low Risk Only)
- Fast-forward or merge to target branch
- Suitable for: bug fixes, small refactors, documentation
- Requires: all prerequisites met + user approval

#### Option B: Pull Request (Recommended Default)
- Push branch to remote and create PR
- Suitable for: features, significant changes, anything touching shared interfaces
- Requires: all prerequisites met + PR description with context

#### Option C: Squash Merge (Clean History)
- Squash all commits into one before merging
- Suitable for: feature branches with messy commit history
- Requires: all prerequisites met + user approval

#### Option D: Abort and Cleanup
- Discard the branch if work is no longer needed
- Suitable for: abandoned experiments, superseded changes
- Requires: user confirmation (destructive)

### 3. Execute the Chosen Option

- Follow the user's choice exactly
- Do not add extra steps or "improvements"
- If pushing to remote, confirm the push succeeds

### 4. Post-Integration Cleanup

- [ ] Delete the feature branch (local and remote) if merged
- [ ] Update any related task tracking (development.md, tasks.md)
- [ ] Verify the main branch builds cleanly after integration
- [ ] Notify the user of completion with a summary

## Red Flags — Stop and Ask

- **User hasn't confirmed the integration method** — don't assume
- **Prerequisites aren't met** — go back to verification
- **Merge conflicts exist** — resolve before proceeding, don't force-push
- **CI is failing on the branch** — fix before creating PR
- **You're about to push to main/master directly** — warn the user

## What NOT to Do

- Don't merge without user approval
- Don't force-push to shared branches
- Don't leave branches unmerged without documenting why
- Don't skip cleanup (branch deletion, task updates)
- Don't assume the user wants the "fastest" option

## Integration with Other Skills

| Skill | When to Use Together |
|-------|---------------------|
| `verification-before-completion` | Mandatory prerequisite — verify before finishing |
| `requesting-code-review` | Option B (PR) should include code review |
| `using-git-worktrees` | If work was done in a worktree, clean it up |
| `subagent-driven-development` | After all subagents complete and merge |

## Decision Flowchart

```
All prerequisites met?
  NO → Fix failing prerequisites first
  YES → Assess risk level

Risk level?
  LOW  → Present options A/B/C/D
  HIGH → Recommend option B (PR) with justification

User chose?
  Execute choice → Post-integration cleanup → Done
```
