## ADDED Requirements

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
