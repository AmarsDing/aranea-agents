## ADDED Requirements

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
- **THEN** the system SHALL extract all failure logs, truncate to the last 200 lines, and store them as a file for diagnosis

#### Scenario: No failure logs available
- **WHEN** failure logs cannot be extracted (e.g., run expired)
- **THEN** the system SHALL log a warning and skip the auto-fix attempt

### Requirement: Failure type classification
The system SHALL classify the failure type as one of: lint-error, test-failure, build-failure, or unknown, and route to the appropriate fix strategy.

#### Scenario: Lint error classification
- **WHEN** failure logs contain golangci-lint, araneactl, eslint, stylelint, commitlint, prettier, vue-tsc, or tsc error patterns
- **THEN** the system SHALL classify the failure as "lint-error" and use rule-based fix

#### Scenario: Test failure classification
- **WHEN** failure logs contain "FAIL" or "panic" or "fatal error" patterns from go test or vitest
- **THEN** the system SHALL classify the failure as "test-failure" and use LLM-based fix

#### Scenario: Build failure classification
- **WHEN** failure logs do not match lint or test patterns
- **THEN** the system SHALL classify the failure as "build-failure" and use LLM-based fix

### Requirement: Rule-based auto-fix for lint errors
The system SHALL automatically fix lint errors using rule-based tools (araneactl lint --fix, eslint --fix, stylelint --fix) without LLM involvement.

#### Scenario: Go lint errors auto-fixed
- **WHEN** failure is classified as "lint-error" with Go lint violations
- **THEN** the system SHALL run araneactl lint --fix, and additionally run eslint --fix and stylelint --fix for frontend files

#### Scenario: Frontend lint errors auto-fixed
- **WHEN** failure is classified as "lint-error" with frontend lint violations
- **THEN** the system SHALL run eslint --fix and stylelint --fix

### Requirement: Self-hosted Agent diagnosis for test and build failures
The system SHALL use the project's own Agent runtime (self-hosted aranea-agents Chat API) to analyze failure logs and generate a git patch to fix test or build failures. When the self-hosted Agent is not configured, the system SHALL fall back to pattern-based matching via `.auto-fix/scripts/pattern-fix.sh`.

#### Scenario: Self-hosted Agent generates fix patch
- **WHEN** failure is classified as "test-failure" or "build-failure" AND ARANEA_API_URL and ARANEA_AUTO_FIX_SESSION secrets are configured
- **THEN** the system SHALL send failure logs + repo context to the self-hosted Agent via `POST /v1/chat/messages` and receive a git patch

#### Scenario: Self-hosted Agent returns no viable fix
- **WHEN** the Agent determines the failure cannot be fixed automatically or returns NOFIX
- **THEN** the system SHALL skip the fix attempt and record the failure pattern

#### Scenario: Self-hosted Agent not configured — pattern-based fallback
- **WHEN** failure is classified as "test-failure" or "build-failure" AND ARANEA_API_URL or ARANEA_AUTO_FIX_SESSION secrets are NOT configured
- **THEN** the system SHALL run `.auto-fix/scripts/pattern-fix.sh` which attempts 7 known pattern fixes: race condition, nil pointer, import cycle, proto/wire sync, gofmt drift, goimports drift, go mod tidy

### Requirement: Fix verification
The system SHALL verify the auto-fix by running `go vet` and `pnpm build` locally before creating a PR.

#### Scenario: Fix passes verification
- **WHEN** the auto-fix patch is applied and verification passes
- **THEN** the system SHALL proceed to check protected files and create an auto-fix PR

#### Scenario: Fix fails verification
- **WHEN** the auto-fix patch is applied but verification fails
- **THEN** the system SHALL discard the fix and record the failure pattern

### Requirement: Auto-fix PR creation
The system SHALL create a pull request with the auto-fix changes, labeled "auto-fix", with a descriptive title referencing the failed CI run. The PR body SHALL include the failure type and diagnosis method.

#### Scenario: Auto-fix PR created
- **WHEN** fix verification passes and protected file check passes
- **THEN** the system SHALL create a PR on a branch named "auto-fix/<run-id>" with label "auto-fix", including failure type and diagnosis method in the PR body

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
- **THEN** the system SHALL discard the fix (git checkout -- .) and reject the patch

### Requirement: Failure pattern knowledge base
The system SHALL record each auto-fix attempt (failure type, logs summary, fix attempted, fix succeeded) to .auto-fix/patterns.jsonl for future learning. The system SHALL also update .auto-fix/stats.json with daily count, total attempts, and total successes.

#### Scenario: Pattern recorded on success
- **WHEN** an auto-fix attempt succeeds
- **THEN** the system SHALL append a success record to .auto-fix/patterns.jsonl and increment total_successes in stats.json

#### Scenario: Pattern recorded on failure
- **WHEN** an auto-fix attempt fails
- **THEN** the system SHALL append a failure record to .auto-fix/patterns.jsonl and update total_attempts in stats.json
