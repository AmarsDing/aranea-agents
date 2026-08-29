import { describe, expect, it } from 'vitest';
import type { ChannelRow } from 'src/features/channels/types';
import { channelStatusBadgeText } from '../channelUi';

function row(overrides: Partial<ChannelRow>): ChannelRow {
  return {
    id: 'c1',
    name: 'ch',
    key: 'ch-1',
    config_json: JSON.stringify({ type: 'wechat_ilink', receive_mode: 'polling' }),
    metadata_json: '',
    status: 'active',
    enabled: true,
    ...overrides,
  } as ChannelRow;
}

describe('channelStatusBadgeText', () => {
  it('returns connected when runtime-connected even if persisted status is error', () => {
    // 回归：实时测试失败会把 DB status 写成 error，但运行时仍 connected，
    // 状态芯片的 status 必须显示 connected（色调由 appStatusMeta 同源保证）
    const r = row({ status: 'error', metadata_json: JSON.stringify({ runtime_connected: true }) });
    expect(channelStatusBadgeText(r)).toBe('connected');
  });

  it('returns persisted status when not connected', () => {
    const r = row({ status: 'error' });
    expect(channelStatusBadgeText(r)).toBe('error');
  });

  it('returns disabled when not enabled', () => {
    const r = row({ enabled: false, status: 'error' });
    expect(channelStatusBadgeText(r)).toBe('disabled');
  });

  it('returns pending_auth when not connected', () => {
    const r = row({ status: 'pending_auth', metadata_json: JSON.stringify({ last_error_message: 'x' }) });
    expect(channelStatusBadgeText(r)).toBe('pending_auth');
  });
});
