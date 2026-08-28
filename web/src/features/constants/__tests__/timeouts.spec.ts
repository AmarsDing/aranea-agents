import { describe, expect, it } from 'vitest';
import {
  CHAT_FIRST_BYTE_NOTIFY_THRESHOLD_MS,
  firstByteNotifyThresholdMs,
} from '../timeouts';

describe('firstByteNotifyThresholdMs', () => {
  it('falls back to 30s when config is missing', () => {
    expect(firstByteNotifyThresholdMs()).toBe(CHAT_FIRST_BYTE_NOTIFY_THRESHOLD_MS);
    expect(firstByteNotifyThresholdMs('')).toBe(CHAT_FIRST_BYTE_NOTIFY_THRESHOLD_MS);
    expect(firstByteNotifyThresholdMs('{}')).toBe(CHAT_FIRST_BYTE_NOTIFY_THRESHOLD_MS);
  });

  it('reads first_byte_timeout_sec from the model pack', () => {
    expect(firstByteNotifyThresholdMs('{"first_byte_timeout_sec":75}')).toBe(75_000);
  });

  it('does not go below the 30s product default', () => {
    expect(firstByteNotifyThresholdMs('{"first_byte_timeout_sec":10}')).toBe(
      CHAT_FIRST_BYTE_NOTIFY_THRESHOLD_MS,
    );
  });
});
