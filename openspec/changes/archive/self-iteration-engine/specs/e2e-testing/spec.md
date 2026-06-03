## ADDED Requirements

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
