import { describe, expect, it } from 'vitest';
import axios from 'axios';
import { mapAgentCreateFieldErrors, parseKratosApiError } from '../kratosError';

describe('parseKratosApiError', () => {
  it('reads Kratos reason and message from axios 400', () => {
    const err = new axios.AxiosError('Bad Request', 'ERR_BAD_REQUEST', undefined, undefined, {
      status: 400,
      statusText: 'Bad Request',
      headers: {},
      config: { headers: new axios.AxiosHeaders() },
      data: { reason: 'AGENT_KEY_CONFLICT', message: 'agent_key already in use' },
    });
    const parsed = parseKratosApiError(err);
    expect(parsed.reason).toBe('AGENT_KEY_CONFLICT');
    expect(parsed.message).toBe('agent_key already in use');
    expect(parsed.status).toBe(400);
  });
});

describe('mapAgentCreateFieldErrors', () => {
  it('maps AGENT_KEY_CONFLICT to agent_key', () => {
    const fields = mapAgentCreateFieldErrors({
      reason: 'AGENT_KEY_CONFLICT',
      message: 'agent_key already in use',
    });
    expect(fields.agent_key).toBe('agent_key already in use');
  });

  it('maps AGENT reason with remote_url hint', () => {
    const fields = mapAgentCreateFieldErrors({
      reason: 'AGENT',
      message: 'a2a_proxy remote_url is required',
    });
    expect(fields.remote_url).toBe('a2a_proxy remote_url is required');
  });
});
