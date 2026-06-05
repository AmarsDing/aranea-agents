# session-status-frontend Specification

## Purpose
TBD - created by archiving change session-status-review-fixes. Update Purpose after archive.
## Requirements
### Requirement: Session API Mapping

The `kratosSessionToLegacy` function in `features/session/api.ts` SHALL map all session status fields from the Kratos proto response to the frontend `Session` interface. Specifically, `status_reason` and `status_changed_at` SHALL be mapped from the API response.

#### Scenario: kratosSessionToLegacy maps status_reason

WHEN `kratosSessionToLegacy` is called with a Kratos Session object containing `statusReason: "timeout"`
THEN the returned legacy Session object SHALL have `status_reason: "timeout"`

#### Scenario: kratosSessionToLegacy maps status_changed_at

WHEN `kratosSessionToLegacy` is called with a Kratos Session object containing `statusChangedAt: "2026-06-05T12:00:00Z"`
THEN the returned legacy Session object SHALL have `status_changed_at: "2026-06-05T12:00:00Z"`

#### Scenario: kratosSessionToLegacy handles empty values

WHEN `kratosSessionToLegacy` is called with a Kratos Session object where `statusReason` is undefined
THEN the returned legacy Session object SHALL have `status_reason: ""` (empty string default)

---

### Requirement: Admin Session Store WS Status Changed Listener

The Admin Session Store (`stores/session/index.ts`) SHALL register an `onSessionMutation` listener for the `status_changed` event type. Upon receiving this event, it SHALL update the corresponding session's `status`, `statusReason`, and `statusChangedAt` fields in the store.

#### Scenario: Admin store updates session on WS status_changed

WHEN a `session.status_changed` WS event is received with `session_id = "s1"`, `status = "interrupted"`, `status_reason = "timeout"`
THEN the Admin Session Store SHALL update session "s1" with the new status, reason, and timestamp

#### Scenario: Admin store ignores unknown session

WHEN a `session.status_changed` WS event is received for a session ID not in the Admin store
THEN the handler SHALL NOT throw an error

---

### Requirement: Remove Dead Code statusBadgeColor

The `statusBadgeColor` function in `components/sessions/sessionUi.ts` SHALL be removed. It uses Quasar color names that are inconsistent with the `SessionStatusBadge` component's CSS variable approach, and it has no callers.

#### Scenario: statusBadgeColor function removed

WHEN the codebase is searched for `statusBadgeColor`
THEN no references SHALL be found in any `.ts` or `.vue` file

