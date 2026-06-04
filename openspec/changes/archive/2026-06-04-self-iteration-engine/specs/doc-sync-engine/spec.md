## ADDED Requirements

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
