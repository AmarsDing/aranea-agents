# testutil — Test Helpers

This package provides lightweight, dependency-minimal helpers for Aranea unit tests.

## Why is this needed?

The `internal/data/ent` package currently panics on `init()` due to a type assertion
on a nil default value in `runtime.go:~1500`:

```
session.DefaultContextUsedRatio = sessionDescContextUsedRatio.Default.(float64)
// panics when Default is nil (schema field ordering issue)
```

This means **any package that transitively imports `internal/data/ent` cannot be tested**
until the generated code is regenerated with a compatible Ent schema.

**Fix**: Run `go generate ./internal/data/ent/...` after ensuring the session schema's
`context_used_ratio` field has an explicit default of `0.0`.

## Safe packages (no Ent dependency)

| Package | Current Coverage |
|---------|-----------------|
| `internal/event` | ~28% |
| `internal/workspace` | 100% |
| `internal/agent/callbacks` | ~50% |
| `internal/testutil` | 100% |
| `pkg/apierror` | ~79% |
| `pkg/safego` | 100% |

## Helpers in this package

- `RecordingBus` — an `event.Bus` that records published envelopes for assertions.
  Use in tests that need to assert on EventBus output without a running process.
