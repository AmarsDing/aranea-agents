import { describe, expect, it } from 'vitest';

import type { VoiceState } from '../types';
import { decideVadAction } from '../voice/vad';

describe('decideVadAction — VAD 事件 → 语音会话动作（V2-T1）', () => {
  it('播报中检测到持续人声 → barge_in（打断）', () => {
    expect(decideVadAction('speech_sustained', 'speaking')).toBe('barge_in');
  });

  it('thinking 中检测到持续人声 → barge_in（取消未产出的 Turn）', () => {
    expect(decideVadAction('speech_sustained', 'thinking')).toBe('barge_in');
  });

  it.each<VoiceState>(['listening', 'idle', 'interrupted', 'error'])(
    'speech_sustained 在 %s 态不触发动作（正常语句 onset / 非语音活动态）',
    (state) => {
      expect(decideVadAction('speech_sustained', state)).toBeNull();
    },
  );

  it('listening 中静音超时 → commit（服务端端点失效时的判停兜底）', () => {
    expect(decideVadAction('silence_timeout', 'listening')).toBe('commit');
  });

  it.each<VoiceState>(['speaking', 'thinking', 'idle', 'interrupted', 'error'])(
    'silence_timeout 在 %s 态不发 commit（语句已由服务端端点或无需提交）',
    (state) => {
      expect(decideVadAction('silence_timeout', state)).toBeNull();
    },
  );

  it.each<VoiceState>(['listening', 'speaking'])('无 VAD 事件 → 恒无动作（%s）', (state) => {
    expect(decideVadAction(null, state)).toBeNull();
  });
});
