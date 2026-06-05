## ADDED Requirements

### Requirement: Application lifecycle context
The system SHALL provide a global application-lifecycle context via `pkg/appctx` that is initialized at server startup and cancelled at server shutdown.

#### Scenario: Background goroutine cancelled on shutdown
- **WHEN** the server starts and calls `appctx.Init()`
- **THEN** `appctx.Ctx()` returns a non-nil context that is cancelled when `appctx.Cancel()` is called

#### Scenario: All safego.Go calls use appctx.Ctx()
- **WHEN** a background goroutine is started via `safego.Go(context.Background(), ...)`
- **THEN** it MUST be replaced with `safego.Go(appctx.Ctx(), ...)` so the goroutine is cancelled on shutdown

### Requirement: appctx package API
The `pkg/appctx` package SHALL export exactly three functions: `Init()`, `Ctx() context.Context`, `Cancel()`.

#### Scenario: Init called once
- **WHEN** `Init()` is called
- **THEN** a new `context.WithCancel(context.Background())` is created and stored

#### Scenario: Ctx before Init
- **WHEN** `Ctx()` is called before `Init()`
- **THEN** it returns `context.Background()` (safe fallback)
