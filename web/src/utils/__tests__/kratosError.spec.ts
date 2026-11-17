import { describe, expect, it } from 'vitest';
import axios from 'axios';
import { isForbiddenApiError, mapAgentCreateFieldErrors, parseKratosApiError } from '../kratosError';

function axiosError(status: number, data: Record<string, unknown>) {
  return new axios.AxiosError('Request failed', 'ERR_BAD_REQUEST', undefined, undefined, {
    status,
    statusText: 'Error',
    headers: {},
    config: { headers: new axios.AxiosHeaders() },
    data,
  });
}

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

describe('isForbiddenApiError', () => {
  it('detects axios 403', () => {
    expect(isForbiddenApiError(axiosError(403, { code: 403, reason: 'PERMISSION_DENIED', message: '无权限' }))).toBe(
      true,
    );
  });

  it('detects PERMISSION_DENIED reason without numeric status', () => {
    expect(isForbiddenApiError({ reason: 'PERMISSION_DENIED', message: '无权限' })).toBe(true);
  });

  it('does not flag a plain 500', () => {
    expect(isForbiddenApiError(axiosError(500, { code: 500, message: 'boom' }))).toBe(false);
  });

  it('detects nested cause', () => {
    const wrapped = new Error('加载失败', { cause: axiosError(403, { message: '无权限' }) });
    expect(isForbiddenApiError(wrapped)).toBe(true);
  });

  it('detects wrapped response envelope with numeric code only', () => {
    expect(isForbiddenApiError({ message: '加载失败', response: { data: { code: 403 } } })).toBe(true);
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
