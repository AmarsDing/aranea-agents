---
name: superpowers-workflow
description: Enforce development discipline during implementation — TDD (RED-GREEN-REFACTOR), two-phase code review (spec compliance + code quality), verification before completion, and bite-sized task execution. Use when implementing tasks from an OpenSpec change or any development task that requires quality assurance.
license: MIT
compatibility: Works with openspec-apply-change and aranea-coding-guide/aranea-frontend-guide skills.
metadata:
  author: aranea-agents
  version: "1.0"
  adaptedFrom: "obra/superpowers"
---

Enforce development discipline during implementation. This skill activates when you are implementing code changes — whether from an OpenSpec change or ad-hoc development tasks.

---

## Core Principles

1. **TDD Mandatory**: Never write implementation before a failing test (RED → GREEN → REFACTOR)
2. **Two-Phase Review**: Spec compliance first, then code quality
3. **Verification Before Completion**: Provide evidence before declaring done
4. **Bite-Sized Tasks**: Each task should be completable in 2-5 minutes
5. **Context Isolation**: Each task gets only the context it needs

---

## Phase 1: Pre-Implementation

Before writing any code:

### 1.1 Read Context Files

If working within an OpenSpec change, read ALL context files:
```
openspec/changes/<name>/proposal.md
openspec/changes/<name>/design.md
openspec/changes/<name>/specs/
openspec/changes/<name>/tasks.md
```

If working ad-hoc, clarify:
- What is the goal?
- What are the non-goals?
- What layers are affected?

### 1.2 Read Cross-Reference

**MANDATORY**: Before touching any module, read the module cross-reference:
```
openspec/specs/module-cross-reference.md
```

Find the target module card and check:
- Upstream dependencies (what interfaces do you call?)
- Downstream impact (who depends on you?)
- Shared contracts (types/events being modified?)
- Database changes (schema migration needed?)

### 1.3 Read Project Rules

**MANDATORY**: Read the relevant skill based on what you're implementing:
- Backend Go code → `aranea-coding-guide` skill
- Frontend Vue code → `aranea-frontend-guide` skill
- Go OOP design → `go-oop-guide` skill

---

## Phase 2: TDD Implementation (RED-GREEN-REFACTOR)

### 2.1 RED — Write a Failing Test

For each task:

1. **Write ONE failing test** that covers the specific behavior
2. Test must be focused: one assertion per test case
3. Test must be isolated: no DB/network dependencies (use interfaces/mocks)
4. Run the test and confirm it FAILS:
   ```bash
   go test ./internal/<layer>/... -run TestXxx -count=1
   ```
5. Show the failing test output

**Test naming convention**:
```go
func TestTaxonomyUsecase_Create(t *testing.T)           // happy path
func TestTaxonomyUsecase_Create_DuplicateKey(t *testing.T) // error path
func TestTaxonomyUsecase_Create_InvalidInput(t *testing.T) // boundary
```

**What to test first**:
- Pure validation / mapping logic (no I/O)
- State machine transitions
- DTO mapping functions
- Error classification

### 2.2 GREEN — Write Minimal Implementation

1. Write the MINIMUM code to make the test pass
2. Do NOT add features not covered by the test
3. Do NOT refactor yet
4. Run the test and confirm it PASSES
5. Show the passing test output

### 2.3 REFACTOR — Clean Up

1. Improve code structure without changing behavior
2. Add error handling
3. Improve naming
4. Run ALL tests to confirm nothing broke:
   ```bash
   go test ./internal/<layer>/... -count=1
   ```

### 2.4 TDD Exceptions

TDD may be skipped ONLY for:
- Hotfixes (mark with `// HOTFIX: skip TDD` comment)
- Typo fixes
- CSS/style-only changes
- Auto-generated code (protoc/wire/ent)

When skipping, document WHY in the commit message.

---

## Phase 3: Two-Phase Code Review

After implementation, perform two-phase review:

### Phase A: Spec Compliance Review

Check against the OpenSpec artifacts (or stated requirements):

| Check | Question |
|-------|----------|
| Goal coverage | Does implementation satisfy all goals in proposal.md? |
| Non-goal respect | Were any non-goals accidentally implemented? |
| Design adherence | Does code follow the design.md decisions? |
| Spec coverage | Do all specs in specs/ have corresponding implementation? |
| DoD met | Does each task meet its Definition of Done? |

**Output format**:
```
## Spec Compliance Review

✅ Goal 1: [description] — Implemented in [file:line]
✅ Goal 2: [description] — Implemented in [file:line]
❌ Goal 3: [description] — NOT IMPLEMENTED
⚠️ Non-goal violation: [what was done beyond scope]

Result: PASS / FAIL / PASS_WITH_CONCERNS
```

### Phase B: Code Quality Review

Use the project's review skills:
- Backend → `go-oop-review` skill
- Frontend → `aranea-review` skill (frontend sections)

**Dimension-based review**: Load review dimensions based on change scope (see `docs/review-dimension-checklists.md` B-side checklists):

| Change scope | Required dimensions |
|-------------|-------------------|
| All changes | 1 (Architecture), 2 (Quality), 3 (Correctness), 8 (Error handling) |
| Involves DB | + 4 (Performance) |
| Involves external input/API | + 5 (Security) |
| Involves Usecase | + 6 (Testability), 11 (Business logic) |
| Involves cross-module | + 7 (Maintainability), 12 (Doc sync) |
| Involves frontend | + 9 (FE Performance), 10 (FE Security) |

| Check | Question |
|-------|----------|
| Layer compliance | Does code follow the dependency direction? |
| Red line check | Are any of the 11 backend / 12 frontend red lines violated? |
| Programming standard check | Are any of the CS-B1~B17 / CS-F1~F8 coding standards violated? |
| OOP compliance | Are structs/interfaces properly designed? |
| Error handling | Are errors using kerrors (not fmt.Errorf)? |
| Concurrency | Are goroutines using safego? |
| Logging | Is logging using loggateway.Logger? |
| Dimension B-side | Are all loaded dimension B-side checklist items passing? |

**Output format**:
```
## Code Quality Review

✅ Layer compliance: All dependencies point inward
❌ Red line #1: biz imports pkg/trpc-agent-go in [file:line]
⚠️ OOP: Interface too wide in [file:line]

Result: APPROVED / NEEDS_CHANGES
```

### Review Priority

If both reviews have issues:
1. Fix spec compliance issues FIRST (wrong thing built)
2. Fix code quality issues SECOND (right thing built wrong)

---

## Phase 4: Verification Before Completion

Before declaring a task complete, provide EVIDENCE:

### 4.1 Evidence Checklist

| Evidence Type | Command | Required |
|---------------|---------|----------|
| Tests pass | `go test ./internal/<layer>/... -count=1` | ✅ Always |
| Lint passes | `make lint` or `pnpm lint` | ✅ Always |
| Build passes | `make build` or `pnpm build` | ✅ Always |
| Type check | `go build ./...` or `pnpm build` | ✅ Always |
| No red line violations | Manual check against project_rules.md | ✅ Always |
| No coding standard violations | Check CS-B1~B17 / CS-F1~F8 | ✅ Always |
| Dimension review passed | B-side checklists in docs/review-dimension-checklists.md | ✅ Always |
| Cross-module impact | Read module-cross-reference.md | ✅ For cross-layer changes |
| Wire generation | `make wire` | ⚠️ If Wire deps changed |
| Proto generation | `make api` | ⚠️ If proto changed |
| Ent generation | `go generate ./internal/data/ent` | ⚠️ If schema changed |

### 4.2 Verification Levels

| Change scope | Minimum verification |
|-------------|---------------------|
| Single layer (biz/data/service) | `go test ./internal/<layer>/... -count=1` |
| Cross-layer (biz+data+service) | `make test` |
| Proto/Wire/Ent changes | `make api && make wire && make build && make test` |
| Frontend only | `cd web && pnpm lint && pnpm test && pnpm build` |
| Full stack | Full validation chain |

### 4.3 Declaration Format

```
## Task Completion Declaration

**Task**: [task description]
**Evidence**:
- [x] Tests pass (output: [summary])
- [x] Lint passes
- [x] Build passes
- [x] No red line violations
- [x] Cross-module impact checked

**Status**: COMPLETE ✅ / INCOMPLETE ❌
```

**NEVER declare a task complete without providing evidence.**

---

## Phase 5: Failure Recovery

When things go wrong, follow these recovery patterns:

### 5.1 TDD Stuck (can't write a reasonable test)

**Symptoms**: Can't write a failing test, or test is too coupled to I/O

**Recovery**:
1. Task is too big — break it down further
2. Dependencies not isolated — define interface + mock
3. Start with pure validation tests (no I/O), then add integration tests

### 5.2 Implementation Drifts from Spec

**Symptoms**: Code "looks right" but doesn't match specs

**Recovery**:
1. Go back to tasks.md — check DoD for each task
2. Remove any non-goal implementations
3. Update design.md if reality differs from plan (with justification)

### 5.3 Verify Fails

**Symptoms**: Tests/lint/build fail after implementation

**Recovery**:
1. Map failures back to tasks.md — which task's DoD is unmet?
2. Fix "cheap failures" first (lint/type errors), then systemic ones
3. Add regression test for each fix
4. Don't batch — fix one, verify, then next

### 5.4 Review Conflicts

**Symptoms**: Spec reviewer and code reviewer disagree

**Recovery**:
1. Spec compliance takes priority — fix spec issues first
2. Code quality issues — limit refactoring scope (no behavior changes)
3. If major refactoring needed — create a new OpenSpec change for it

---

## Integration with OpenSpec

This skill integrates with the OpenSpec workflow:

| OpenSpec Phase | Superpowers Activation |
|---------------|----------------------|
| `explore` | Not active (thinking mode) |
| `propose` | Not active (specification mode) |
| `apply` | **FULLY ACTIVE** — TDD + Review + Verify |
| `archive` | Review summary + evidence check |

When using `openspec-apply-change`, this skill should be automatically engaged for the implementation phase.

---

## Integration with Aranea-Agents Skills

| Skill | When to Use Together |
|-------|---------------------|
| `aranea-coding-guide` | Phase 2 (implementation) — follow backend coding rules |
| `aranea-frontend-guide` | Phase 2 (implementation) — follow frontend coding rules |
| `go-oop-guide` | Phase 2 (implementation) — follow OOP design patterns |
| `go-oop-review` | Phase 3B (code quality review) — backend review checklist |
| `aranea-review` | Phase 3 (full review) — full-stack review checklist |
| `aranea-test-loop` | Phase 4 (verification) — test loop process |
| `openspec-apply-change` | Phase 2 (implementation) — task tracking |
| Dimension checklists | Phase 3B (code quality review) — `docs/review-dimension-checklists.md` B-side |

---

## Quick Reference Card

```
┌─────────────────────────────────────────────────┐
│           SUPERPOWERS WORKFLOW                   │
├─────────────────────────────────────────────────┤
│                                                  │
│  1. READ context + cross-reference + rules       │
│     ↓                                            │
│  2. RED    → Write failing test                  │
│     ↓                                            │
│  3. GREEN  → Write minimal implementation        │
│     ↓                                            │
│  4. REFACTOR → Clean up, all tests still pass    │
│     ↓                                            │
│  5. REVIEW → Spec compliance → Code quality      │
│     ↓                                            │
│     Code quality includes:                        │
│     - Red lines (architecture boundaries)         │
│     - Coding standards (CS-B/F rules)             │
│     - Dimension B-side checklists                 │
│     ↓                                            │
│  6. VERIFY → Evidence before declaration         │
│                                                  │
│  ⚠️  Never skip TDD (except hotfix/typo/CSS)    │
│  ⚠️  Never declare done without evidence         │
│  ⚠️  Spec compliance > Code quality priority     │
│                                                  │
└─────────────────────────────────────────────────┘
```
