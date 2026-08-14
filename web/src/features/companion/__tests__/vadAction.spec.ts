import { describe, expect, it } from 'vitest';

import type { VoiceState } from '../types';
import { decideVadAction } from '../voice/vad';

// V11-T2（设计 §17.3）：barge_in 仅由 speech_barge_in（持续人声 ≥450ms）产生；
// speech_sustained（200ms onset）只武装判停计时，任何状态都不触发打断——
// 短促背景人声（咳嗽/旁人插话/电视对白）不再杀播报、不再取消在途 Turn。
describe('decideVadAction — VAD 事件 → 语音会话动作（V2-T1 / V11-T2）', () => {
  it('播报中持续人声 ≥450ms → barge_in（打断）', () => {
    expect(decideVadAction('speech_barge_in', 'speaking')).toBe('barge_in');
  });

  it('thinking 中持续人声 ≥450ms → barge_in（取消未产出的 Turn）', () => {
    expect(decideVadAction('speech_barge_in', 'thinking')).toBe('barge_in');
  });

  it.each<VoiceState>(['listening', 'idle', 'interrupted', 'error', 'dormant'])(
    'speech_barge_in 在 %s 态不触发动作（正常语句 / 非语音活动态）',
    (state) => {
      expect(decideVadAction('speech_barge_in', state)).toBeNull();
    },
  );

  it.each<VoiceState>(['listening', 'speaking', 'thinking', 'idle', 'interrupted', 'error', 'dormant'])(
    'speech_sustained（200ms onset）在 %s 态恒不触发动作（V11：仅武装判停计时）',
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
