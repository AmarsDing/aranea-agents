import { describe, it, expect } from 'vitest';
import {
  normalizeAgentFromService,
  normalizeRuntimeSettingsFromWire,
  normalizePromptFileFromWire,
} from '../wireNormalize';

describe('normalizeRuntimeSettingsFromWire', () => {
  it('returns undefined for null/undefined input', () => {
    expect(normalizeRuntimeSettingsFromWire(null)).toBeUndefined();
    expect(normalizeRuntimeSettingsFromWire(undefined)).toBeUndefined();
  });

  it('returns undefined for empty object', () => {
    expect(normalizeRuntimeSettingsFromWire({})).toBeUndefined();
  });

  it('picks camelCase wire fields correctly', () => {
    const settings = normalizeRuntimeSettingsFromWire({
      memoryEnabled: true,
      memoryMaxResults: 10,
      toolsEnabled: false,
      toolsProfile: 'all',
    });
    expect(settings?.memory_enabled).toBe(true);
    expect(settings?.memory_max_results).toBe(10);
    expect(settings?.tools_enabled).toBe(false);
    expect(settings?.tools_profile).toBe('all');
  });

  it('picks snake_case wire fields as fallback', () => {
    const settings = normalizeRuntimeSettingsFromWire({
      memory_enabled: false,
      tools_enabled: true,
      memory_min_score: 0.5,
    });
    expect(settings?.memory_enabled).toBe(false);
    expect(settings?.tools_enabled).toBe(true);
    expect(settings?.memory_min_score).toBe(0.5);
  });

  it('applies numeric defaults', () => {
    const settings = normalizeRuntimeSettingsFromWire({ agentId: 'a1' });
    expect(settings?.subagents_max_concurrency).toBe(20);
    expect(settings?.memory_max_chunk_length).toBe(1000);
    expect(settings?.heartbeat_interval_minutes).toBe(30);
  });

  it('applies boolean defaults', () => {
    const settings = normalizeRuntimeSettingsFromWire({ agentId: 'a1' });
    expect(settings?.self_evolve).toBe(true);
    expect(settings?.memory_enabled).toBe(true);
    expect(settings?.heartbeat_enabled).toBe(false);
    expect(settings?.intent_pass_enabled).toBe(true);
    expect(settings?.tools_profile).toBe('coding');
  });
});

describe('normalizePromptFileFromWire', () => {
  it('maps camelCase fields', () => {
    const file = normalizePromptFileFromWire({
      id: 'f1',
      agentId: 'a1',
      name: 'system.md',
      body: 'You are...',
      sortOrder: 1,
      createdAt: '2026-01-01',
      updatedAt: '2026-01-02',
    });
    expect(file.id).toBe('f1');
    expect(file.agent_id).toBe('a1');
    expect(file.name).toBe('system.md');
    expect(file.body).toBe('You are...');
    expect(file.sort_order).toBe(1);
  });

  it('maps snake_case fields as fallback', () => {
    const file = normalizePromptFileFromWire({
      id: 'f2',
      agent_id: 'a2',
      name: 'prompt.md',
      body: 'Hello',
      sort_order: 0,
    });
    expect(file.agent_id).toBe('a2');
    expect(file.sort_order).toBe(0);
  });
});

describe('normalizeAgentFromService', () => {
  it('normalizes a complete agent wire response', () => {
    const agent = normalizeAgentFromService({
      id: 'ag-1',
      agentKey: 'my-agent',
      displayName: 'My Agent',
      provider: 'openai',
      model: 'gpt-4o',
      status: 'active',
      isDefault: false,
      isFavorite: true,
      settings: { memoryEnabled: true, toolsEnabled: true },
    });
    expect(agent.id).toBe('ag-1');
    expect(agent.agent_key).toBe('my-agent');
    expect(agent.display_name).toBe('My Agent');
    expect(agent.is_favorite).toBe(true);
    expect(agent.settings?.memory_enabled).toBe(true);
  });

  it('normalizes agent with no settings', () => {
    const agent = normalizeAgentFromService({ id: 'ag-2', agentKey: 'bare' });
    expect(agent.id).toBe('ag-2');
    expect(agent.settings).toBeUndefined();
  });

  it('normalizes agent with files array', () => {
    const agent = normalizeAgentFromService({
      id: 'ag-3',
      agentKey: 'with-files',
      files: [{ id: 'f1', agentId: 'ag-3', name: 'sys.md', body: 'sys', sortOrder: 0 }],
    });
    expect(agent.files).toHaveLength(1);
    expect(agent.files?.[0].name).toBe('sys.md');
  });

  it('defaults status to active when missing', () => {
    const agent = normalizeAgentFromService({ id: 'ag-4', agentKey: 'x' });
    expect(agent.status).toBe('active');
  });

  it('maps a2a_endpoint_enabled', () => {
    const agent = normalizeAgentFromService({ id: 'ag-5', agentKey: 'ep', a2aEndpointEnabled: true });
    expect(agent.a2a_endpoint_enabled).toBe(true);
  });
});
