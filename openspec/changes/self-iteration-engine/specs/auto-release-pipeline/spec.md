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
The system SHALL build binaries for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64, windows/arm64 using GoReleaser.

#### Scenario: All platform binaries built
- **WHEN** the release workflow runs
- **THEN** GoReleaser SHALL produce binaries for all 6 platform/arch combinations (3 OS × 2 arch)

### Requirement: Docker image build and push
The system SHALL build Docker images for the admin server and push them to GitHub Container Registry (ghcr.io).

#### Scenario: Docker images pushed
- **WHEN** the release workflow runs
- **THEN** Docker images SHALL be built and pushed with tags <version> and "latest" to ghcr.io

### Requirement: Changelog generation
The system SHALL auto-generate a Changelog from conventional commits using GoReleaser's changelog feature, grouped by: Features, Bug fixes, Others.

#### Scenario: Changelog generated
- **WHEN** the release workflow runs
- **THEN** a Changelog SHALL be generated and attached to the GitHub Release

### Requirement: Version metadata injection
The system SHALL inject version and name into the binary via ldflags at build time.

#### Scenario: Version info available at runtime
- **WHEN** the admin binary is built
- **THEN** the binary SHALL contain version and name metadata accessible via --version flag

### Requirement: Release workflow runs CI first
The system SHALL run the full CI workflow before proceeding with the release.

#### Scenario: CI runs before release
- **WHEN** the release workflow is triggered
- **THEN** the CI workflow SHALL run first and the release job SHALL only proceed if CI passes

### Requirement: Archive generation
The system SHALL generate release archives in tar.gz format (zip for Windows) for both admin and araneactl binaries.

#### Scenario: Archives generated
- **WHEN** the release workflow runs
- **THEN** GoReleaser SHALL produce archives for admin and araneactl with naming template: <project>_<version>_<os>_<arch>

### Requirement: Staging deployment (deferred)
The system SHALL deploy the new version to a staging environment and run smoke tests before allowing production promotion. **Current status: deferred — release workflow only includes GoReleaser + Docker push. Staging/production steps require infrastructure setup.**

#### Scenario: Staging deployment succeeds (future)
- **WHEN** the Docker image is pushed
- **THEN** the system SHALL deploy to staging and run health check

#### Scenario: Staging smoke test fails (future)
- **WHEN** the staging health check fails
- **THEN** the system SHALL block production promotion and create an alert

### Requirement: Production promotion requires manual approval (deferred)
The system SHALL NOT automatically deploy to production; production promotion MUST require manual approval. **Current status: deferred.**

#### Scenario: Manual production promotion (future)
- **WHEN** staging smoke tests pass
- **THEN** the system SHALL wait for manual approval before deploying to production
