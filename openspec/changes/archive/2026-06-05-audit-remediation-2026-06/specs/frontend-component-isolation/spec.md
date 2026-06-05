## ADDED Requirements

### Requirement: Display components must not import Store
Components in `web/src/components/` SHALL NOT directly import or call `useXxxStore`, `axios`, or `kratosApi`. Data MUST flow in via props; actions MUST flow out via emits.

#### Scenario: SelfCheckStatusPanel receives data via props
- **WHEN** SelfCheckStatusPanel renders
- **THEN** it receives `loading`, `triggering`, `latestReport` as props and emits `refresh`/`trigger` events

#### Scenario: ProviderLogo receives fetch function via props
- **WHEN** ProviderLogo renders
- **THEN** it receives `fetchSvg` as a prop function and calls it to load the SVG

### Requirement: Page layer provides Store data to components
Pages and container components SHALL be responsible for connecting Stores to display components via props and handling emitted events.

#### Scenario: Monitor page wires Store to SelfCheckStatusPanel
- **WHEN** the Monitor page renders SelfCheckStatusPanel
- **THEN** it passes Store state as props and handles events by calling Store actions
