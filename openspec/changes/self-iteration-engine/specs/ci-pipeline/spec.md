## MODIFIED Requirements

### Requirement: CI pipeline stages
The CI pipeline SHALL consist of 4 stages with 12 jobs total:

Stage 1 (parallel): lint-go, lint-web, proto-clean, wire-clean, commitlint, typecheck-web
Stage 2 (depends on lint): test-go, test-web, test-integration
Stage 3 (depends on test): smoke, coverage-gate
Stage 4 (parallel): security-scan, doc-sync-check

#### Scenario: Full CI pipeline runs on PR
- **WHEN** a PR is opened against main/master
- **THEN** all 12 jobs SHALL run in the correct stage order

#### Scenario: Commitlint job validates commit messages
- **WHEN** the commitlint job runs
- **THEN** it SHALL validate that all commit messages follow conventional commits format

#### Scenario: Typecheck-web job validates TypeScript
- **WHEN** the typecheck-web job runs
- **THEN** it SHALL run vue-tsc --noEmit to check TypeScript types

#### Scenario: Test-integration job runs integration tests
- **WHEN** the test-integration job runs
- **THEN** it SHALL run Go integration tests using testcontainers-go with a real PostgreSQL container

#### Scenario: Coverage-gate enforces threshold
- **WHEN** the coverage-gate job runs
- **THEN** it SHALL enforce the coverage threshold (40% for M3, 60% for M4, 70% for M5)

#### Scenario: Security-scan runs CodeQL and Trivy
- **WHEN** the security-scan job runs
- **THEN** it SHALL run CodeQL analysis and Trivy container vulnerability scan

#### Scenario: Doc-sync-check validates documentation consistency
- **WHEN** the doc-sync-check job runs
- **THEN** it SHALL verify that proto-generated code is consistent (proto-clean) and that OpenAPI specs are up-to-date

### Requirement: E2E nightly job
The CI pipeline SHALL include a separate nightly workflow that runs E2E tests at 03:00 UTC daily.

#### Scenario: Nightly E2E runs
- **WHEN** 03:00 UTC arrives
- **THEN** the E2E nightly workflow SHALL run Playwright tests against a deployed staging environment
