import { describe, expect, it } from 'vitest';
import type { ChannelRow } from 'src/features/channels/types';
import { channelStatusBadgeColor, channelStatusBadgeText } from '../channelUi';

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

describe('channelStatusBadgeColor', () => {
  it('returns positive when runtime-connected even if persisted status is error (text/color consistency)', () => {
    // 回归：实时测试失败会把 DB status 写成 error，但运行时仍 connected，
    // 徽标文字显示 connected 时颜色必须同为 positive，不能红字 connected
    const r = row({ status: 'error', metadata_json: JSON.stringify({ runtime_connected: true }) });
    expect(channelStatusBadgeText(r)).toBe('connected');
    expect(channelStatusBadgeColor(r)).toBe('positive');
  });

  it('returns negative when not connected and status is error', () => {
    const r = row({ status: 'error' });
    expect(channelStatusBadgeColor(r)).toBe('negative');
  });

  it('returns grey when disabled', () => {
    const r = row({ enabled: false, status: 'error' });
    expect(channelStatusBadgeColor(r)).toBe('grey');
  });

  it('returns warning for pending_auth when not connected', () => {
    const r = row({ status: 'pending_auth', metadata_json: JSON.stringify({ last_error_message: 'x' }) });
    expect(channelStatusBadgeColor(r)).toBe('warning');
  });
});
