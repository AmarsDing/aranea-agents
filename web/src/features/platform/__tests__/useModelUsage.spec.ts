import { describe, expect, it, vi } from 'vitest';
import { ref } from 'vue';
import type { PlatformResource } from '../types';

vi.mock('../../agents/api', () => ({
  listAgentsPaged: vi.fn(async () => ({
    items: [
      { id: 'a1', agent_key: 'x', display_name: 'X', provider: 'deepseek', model: 'deepseek-chat' },
      { id: 'a2', agent_key: 'y', display_name: 'Y', provider: 'DeepSeek', model: 'deepseek-chat' },
      { id: 'a3', agent_key: 'z', display_name: 'Z', provider: 'openrouter', model: 'gpt-4.1' },
    ],
    total: 3,
    limit: 1000,
    offset: 0,
  })),
}));

import { useModelUsage } from '../useModelUsage';
import { channelHealth } from '../../channels/useChannelsPage';
import type { ChannelRow } from '../../channels/types';

const model = (provider: string, name: string, enabled = true) =>
  ({ id: `${provider}/${name}`, provider, model: name, enabled }) as unknown as PlatformResource;

describe('useModelUsage', () => {
  it('counts users per model (case-insensitive) and lists unregistered models by reference count', async () => {
    const rows = ref([model('openrouter', 'gpt-4.1', false)]);
    const usage = useModelUsage(rows);
    await usage.load();
    expect(usage.agentsUsing(rows.value[0]!).map((a) => a.id)).toEqual(['a3']);
    expect(usage.missing.value.map((m) => [m.provider, m.model, m.agents.length])).toEqual([
      ['deepseek', 'deepseek-chat', 2],
    ]);
    expect(usage.onDisabled.value).toBe(1);
  });
});

describe('channelHealth', () => {
  const row = (patch: Partial<ChannelRow>) =>
    ({ enabled: true, status: 'active', metadata_json: '{}', config_json: '{}', ...patch }) as ChannelRow;
  it('buckets channels into the status strip', () => {
    expect(channelHealth(row({ enabled: false }))).toBe('off');
    expect(channelHealth(row({}))).toBe('online');
    expect(channelHealth(row({ status: 'error' }))).toBe('alert');
    expect(channelHealth(row({ metadata_json: '{"last_error_message":"boom"}' }))).toBe('alert');
    expect(channelHealth(row({ status: 'pending_auth' }))).toBe('pending');
  });
});
