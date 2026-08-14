import { describe, expect, it } from 'vitest';

import { captureAudioConstraints } from '../voice/audioCapture';

// V11-T1（设计 §17.2）：采集层抗干扰约束——ML 人声隔离 + 自动增益 +
// 回声消除 + 降噪。不支持的浏览器按 WebIDL 静默忽略未知约束（基础字典
// 非 exact 约束不抛 OverconstrainedError），零风险降级。
describe('captureAudioConstraints — V11 抗干扰采集约束', () => {
  it('开启 AGC + voiceIsolation（压制背景人声/电平漂移）', () => {
    const c = captureAudioConstraints() as Record<string, unknown>;
    expect(c.autoGainControl).toBe(true);
    expect(c.voiceIsolation).toBe(true);
  });

  it('保留既有 AEC + NS + 单声道（回归）', () => {
    const c = captureAudioConstraints() as Record<string, unknown>;
    expect(c.echoCancellation).toBe(true);
    expect(c.noiseSuppression).toBe(true);
    expect(c.channelCount).toBe(1);
  });
});
