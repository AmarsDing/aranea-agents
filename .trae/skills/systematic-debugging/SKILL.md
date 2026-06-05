---
name: systematic-debugging
description: Use when encountering any bug, test failure, or unexpected behavior, before proposing fixes
---

# Systematic Debugging

## Iron Law

**Never fix what you don't understand. Reproduce the problem, form a hypothesis, test the hypothesis, then fix the root cause — never the symptom.**

## Core Principle

Debugging is a scientific process, not a guessing game. When you encounter a bug, test failure, or unexpected behavior, resist the urge to immediately propose a fix. Instead, follow a systematic approach: observe, hypothesize, test, and only then fix.

## Process

### 1. Observe and Reproduce

- [ ] Describe the exact symptom: what happened vs. what was expected
- [ ] Identify the minimal reproduction steps
- [ ] Determine if the issue is deterministic or intermittent
- [ ] Check if the issue is new (did it work before?) — if so, what changed?

**Key questions:**
- What is the exact error message or unexpected behavior?
- Can you reproduce it consistently?
- Does it happen in all environments or only specific ones?
- When was the last time it worked (if ever)?

### 2. Gather Evidence

- [ ] Read the relevant code paths carefully
- [ ] Check logs for error messages, stack traces, or anomalies
- [ ] Examine the state at the point of failure (variables, DB records, etc.)
- [ ] Look for recent changes that could be related (`git log`, `git diff`)

**Evidence sources:**
- Error messages and stack traces
- Log output (use `pkg/loggateway.Logger`, never `log/slog`)
- Test output with verbose flags
- Database state before/after the operation
- Network requests/responses (if applicable)

### 3. Form a Hypothesis

Based on the evidence, form a specific, testable hypothesis:

**Bad hypothesis**: "Something is wrong with the data layer"
**Good hypothesis**: "The `CreateSession` method doesn't validate the `agentID` field, causing a nil pointer dereference when `agentID` is empty"

A good hypothesis:
- Is specific about the root cause
- Explains all observed symptoms
- Is testable with a targeted experiment

### 4. Test the Hypothesis

Design the simplest experiment to confirm or deny:

- Add a targeted log statement at the suspected point
- Write a failing test that demonstrates the bug
- Use a debugger or print statements to inspect state
- Check the hypothesis against the code logic

**If the hypothesis is confirmed** → proceed to fix
**If the hypothesis is denied** → go back to step 2 with new evidence

### 5. Fix the Root Cause

- Fix the actual root cause, not just the symptom
- The fix should be the minimal change that addresses the root cause
- Add a test that would have caught this bug (regression test)
- Verify the fix resolves the original symptom AND the test passes

### 6. Verify the Fix

- [ ] The original reproduction steps no longer produce the bug
- [ ] The regression test passes
- [ ] No new failures introduced (run full test suite)
- [ ] Follow `verification-before-completion` for full validation

## Common Debugging Anti-Patterns

| Anti-Pattern | Why It's Bad | What to Do Instead |
|-------------|-------------|-------------------|
| Shotgun debugging | Changing random things hoping something works | Form and test hypotheses |
| Fixing the symptom | Suppressing the error without addressing cause | Trace to root cause |
| "It works on my machine" | Ignoring environment differences | Reproduce in the target environment |
| Assuming causation | "X changed, Y broke, therefore X caused Y" | Test the causal link |
| Over-debugging | Adding too many logs/prints | Target your investigation |

## Red Flags — Stop and Re-evaluate

- **You've tried 3+ fixes and none worked** — your hypothesis is wrong, go back to evidence
- **The bug "disappeared" after a change** — it may be masked, not fixed; verify with the original reproduction
- **You're fixing something unrelated to the reported symptom** — you may be scope-creeping
- **The fix is complex** — simple bugs usually have simple root causes; reconsider your hypothesis
- **You can't reproduce the bug** — you don't understand it well enough to fix it

## What NOT to Do

- Don't propose a fix before understanding the root cause
- Don't add `try/catch` or error suppression as a "fix"
- Don't change unrelated code while debugging
- Don't assume the bug is in someone else's code without evidence
- Don't skip writing a regression test

## Integration with Other Skills

| Skill | When to Use Together |
|-------|---------------------|
| `verification-before-completion` | After fixing, verify thoroughly |
| `test-driven-development` | Write a failing test first, then fix |
| `TRAE-debugger` | For complex runtime issues requiring instrumentation |
| `requesting-code-review` | After fixing a complex bug, get review on the fix |
| `receiving-code-review` | If reviewer identifies issues with your fix |

## The Debugging Mantra

> "If you can't reproduce it, you don't understand it. If you don't understand it, you can't fix it. If you fix the symptom, the bug will return."
