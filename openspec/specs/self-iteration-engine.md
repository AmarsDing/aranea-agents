# Self Iteration Engine

## Auto Fix Engine

### Requirement: CI failure auto-detection
The system SHALL detect CI workflow failures via GitHub Actions `workflow_run` event (conclusion: failure) and trigger the auto-fix pipeline automatically.

#### Scenario: CI test failure triggers auto-fix
- **WHEN** a CI workflow run completes with conclusion "failure"
- **THEN** the auto-fix workflow SHALL be triggered within 60 seconds

#### Scenario: Successful CI run does not trigger auto-fix
- **WHEN** a CI workflow run completes with conclusion "success"
- **THEN** the auto-fix workflow SHALL NOT be triggered

### Requirement: Failure log extraction
The system SHALL extract failure logs from the failed CI run using `gh run view --log-failed` and make them available for diagnosis.

#### Scenario: Failure logs are extracted
- **WHEN** auto-fix is triggered for a failed CI run
- **THEN** the system SHALL extract all failure logs and store them as a file for diagnosis

#### Scenario: No failure logs available
- **WHEN** failure logs cannot be extracted (e.g., run expired)
- **THEN** the system SHALL log a warning and skip the auto-fix attempt

### Requirement: Failure type classification
The system SHALL classify the failure type as one of: lint-error, test-failure, build-failure, or unknown, and route to the appropriate fix strategy.

#### Scenario: Lint error classification
- **WHEN** failure logs contain golangci-lint, araneactl, or eslint error patterns
- **THEN** the system SHALL classify the failure as "lint-error" and use rule-based fix

#### Scenario: Test failure classification
- **WHEN** failure logs contain "FAIL" or "panic" patterns from go test or vitest
- **THEN** the system SHALL classify the failure as "test-failure" and use LLM-based fix

#### Scenario: Build failure classification
- **WHEN** failure logs contain "cannot compile" or "build failed" patterns
- **THEN** the system SHALL classify the failure as "build-failure" and use LLM-based fix

### Requirement: Rule-based auto-fix for lint errors
The system SHALL automatically fix lint errors using rule-based tools (golangci-lint --fix, eslint --fix, stylelint --fix, gofmt, goimports) without LLM involvement.

#### Scenario: Go lint errors auto-fixed
- **WHEN** failure is classified as "lint-error" with Go lint violations
- **THEN** the system SHALL run golangci-lint --fix, gofmt -w, and goimports -w

#### Scenario: Frontend lint errors auto-fixed
- **WHEN** failure is classified as "lint-error" with frontend lint violations
- **THEN** the system SHALL run eslint --fix and stylelint --fix

### Requirement: LLM-based auto-fix for test and build failures
The system SHALL use an LLM API to analyze failure logs and generate a git patch to fix test or build failures.

#### Scenario: LLM generates fix patch
- **WHEN** failure is classified as "test-failure" or "build-failure"
- **THEN** the system SHALL send failure logs + repo context to the LLM and receive a git patch

#### Scenario: LLM returns no viable fix
- **WHEN** the LLM determines the failure cannot be fixed automatically
- **THEN** the system SHALL skip the fix attempt and record the failure pattern

### Requirement: Fix verification
The system SHALL verify the auto-fix by running the same CI checks that failed (make test / make lint) locally before creating a PR.

#### Scenario: Fix passes verification
- **WHEN** the auto-fix patch is applied and verification passes
- **THEN** the system SHALL proceed to create an auto-fix PR

#### Scenario: Fix fails verification
- **WHEN** the auto-fix patch is applied but verification fails
- **THEN** the system SHALL discard the fix and record the failure pattern

### Requirement: Auto-fix PR creation
The system SHALL create a pull request with the auto-fix changes, labeled "auto-fix", with a descriptive title referencing the failed CI run.

#### Scenario: Auto-fix PR created
- **WHEN** fix verification passes
- **THEN** the system SHALL create a PR on a branch named "auto-fix/<run-id>" with label "auto-fix"

#### Scenario: Auto-fix PR requires human review
- **WHEN** an auto-fix PR is created
- **THEN** the PR SHALL require at least one human approval before merging

### Requirement: Daily fix limit
The system SHALL limit auto-fix attempts to a configurable maximum per day (default: 10) to control API costs.

#### Scenario: Daily limit reached
- **WHEN** auto-fix attempts for the day reach the configured limit
- **THEN** subsequent CI failures SHALL NOT trigger auto-fix until the next day

### Requirement: Protected file exclusion
The system SHALL NOT auto-fix files in protected paths: .github/workflows/, Makefile, go.mod, go.sum, api/kratos/**/*.proto.

#### Scenario: Fix touches protected file
- **WHEN** the generated fix patch modifies a protected file
- **THEN** the system SHALL discard the fix and log a warning

### Requirement: Failure pattern knowledge base
The system SHALL record each auto-fix attempt (failure type, logs summary, fix attempted, fix succeeded) to .auto-fix/patterns.jsonl for future learning.

#### Scenario: Pattern recorded on success
- **WHEN** an auto-fix attempt succeeds
- **THEN** the system SHALL append a success record to .auto-fix/patterns.jsonl

#### Scenario: Pattern recorded on failure
- **WHEN** an auto-fix attempt fails
- **THEN** the system SHALL append a failure record to .auto-fix/patterns.jsonl

## Auto Release Pipeline

### Requirement: Tag-triggered release
The system SHALL automatically trigger a release when a version tag (v*) is pushed to the repository.

#### Scenario: Version tag triggers release
- **WHEN** a tag matching "v*" is pushed
- **THEN** the release workflow SHALL be triggered

#### Scenario: Non-version tag does not trigger release
- **WHEN** a tag not matching "v*" is pushed
- **THEN** the release workflow SHALL NOT be triggered

### Requirement: Multi-platform binary build
The system SHALL build binaries for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64 using GoReleaser.

#### Scenario: All platform binaries built
- **WHEN** the release workflow runs
- **THEN** GoReleaser SHALL produce binaries for all 5 platform/arch combinations

### Requirement: Docker image build and push
The system SHALL build Docker images for the admin server and push them to GitHub Container Registry (ghcr.io).

#### Scenario: Docker images pushed
- **WHEN** the release workflow runs
- **THEN** Docker images SHALL be built and pushed with tags <version> and "latest"

### Requirement: Changelog generation
The system SHALL auto-generate a Changelog from conventional commits using GoReleaser's changelog feature, grouped by: Features, Bug fixes, Others.

#### Scenario: Changelog generated
- **WHEN** the release workflow runs
- **THEN** a Changelog SHALL be generated and attached to the GitHub Release

### Requirement: Staging deployment with smoke test
The system SHALL deploy the new version to a staging environment and run smoke tests before allowing production promotion.

#### Scenario: Staging deployment succeeds
- **WHEN** the Docker image is pushed
- **THEN** the system SHALL deploy to staging and run health check

#### Scenario: Staging smoke test fails
- **WHEN** the staging health check fails
- **THEN** the system SHALL block production promotion and create an alert

### Requirement: Production promotion requires manual approval
The system SHALL NOT automatically deploy to production; production promotion MUST require manual approval.

#### Scenario: Manual production promotion
- **WHEN** staging smoke tests pass
- **THEN** the system SHALL wait for manual approval before deploying to production

### Requirement: Version metadata injection
The system SHALL inject version, commit hash, and build date into the binary via ldflags at build time.

#### Scenario: Version info available at runtime
- **WHEN** the admin binary is built
- **THEN** the binary SHALL contain version, commit, and date metadata accessible via --version flag

## CI Pipeline

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

## Doc Sync Engine

### Requirement: Code change impact detection
The system SHALL detect which documentation files are affected when a PR is merged to main, by analyzing changed file paths against a mapping of code-to-doc relationships.

#### Scenario: Proto file change detected
- **WHEN** a PR changes files under api/kratos/
- **THEN** the system SHALL identify openspec/specs/architecture-blueprint.md and OpenAPI docs as affected

#### Scenario: Biz layer change detected
- **WHEN** a PR changes files under internal/biz/
- **THEN** the system SHALL identify openspec/specs/module-cross-reference.md as affected

#### Scenario: No documentation impact
- **WHEN** a PR only changes test files or documentation files
- **THEN** the system SHALL NOT trigger doc sync

### Requirement: OpenAPI regeneration on proto change
The system SHALL automatically regenerate OpenAPI specs when proto files are changed, using `make api`.

#### Scenario: OpenAPI regenerated
- **WHEN** proto files are changed in a merged PR
- **THEN** the system SHALL run `make api` and include the regenerated OpenAPI in the doc-sync PR

### Requirement: LLM-assisted spec document update
The system SHALL use an LLM to update affected spec documents based on code changes, preserving existing structure and style.

#### Scenario: Spec document updated
- **WHEN** a code change affects a spec document
- **THEN** the system SHALL send the current spec content + code diff to the LLM and generate an updated version

#### Scenario: LLM update preserves structure
- **WHEN** the LLM generates an updated spec
- **THEN** the output SHALL preserve the existing document structure, only modifying relevant sections

### Requirement: Doc-sync PR creation
The system SHALL create a pull request with documentation updates, labeled "docs", referencing the source PR.

#### Scenario: Doc-sync PR created
- **WHEN** documentation updates are generated
- **THEN** the system SHALL create a PR on branch "doc-sync/<source-pr-number>" with label "docs"

### Requirement: Critical spec notification only
The system SHALL NOT auto-update critical specs (architecture-blueprint.md, module-cross-reference.md); instead it SHALL create a GitHub Issue notifying maintainers to review.

#### Scenario: Critical spec affected
- **WHEN** a code change affects architecture-blueprint.md or module-cross-reference.md
- **THEN** the system SHALL create a GitHub Issue with the change details instead of auto-updating

### Requirement: Changelog auto-generation
The system SHALL automatically generate a changelog entry in openspec/changelog/ when a PR is merged to main.

#### Scenario: Changelog entry created
- **WHEN** a PR is merged to main
- **THEN** the system SHALL create a file at openspec/changelog/<YYYY-MM-DD>-pr<number>.md with PR title, author, labels, and description

## E2E Testing

### Requirement: Playwright E2E test framework setup
The system SHALL set up Playwright as the E2E testing framework under web/e2e/ with TypeScript configuration.

#### Scenario: Playwright installed and configured
- **WHEN** the E2E framework is set up
- **THEN** web/e2e/ SHALL contain playwright.config.ts and at least one test file

#### Scenario: Playwright browsers installed
- **WHEN** pnpm install is run in web/
- **THEN** Playwright browsers SHALL be installed via postinstall script

### Requirement: Critical path E2E test coverage
The system SHALL implement E2E tests for the following critical user paths: chat flow, agent creation, team orchestration.

#### Scenario: Chat flow E2E test
- **WHEN** the E2E test suite runs
- **THEN** a test SHALL verify: create session → send message → receive assistant reply

#### Scenario: Agent creation E2E test
- **WHEN** the E2E test suite runs
- **THEN** a test SHALL verify: navigate to agent page → create agent → verify agent appears in list

#### Scenario: Team orchestration E2E test
- **WHEN** the E2E test suite runs
- **THEN** a test SHALL verify: create team → add agents → run team → verify execution

### Requirement: Nightly E2E CI job
The system SHALL run E2E tests nightly (03:00 UTC) and on manual trigger, not on every PR.

#### Scenario: Nightly E2E runs
- **WHEN** the scheduled time arrives (03:00 UTC daily)
- **THEN** the E2E workflow SHALL run against the main branch

#### Scenario: E2E on manual trigger
- **WHEN** a user triggers the E2E workflow manually
- **THEN** the E2E tests SHALL run

### Requirement: E2E test artifact collection
The system SHALL collect Playwright traces and screenshots on test failure and upload them as CI artifacts.

#### Scenario: Failure artifacts collected
- **WHEN** an E2E test fails
- **THEN** the system SHALL upload Playwright traces and screenshots as GitHub Actions artifacts

### Requirement: E2E test retry mechanism
The system SHALL retry failed E2E tests up to 2 times to handle flaky tests.

#### Scenario: Flaky test retry
- **WHEN** an E2E test fails on the first attempt
- **THEN** the system SHALL retry the test up to 2 more times before marking it as failed

## Iteration Dashboard

### Requirement: Weekly iteration report generation
The system SHALL generate a weekly iteration report every Monday at 06:00 UTC, collecting metrics from the past week.

#### Scenario: Weekly report generated
- **WHEN** Monday 06:00 UTC arrives
- **THEN** the system SHALL generate a report with: coverage trend, test failure rate, build time, auto-fix stats, release count

### Requirement: Coverage trend tracking
The system SHALL track Go test coverage percentage over time and include it in the iteration report.

#### Scenario: Coverage trend reported
- **WHEN** the weekly report is generated
- **THEN** it SHALL include current coverage %, previous week coverage %, and trend direction

### Requirement: Auto-fix success rate tracking
The system SHALL track auto-fix attempt count, success count, and manual intervention count, and include them in the iteration report.

#### Scenario: Auto-fix stats reported
- **WHEN** the weekly report is generated
- **THEN** it SHALL include: weekly fix attempts, weekly success rate, weekly manual interventions, cumulative stats

### Requirement: Release frequency tracking
The system SHALL track release count and average release interval, and include them in the iteration report.

#### Scenario: Release stats reported
- **WHEN** the weekly report is generated
- **THEN** it SHALL include: weekly release count, weekly rollback count, average release interval

### Requirement: Dashboard issue creation
The system SHALL create a GitHub Issue with the iteration report, labeled "dashboard", for team review and discussion.

#### Scenario: Dashboard issue created
- **WHEN** the weekly report is generated
- **THEN** a GitHub Issue SHALL be created with title "Iteration Dashboard - <year>-W<week>" and label "dashboard"

## Lint System

### Requirement: Pre-commit lint enforcement
The system SHALL run lint checks on staged files before each commit using Husky + lint-staged, blocking commits that fail lint.

#### Scenario: Go files linted before commit
- **WHEN** a developer commits staged .go files
- **THEN** lint-staged SHALL run gofmt -w and go vet on the staged files

#### Scenario: Frontend files linted before commit
- **WHEN** a developer commits staged .ts or .vue files
- **THEN** lint-staged SHALL run eslint --fix and stylelint --fix on the staged files

#### Scenario: Proto files checked before commit
- **WHEN** a developer commits staged .proto files
- **THEN** lint-staged SHALL run buf format -w on the staged files

#### Scenario: Commit blocked on lint failure
- **WHEN** a lint check fails on staged files
- **THEN** the commit SHALL be blocked with an error message

### Requirement: Commit message format enforcement
The system SHALL validate commit messages against conventional commits format using commitlint.

#### Scenario: Valid conventional commit
- **WHEN** a commit message follows conventional format (e.g., "feat(chat): add message grouping")
- **THEN** the commit SHALL be allowed

#### Scenario: Invalid commit message rejected
- **WHEN** a commit message does not follow conventional format
- **THEN** the commit SHALL be rejected with a descriptive error

### Requirement: ESLint configuration for frontend
The system SHALL configure ESLint with flat config (eslint.config.js) for Vue 3 + TypeScript + Quasar, covering code quality and best practices rules.

#### Scenario: ESLint runs on frontend code
- **WHEN** eslint is run on web/src/
- **THEN** it SHALL check all .ts and .vue files for code quality issues

### Requirement: Prettier configuration for frontend
The system SHALL configure Prettier for consistent code formatting across the frontend codebase.

#### Scenario: Prettier formats frontend code
- **WHEN** prettier is run on web/src/
- **THEN** all .ts, .vue, .css, .scss files SHALL be formatted consistently

### Requirement: araneactl --fix mode
The araneactl lint command SHALL support a --fix flag that automatically fixes lint violations where possible.

#### Scenario: araneactl --fix runs all auto-fixers
- **WHEN** araneactl lint --fix is run
- **THEN** it SHALL run golangci-lint --fix, gofmt -w, goimports -w, eslint --fix, and stylelint --fix

#### Scenario: araneactl --fix reports unfixable violations
- **WHEN** araneactl lint --fix encounters violations that cannot be auto-fixed
- **THEN** it SHALL report the remaining violations to the user
