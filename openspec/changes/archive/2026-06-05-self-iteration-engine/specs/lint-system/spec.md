## MODIFIED Requirements

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
