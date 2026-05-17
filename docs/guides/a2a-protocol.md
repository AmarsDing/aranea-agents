# A2A Protocol Guide

> Sprint 6 — T40 | M-9

The Agent-to-Agent (A2A) protocol enables structured communication between agents within the Aranea platform. An agent may invoke a named capability on another agent using the `call_agent` tool, subject to explicit enablement, capability advertisement, and workspace isolation.

---

## Core Principles

1. **Opt-in by default** — every agent starts with A2A disabled. An administrator or the agent owner must explicitly enable A2A and publish its capability list.
2. **Workspace isolation** — cross-workspace calls are forbidden at the biz layer. The `Discover` and `Invoke` endpoints only return/accept agents within the same workspace.
3. **Audit log** — every invocation (success or failure) writes an `a2a_audit` record.
4. **Minimal trust surface** — the `call_agent` tool verifies the card before dispatching.

---

## Message Format

### Invocation Request

```json
{
  "callee_agent_id": "agent-456",
  "capability": "summarize",
  "payload_json": "{\"text\": \"Long document...\"}",
  "caller_session_id": "sess-789",
  "timeout_seconds": 30
}
```

### Invocation Response

```json
{
  "invoke_id": "a2a-xxxxxxxx",
  "status": "pending | success | error | timeout",
  "result_json": "{}",
  "error_message": "",
  "duration_ms": 142
}
```

---

## Agent Card

Each A2A-enabled agent publishes a card:

```json
{
  "agent_id": "agent-123",
  "display_name": "Research Assistant",
  "workspace": "workspace-A",
  "enabled": true,
  "capabilities": [
    {
      "name": "summarize",
      "description": "Summarize a block of text.",
      "input_schema_json": "{\"type\":\"object\",\"properties\":{\"text\":{\"type\":\"string\"}}}",
      "output_schema_json": "{\"type\":\"object\",\"properties\":{\"summary\":{\"type\":\"string\"}}}"
    }
  ]
}
```

---

## Components

| Component | Path | Purpose |
|-----------|------|---------|
| Proto | `api/kratos/a2a/v1/a2a.proto` | HTTP + gRPC API |
| Biz | `internal/biz/a2a.go` | Domain types + `A2ARepo` interface |
| Data | `internal/data/a2a.go` | SQLite/Postgres persistence |
| Tool | `internal/a2a/tool.go` | `call_agent` trpc tool |
| Service | `internal/service/a2a.go` | Kratos service adapter |

---

## Database Schema

Tables created by `data.EnsureA2ASchema(ctx, db)`:

```sql
a2a_agent_cards   (agent_id PK, display_name, workspace, enabled, capabilities JSON, updated_at)
a2a_invocations   (id PK, caller_agent_id, callee_agent_id, capability, payload_json, status, ...)
a2a_audit         (id PK, invoke_id, caller_agent_id, callee_agent_id, status, duration_ms, ...)
```

---

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/v1/a2a/discover` | Discover enabled agents |
| POST | `/v1/a2a/invoke` | Invoke a capability |
| PUT | `/v1/a2a/agents/{agent_id}/card` | Update agent's A2A card |
| GET | `/v1/a2a/agents/{agent_id}/card` | Get agent's A2A card |
| GET | `/v1/a2a/audit` | Browse audit log |

---

## Agent Tool: `call_agent`

Attach context helpers before running:

```go
ctx = a2a.WithA2AUsecase(ctx, a2aUsecase)
ctx = a2a.WithCallerAgentID(ctx, "agent-123")
ctx = a2a.WithInvoker(ctx, myInvokerFunc)
```

The model can then call:

```json
{
  "agent_id": "agent-456",
  "capability": "summarize",
  "payload": { "text": "Long document..." },
  "timeout_seconds": 30
}
```

The tool:
1. Verifies the callee's card (`enabled=true`).
2. Verifies the capability is in the card's list.
3. Calls `invokerFunc` to dispatch the actual work.
4. Writes an audit entry (success or error).

---

## Security

| Control | Implementation |
|---------|---------------|
| Opt-in | `enabled=false` by default for every agent |
| Workspace isolation | `ListEnabledCards` filters by workspace |
| Audit | Every call writes `a2a_audit` with caller/callee/status |
| Rate limiting | Recommended: apply API-gateway rate limit on `/v1/a2a/invoke` |

---

## Prometheus Metrics

| Metric | Labels | Description |
|--------|--------|-------------|
| `aranea_a2a_invoke_total` | `caller, callee, status` | Total invoke calls |
| `aranea_a2a_invoke_duration_seconds` | — | Invoke latency histogram |

---

## Routing

- **Same workspace**: direct call through the service layer.
- **Cross-workspace**: forbidden by default; future gateway routing is a S7+ candidate.

---

## Acceptance Criteria

- Agent A calling Agent B in a different workspace → `403 Forbidden`.
- Agent A calling A2A-disabled Agent B in the same workspace → error from `call_agent`.
- Agent A calling A2A-enabled Agent B (same workspace) → succeeds; audit log record written.
