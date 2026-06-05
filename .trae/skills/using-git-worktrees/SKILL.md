---
name: using-git-worktrees
description: Use when starting feature work that needs isolation from current workspace or before executing implementation plans - ensures an isolated workspace exists via native tools or git worktree fallback
---

# Using Git Worktrees

## Iron Law

**Isolate feature work from the main workspace. Never develop on main. Use worktrees to create clean, independent working directories for each feature or change.**

## Core Principle

Git worktrees provide isolated working directories that share the same repository. This allows you to work on multiple features simultaneously without stashing, committing half-done work, or creating conflicts in your main workspace.

## When to Use Worktrees

- Starting a new feature branch
- Working on a bug fix while in the middle of another feature
- Executing an implementation plan that requires a clean workspace
- Parallel development by multiple subagents
- Any work that should not disturb the current workspace state

## Process

### 1. Check Current State

Before creating a worktree:
- [ ] Verify you're in a git repository: `git rev-parse --git-dir`
- [ ] Check current branch and status: `git status`
- [ ] Ensure no uncommitted changes that would conflict

### 2. Create the Worktree

```bash
# Create a new branch and worktree in one step
git worktree add ../<feature-name> -b <feature-name>

# Or create a worktree for an existing branch
git worktree add ../<feature-name> <existing-branch>
```

**Naming convention**: Use the feature/task name as both branch and directory name.

**Location**: Worktrees are created as sibling directories by default (e.g., `F:\aranea-agents-feature-x\`). This keeps them outside the main workspace but easily accessible.

### 3. Work in the Worktree

- Navigate to the worktree directory
- All git operations work normally within the worktree
- The worktree has its own working tree but shares the `.git` directory
- Changes in the worktree don't affect the main workspace

### 4. Merge or Create PR

When work is complete:
- Follow `finishing-a-development-branch` for integration decisions
- Push the branch from the worktree
- Create PR or merge as appropriate

### 5. Clean Up

After integration:
```bash
# Remove the worktree
git worktree remove ../<feature-name>

# Delete the branch if merged
git branch -d <feature-name>

# Prune stale worktree references
git worktree prune
```

## Worktree Management Commands

| Command | Purpose |
|---------|---------|
| `git worktree list` | List all worktrees |
| `git worktree add <path> -b <branch>` | Create new worktree + branch |
| `git worktree add <path> <branch>` | Create worktree for existing branch |
| `git worktree remove <path>` | Remove a worktree |
| `git worktree prune` | Clean up stale references |

## Red Flags — Stop and Re-evaluate

- **Worktree creation fails** — check for branch name conflicts or path issues
- **"already checked out" error** — the branch is checked out in another worktree; use a different branch name
- **Uncommitted changes in main workspace** — stash or commit before creating worktree
- **Worktree directory already exists** — remove it or choose a different path
- **Disk space concerns** — worktrees share objects but have separate working trees

## What NOT to Do

- Don't create worktrees inside the main workspace directory
- Don't leave worktrees after merging — clean them up
- Don't create more worktrees than you can manage (practical limit: 3-5)
- Don't work on the same branch in multiple worktrees simultaneously
- Don't forget to prune after removing worktrees manually

## Integration with Other Skills

| Skill | When to Use Together |
|-------|---------------------|
| `subagent-driven-development` | Each subagent may work in its own worktree |
| `executing-plans` | Create a worktree before executing to isolate changes |
| `finishing-a-development-branch` | Clean up worktree after integration |
| `verification-before-completion` | Verify in the worktree before merging |

## Fallback: When Worktrees Aren't Available

If git worktrees aren't supported or practical:
1. Create a new branch in the current workspace: `git checkout -b <feature-name>`
2. Stash any in-progress work: `git stash push -m "WIP: <description>"`
3. Do the feature work
4. Commit and push
5. Restore previous work: `git checkout <previous-branch> && git stash pop`

This is less ideal (no true isolation) but works as a fallback.
