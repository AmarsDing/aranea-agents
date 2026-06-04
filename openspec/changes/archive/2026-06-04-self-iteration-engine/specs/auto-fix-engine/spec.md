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
