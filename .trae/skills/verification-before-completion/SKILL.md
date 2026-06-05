---
name: verification-before-completion
description: Use when about to claim work is complete, fixed, or passing, before committing or creating PRs - requires running verification commands and confirming output before making any success claims; evidence before assertions always
---

# Verification Before Completion

## Iron Law

**Never claim work is complete, fixed, or passing without running verification commands and confirming the output yourself. Evidence before assertions — always.**

## Core Principle

Every success claim must be backed by observable evidence. "I fixed the bug" is meaningless without a passing test. "The build works" is meaningless without a clean build output. Before you say "done", you must have proof.

## Process

### 1. Before Claiming "Fixed"

- [ ] Run the specific test that was failing — confirm it passes
- [ ] Run the broader test suite for the affected module — confirm no regressions
- [ ] Check that the fix doesn't break any other tests
- [ ] Verify the root cause is addressed, not just the symptom

### 2. Before Claiming "Feature Complete"

- [ ] Run all project verification commands:
  - Backend: `make build && make test && make lint`
  - Frontend: `pnpm lint && pnpm test && pnpm build`
- [ ] Confirm zero errors in output — warnings are acceptable only if pre-existing
- [ ] Manually verify the acceptance criteria from the task spec
- [ ] Check for type errors: `pnpm typecheck` (frontend) or `go vet ./...` (backend)

### 3. Before Claiming "Tests Pass"

- [ ] Run the test command yourself — do not assume
- [ ] Read the output — confirm "PASS" or equivalent, not just "no error"
- [ ] Check for race conditions: `go test -race ./...` (Go)
- [ ] Verify test coverage hasn't dropped significantly

### 4. Before Committing

- [ ] All verification commands pass cleanly
- [ ] No unintended changes in `git diff` (check for stray edits, debug logs)
- [ ] No secrets, credentials, or `.env` files in staged changes
- [ ] Lint passes with zero new warnings

### 5. Before Creating a PR

- [ ] Full verification suite passes
- [ ] Branch is up to date with target branch
- [ ] Commit messages follow project conventions
- [ ] No TODO/FIXME comments left as placeholders for required logic

## Evidence Requirements

| Claim | Required Evidence |
|-------|-------------------|
| "Bug fixed" | Failing test now passes + no regressions |
| "Feature works" | Acceptance criteria verified + all tests pass |
| "Build succeeds" | Clean `make build` output with exit code 0 |
| "Lint clean" | `make lint` / `pnpm lint` exits 0 |
| "Tests pass" | Test runner output showing PASS with exit code 0 |
| "No type errors" | `go vet` / `pnpm typecheck` exits 0 |

## Red Flags — Stop and Verify

- **You're about to say "done" but haven't run commands** — stop, run them
- **You ran commands in a previous session** — re-run now; code may have changed
- **A test was "flaky"** — run it 3+ times to confirm stability
- **You modified generated code** (proto, wire) — re-generate and rebuild
- **You changed shared interfaces** — run the full test suite, not just the module

## What NOT to Do

- Don't claim "this should work" without running it
- Don't trust your reasoning over runtime evidence
- Don't skip verification because "it's a small change"
- Don't copy-paste previous command output as proof — run it now
- Don't suppress errors to make verification "pass" (e.g., `|| true`)

## Integration with Other Skills

| Skill | When to Use Together |
|-------|---------------------|
| `test-driven-development` | TDD ensures tests exist before claiming "implemented" |
| `finishing-a-development-branch` | Verification is a prerequisite before finishing |
| `requesting-code-review` | Only request review after verification passes |
| `systematic-debugging` | When verification fails, use systematic debugging before claiming "fixed" |
| `subagent-driven-development` | Each subagent must verify before reporting completion |

## The Verification Mantra

> "If you didn't run it, it doesn't work. If you didn't read the output, it didn't pass."
