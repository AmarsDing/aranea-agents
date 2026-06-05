---
name: receiving-code-review
description: Use when receiving code review feedback, before implementing suggestions, especially if feedback seems unclear or technically questionable - requires technical rigor and verification, not performative agreement or blind implementation
---

# Receiving Code Review

## Iron Law

**Review feedback deserves thoughtful evaluation, not performative agreement. Understand before implementing. Verify before accepting. Push back with evidence when feedback is technically wrong.**

## Core Principle

Code review is a conversation, not a command hierarchy. When you receive feedback, your job is to evaluate it with the same rigor you'd apply to any technical claim. Blindly implementing every suggestion is as harmful as ignoring all feedback.

## Process

### 1. Read All Feedback First

- Read every comment before responding to any
- Understand the overall themes and concerns
- Categorize feedback into types:

| Category | Examples | Response |
|----------|----------|----------|
| **Bug found** | Logic error, edge case missed | Fix immediately — this is a real problem |
| **Design concern** | Architecture, abstraction level | Evaluate, discuss, decide together |
| **Style preference** | Naming, formatting, patterns | Follow project conventions, not personal taste |
| **Suggestion** | "Consider using X instead" | Evaluate on merit, accept or decline with reason |
| **Question** | "Why did you do it this way?" | Answer clearly, may reveal a documentation gap |
| **Incorrect feedback** | Factually wrong about behavior | Correct with evidence, don't just comply |

### 2. Evaluate Each Piece of Feedback

For each comment, ask:

1. **Is it factually correct?** Does the reviewer understand the code as written?
2. **Is it relevant?** Does it address the change's purpose and scope?
3. **Is it consistent with project standards?** Check against project SKILLs and coding guidelines.
4. **What happens if I don't implement it?** Assess the actual risk.

### 3. Respond with Technical Rigor

**When accepting feedback:**
- Acknowledge the specific value: "Good catch — this edge case would cause X"
- Implement the change
- Verify the fix works

**When questioning feedback:**
- State your understanding of the concern
- Explain why you disagree, with technical evidence
- Propose an alternative if applicable
- Be open to being wrong

**When feedback is technically incorrect:**
- Don't comply just to be agreeable
- Provide evidence: code references, test results, documentation
- Example: "The reviewer suggests this function is thread-unsafe, but it's only called from the single-threaded event loop (see `internal/event/loop.go:42`). Adding a mutex would add overhead without benefit."

### 4. Handle Ambiguous Feedback

If feedback is unclear:
- Ask for clarification before implementing
- Don't guess at what the reviewer meant
- Example: "Could you elaborate on what you mean by 'this should be more robust'? What specific failure scenario are you concerned about?"

### 5. Verify All Changes

After implementing accepted feedback:
- Run full verification (`verification-before-completion`)
- Ensure no regressions from the review-driven changes
- Confirm the original issue is still fixed

## Red Flags — Stop and Think

- **You're implementing every suggestion without evaluation** — you're not thinking
- **You're rejecting every suggestion defensively** — you're not listening
- **Feedback contradicts project standards** — defer to project SKILLs
- **Feedback would introduce a bug** — push back with evidence
- **You feel pressured to agree** — take a step back and evaluate technically

## What NOT to Do

- Don't implement feedback you don't understand
- Don't agree just to avoid conflict
- Don't reject feedback without technical justification
- Don't treat all feedback as equally important
- Don't implement style changes that contradict project conventions
- Don't silently ignore feedback — respond to every comment

## Integration with Other Skills

| Skill | When to Use Together |
|-------|---------------------|
| `verification-before-completion` | After implementing review-driven changes |
| `requesting-code-review` | The skill that initiated the review |
| `systematic-debugging` | If review identifies a bug you need to investigate |
| `aranea-review` / `go-oop-review` | Project-specific review standards to validate feedback against |

## The Review Response Mantra

> "I will understand before I implement. I will verify before I accept. I will push back with evidence when feedback is wrong. I will not perform agreement."
