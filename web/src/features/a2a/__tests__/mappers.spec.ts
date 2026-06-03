import { describe, it, expect } from 'vitest';
import { mapAgentCard, mapAuditEntry } from '../mappers';

describe('a2a mappers', () => {
  it('mapAgentCard normalizes capabilities', () => {
    const card = mapAgentCard({
      agent_id: 'ag-1',
      display_name: 'Demo',
      workspace: 'ws',
      enabled: true,
      capabilities: [{ name: 'ping', description: 'ping' }],
      updated_at: '2026-05-20T00:00:00Z',
    });
    expect(card.agent_id).toBe('ag-1');
    expect(card.capabilities).toHaveLength(1);
    expect(card.capabilities[0].input_schema_json).toBe('{}');
  });

  it('mapAuditEntry coerces numeric fields', () => {
    const row = mapAuditEntry({
      id: '1',
      invoke_id: 'inv',
      caller_agent_id: 'a',
      callee_agent_id: 'b',
      capability: 'cap',
      status: 'success',
      duration_ms: '42',
      workspace: 'ws',
      created_at: 't',
    });
    expect(row.duration_ms).toBe(42);
    expect(row.status).toBe('success');
  });
});
