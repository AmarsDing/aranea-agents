## ADDED Requirements

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
