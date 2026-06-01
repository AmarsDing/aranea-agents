---
name: "aranea-test-loop"
description: "Aranea-Agents automated test loop skill. Invoke when running tests, generating test reports, fixing test failures, or executing the develop-test-fix-retest-release cycle."
---

# Aranea-Agents Test Loop Skill

Automated test execution, report generation, failure analysis, and fix loop for the Aranea-Agents project.

## 1. When to Invoke

- User asks to run tests or check system health
- User asks to generate a test report
- User asks to fix test failures
- User asks to execute the full test loop (develop → test → fix → retest → release)
- Before any release or merge

## 2. Test Loop Process

```
Develop → Test → Report → Fix → Retest → Release
                                    ↑          │
                                    └──────────┘ (if FAIL, max 3 rounds)
```

### 2.1 Maximum Loop Count

- **3 rounds** maximum for automatic fix-retest loop
- If same error appears **2 consecutive rounds**, mark as **stubborn issue** and request human intervention
- After 3 rounds with remaining failures, generate **blocked report** and stop

### 2.2 Fix Priority

| Priority | Condition | Action |
|----------|-----------|--------|
| P0 | Build failure / panic | Fix immediately, block release |
| P1 | Core business logic failure | Must fix this round |
| P2 | Non-core feature failure | Can defer to next round |
| P3 | Lint warning / style | Batch fix |

## 3. Test Execution Steps

Execute tests in this exact order. Stop and report on first P0 failure.

### Step 1: Backend Unit Tests (L1-L2)

```bash
# Full backend test suite with coverage
go test -cover -coverprofile=coverage.out ./internal/... 2>&1

# If needed, run by layer:
# go test ./internal/biz/... -count=1
# go test ./internal/service/... -count=1
# go test ./internal/data/... -count=1
# go test ./internal/runtime/... -count=1
# go test ./internal/team/... -count=1
# go test ./internal/tools/... -count=1
```

### Step 2: Frontend Tests (L1)

```bash
cd web && pnpm test 2>&1
```

### Step 3: Code Quality Gates

```bash
# Backend lint
make lint 2>&1

# Backend smoke (compile check)
make smoke 2>&1

# Frontend lint
cd web && pnpm lint 2>&1

# Frontend build
cd web && pnpm build 2>&1

# Frontend layer compliance
cd web && pnpm check:layer 2>&1
```

### Step 4: Generation Sync Checks

```bash
# Wire generation sync
make wire-clean 2>&1

# Proto generation sync
make proto-clean 2>&1
```

### Step 5: ModelCatalog Overlay Check

```bash
make check-overlay 2>&1
```

## 4. Report Generation

After all tests complete, generate a report following the template at `docs/testing/test-report-template.md`.

### 4.1 Report Naming

```
docs/testing/reports/report-YYYYMMDD-HHMMSS.md
```

### 4.2 Report Structure

Every report MUST contain:

1. **Header**: Report ID, timestamp, executor, trigger reason, branch/commit, loop round
2. **Overall Results**: Total/PASS/FAIL/SKIP/ERROR counts, pass rate, coverage
3. **Backend Results**: Per-layer breakdown, failure details with root cause analysis
4. **Frontend Results**: Per-check breakdown, failure details
5. **Fix Record**: What was fixed, how, verification result
6. **Comparison**: Delta vs previous report
7. **Release Gate Checklist**: All 11 gates must pass
8. **Next Actions**: Prioritized action items

### 4.3 Root Cause Analysis Rules

For each failure, analyze and classify:

| Category | Pattern | Typical Fix |
|----------|---------|-------------|
| Compilation Error | Syntax/type error | Fix code syntax |
| Logic Error | Wrong assertion/condition | Fix business logic |
| Missing Mock | Unmet interface | Add mock struct |
| Race Condition | Intermittent failure | Add synchronization |
| Environment | External dependency unavailable | Skip or mock |
| Regression | Previously passing test now fails | Bisect recent changes |
| Flaky Test | Non-deterministic pass/fail | Stabilize or quarantine |

## 5. Fix Execution Rules

### 5.1 Fix Scope

- **ONLY fix files directly related to the failure**
- Do NOT refactor adjacent modules
- Do NOT add features while fixing
- Follow project coding conventions (aranea-coding-guide / aranea-frontend-guide)

### 5.2 Fix Verification

After each fix:
1. Run the specific failing test to confirm fix: `go test ./path/to/... -run TestName -count=1`
2. Run the full layer test suite to check for regressions
3. Update report with fix result

### 5.3 Fix Restrictions

- Backend: Use `kerrors` for errors, never `fmt.Errorf`
- Backend: All goroutines must use `pkg/safego`
- Backend: Follow dependency direction (api → service → biz → data)
- Frontend: Follow data flow (api.ts → store → composable → page → component)
- Frontend: No direct API calls in components
- Do NOT modify tool-generated code (protoc/wire/ent)

## 6. Release Gate Checklist

ALL gates must pass before release:

| # | Gate | Command | Must Be |
|---|------|---------|---------|
| 1 | Backend tests | `make test` | All PASS |
| 2 | Backend lint | `make lint` | 0 errors |
| 3 | Backend smoke | `make smoke` | Build success |
| 4 | Wire sync | `make wire-clean` | No diff |
| 5 | Proto sync | `make proto-clean` | No diff |
| 6 | Frontend tests | `pnpm test` | All PASS |
| 7 | Frontend lint | `pnpm lint` | 0 errors |
| 8 | Frontend build | `pnpm build` | Build success |
| 9 | Layer compliance | `pnpm check:layer` | 0 violations |
| 10 | Coverage | `go test -cover` | ≥ 40% |
| 11 | No P0/P1 defects | Report check | 0 remaining |

## 7. Test Data

All test data and sample fixtures are in `docs/testing/test-data/`:

| File | Purpose |
|------|---------|
| `sample-agent-config.json` | Agent creation test data |
| `sample-chat-messages.json` | Chat message scenarios |
| `sample-tool-definitions.json` | Tool registration test data |
| `sample-webhook-config.json` | Webhook configuration test data |
| `sample-graph-definition.json` | Graph definition test data |
| `sample-team-config.json` | Team configuration test data |
| `error-codes.json` | Error code reference for assertions |
| `test-users.json` | Test user and workspace data |

## 8. Reference Documents

| Document | Path |
|----------|------|
| Test Plan | `docs/testing/test-plan.md` |
| Loop Process | `docs/testing/test-loop-process.md` |
| Report Template | `docs/testing/test-report-template.md` |
| Architecture Blueprint | `docs/architecture-blueprint.md` |
| Module Cross Reference | `docs/module-cross-reference.md` |

## 9. Execution Workflow

When this skill is invoked, follow this exact workflow:

```
1. Read test plan: docs/testing/test-plan.md
2. Execute Step 1-5 (backend + frontend + quality gates)
3. Parse all outputs, count PASS/FAIL/SKIP/ERROR
4. Generate report to docs/testing/reports/report-YYYYMMDD-HHMMSS.md
5. If any FAIL:
   a. Analyze root cause for each failure
   b. Implement fixes (respect fix scope rules)
   c. Re-run failing tests
   d. If all fixed, re-run full suite
   e. Update report
   f. Repeat up to 3 rounds
6. Final report with release gate checklist
7. If all gates pass → recommend release
8. If gates blocked → list remaining issues for human
```
