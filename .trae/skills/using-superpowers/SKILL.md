---
name: using-superpowers
description: Use when starting any conversation - establishes how to find and use skills, requiring Skill tool invocation before ANY response including clarifying questions
---

# Using Superpowers

## Iron Law

**Always check for relevant skills before responding. The Skill tool is your first action, not an afterthought. If a skill exists for the task, use it — don't reinvent the wheel.**

## Core Principle

Skills encode project-specific knowledge, standards, and processes. They are the project's accumulated wisdom. Before responding to any request, check whether a skill exists that applies. If one does, invoke it immediately — before writing any code, before asking clarifying questions, before doing anything else.

## Process

### 1. On Every New Request

Before responding to any user message:

1. **Scan the request** for keywords that match skill descriptions
2. **Check the skill list** for relevant skills
3. **Invoke the most relevant skill** via the Skill tool
4. **Then respond** according to the skill's guidance

### 2. Skill Selection Guide

| User Intent | Skill to Invoke |
|-------------|----------------|
| Writing Go backend code | `aranea-coding-guide` |
| Writing Vue/TS frontend code | `aranea-frontend-guide` |
| Designing Go structs/interfaces | `go-oop-guide` |
| Designing Vue components | `vue-frontend-guide` |
| Reviewing code | `aranea-review` or `go-oop-review` |
| Exploring requirements | `openspec-explore` |
| Creating a proposal | `openspec-propose` |
| Implementing a change | `openspec-apply-change` |
| Running tests/fixing failures | `aranea-test-loop` |
| Debugging a bug | `systematic-debugging` |
| Writing a plan | `writing-plans` |
| Executing a plan | `executing-plans` |
| Doing TDD | `test-driven-development` |
| Requesting review | `requesting-code-review` |
| Receiving review feedback | `receiving-code-review` |
| Finishing a branch | `finishing-a-development-branch` |
| Using git worktrees | `using-git-worktrees` |
| Parallel task execution | `subagent-driven-development` |
| Verifying before completion | `verification-before-completion` |
| Creative work / brainstorming | `brainstorming` |
| Security review | `TRAE-security-review` |

### 3. Multiple Skills May Apply

When multiple skills are relevant:
- Invoke the most specific skill first
- Skills can be invoked sequentially as the task evolves
- Example: Start with `aranea-coding-guide` for Go code, then `verification-before-completion` when done

### 4. When No Skill Applies

If no skill matches the task:
- Proceed with general best practices
- Consider whether a new skill should be created (suggest to user)
- Don't force a skill that doesn't fit

## Skill Invocation Rules

1. **Invoke BEFORE any response** — including clarifying questions
2. **Invoke IMMEDIATELY** — don't announce "I'll use skill X", just invoke it
3. **One skill per invocation** — invoke sequentially if multiple are needed
4. **Don't re-invoke** a skill that's already active in the conversation
5. **Follow the skill's guidance** — skills contain project-specific rules that override general practices

## Red Flags — You're Doing It Wrong

- **You're writing code without checking for skills** — stop and check
- **You're about to ask a clarifying question without checking skills** — the skill may already answer it
- **You're following general best practices that contradict a project skill** — project skill wins
- **You invoked a skill but aren't following its guidance** — re-read the skill output
- **You're reinventing a process that a skill already defines** — use the skill

## What NOT to Do

- Don't skip skill invocation because you "already know" the skill
- Don't invoke skills you don't need (overhead without benefit)
- Don't mention skill names to the user as if they should care — just use them
- Don't treat skills as optional — they encode project standards
- Don't override skill guidance with your own preferences

## Integration with Other Skills

This skill is the meta-skill that governs all other skill usage. It doesn't compete with other skills — it ensures they're used correctly.

| Scenario | What This Skill Tells You |
|----------|--------------------------|
| "Fix this bug" | Invoke `systematic-debugging` first |
| "Implement this feature" | Invoke `aranea-coding-guide` or `aranea-frontend-guide` first |
| "Review this code" | Invoke `aranea-review` first |
| "I'm done" | Invoke `verification-before-completion` first |
| "Let's plan this" | Invoke `writing-plans` or `brainstorming` first |

## The Superpowers Mantra

> "Skills first. Always. Before code, before questions, before assumptions. The skill knows what the project needs — let it guide you."
