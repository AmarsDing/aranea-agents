---
name: requesting-code-review
description: Use when completing tasks, implementing major features, or before merging to verify work meets requirements
---

# Requesting Code Review

## Iron Law

**Request review with context, not just code. A reviewer can only evaluate what they can understand — provide the "why" alongside the "what".**

## Core Principle

Code review is a quality gate, not a formality. When requesting review, you must provide enough context for the reviewer to make informed judgments about correctness, design, and adherence to project standards.

## When to Request Review

- After completing a feature or significant change
- Before merging to a protected branch
- After fixing a complex bug (to verify the fix is sound)
- When a change affects shared interfaces or cross-module boundaries
- When a change touches security-sensitive code

## Process

### 1. Pre-Review Self-Check

Before requesting, verify:
- [ ] All tests pass (verified via `verification-before-completion`)
- [ ] Lint is clean
- [ ] Build succeeds
- [ ] No unintended changes in the diff
- [ ] Commit messages are clear and follow conventions

### 2. Prepare the Review Request

Provide the following context:

**What changed and why:**
- Summary of the change in 1-3 sentences
- Link to the task/issue/spec that drove the change
- Business justification if not obvious

**How it was tested:**
- Which tests were added or modified
- Manual verification steps performed
- Edge cases considered

**Areas of concern:**
- Parts of the code you're uncertain about
- Trade-offs made and alternatives considered
- Known limitations or follow-up work needed

**Review focus areas:**
- Specific aspects you want the reviewer to focus on
  - e.g., "Please review the error handling in the data layer"
  - e.g., "Focus on the concurrency model in the session manager"

### 3. Scope the Review Appropriately

| Change Size | Review Approach |
|-------------|----------------|
| < 50 lines | Quick review, focus on correctness |
| 50-200 lines | Standard review, check design + correctness |
| 200-500 lines | Detailed review, consider splitting into logical chunks |
| > 500 lines | Request review per commit or per logical unit |

### 4. Use Project-Specific Review Standards

For this project, code review must use project SKILLs:
- **Full-stack review**: `aranea-review` (architecture + data flow + OOP + UX)
- **Go OOP review**: `go-oop-review` (struct design + interfaces + composition)
- **Frontend review**: `aranea-frontend-review` (data flow + component layers + UX theme)
- Generic review (`TRAE-code-review`) can supplement but must not replace project SKILLs

### 5. Handle Review Feedback

- See `receiving-code-review` skill for how to process feedback
- Respond to every comment, even if just acknowledging
- Don't dismiss feedback without technical justification

## Red Flags — Don't Request Review If

- Tests are failing — fix first
- Lint has new errors — fix first
- You haven't verified the change works — verify first
- The diff includes unrelated changes — split the PR
- You can't explain why the change was made — clarify first

## What NOT to Do

- Don't request review on broken code
- Don't bundle unrelated changes into one review
- Don't ignore review comments or argue defensively
- Don't treat review as a rubber stamp
- Don't skip project-specific review SKILLs in favor of generic review only

## Integration with Other Skills

| Skill | When to Use Together |
|-------|---------------------|
| `verification-before-completion` | Mandatory before requesting review |
| `receiving-code-review` | After review feedback arrives |
| `finishing-a-development-branch` | Review is typically part of the PR process |
| `aranea-review` / `go-oop-review` | Project-specific review standards |
| `TRAE-code-review` | Supplemental generic review |
