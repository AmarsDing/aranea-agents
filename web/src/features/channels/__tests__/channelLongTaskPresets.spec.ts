import { describe, expect, it } from 'vitest';
import { FEISHU_IM_PREVIEW_DEFAULTS } from '../channelImPreviewDefaults';
import { inferLongTaskPresetId } from '../channelLongTaskPresets';

describe('inferLongTaskPresetId', () => {
  it('matches feishu IM preview preset', () => {
    expect(inferLongTaskPresetId('websocket', { ...FEISHU_IM_PREVIEW_DEFAULTS })).toBe('feishu_im_preview');
  });

  it('returns empty when config diverges from presets', () => {
    expect(
      inferLongTaskPresetId('websocket', {
        ...FEISHU_IM_PREVIEW_DEFAULTS,
        im_render_mode: 'reply_only',
      }),
    ).toBe('');
  });

  it('respects receive_mode constraint', () => {
    expect(inferLongTaskPresetId('webhook', { ...FEISHU_IM_PREVIEW_DEFAULTS })).toBe('');
  });
});
