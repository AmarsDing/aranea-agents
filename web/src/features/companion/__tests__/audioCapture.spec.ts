import { describe, expect, it } from 'vitest';

import { captureAudioConstraints, getMicStreamWithFallback } from '../voice/audioCapture';

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

// V11-S1（评审修复）：voiceIsolation 硬约束在「识别约束名但平台无法满足」的
// Chromium 构建上抛 OverconstrainedError——必须去 voiceIsolation 降级重试，
// 抗干扰增强不得导致语音模式整体不可用。
describe('getMicStreamWithFallback — voiceIsolation 降级重试', () => {
  const base = captureAudioConstraints();

  it('OverconstrainedError → 去掉 voiceIsolation 重试并返回流', async () => {
    const fakeStream = {} as MediaStream;
    const seen: Array<Record<string, unknown>> = [];
    const gum = async (audio: MediaTrackConstraints): Promise<MediaStream> => {
      seen.push(audio as Record<string, unknown>);
      if (seen.length === 1) throw new DOMException('unsatisfiable', 'OverconstrainedError');
      return fakeStream;
    };
    const stream = await getMicStreamWithFallback(base, gum);
    expect(stream).toBe(fakeStream);
    expect(seen).toHaveLength(2);
    expect(seen[0].voiceIsolation).toBe(true);
    expect(seen[1].voiceIsolation).toBeUndefined();
    // 降级仅移除 voiceIsolation，其余约束保留
    expect(seen[1].echoCancellation).toBe(true);
    expect(seen[1].noiseSuppression).toBe(true);
    expect(seen[1].autoGainControl).toBe(true);
    expect(seen[1].channelCount).toBe(1);
  });

  it('权限拒绝（NotAllowedError）不重试，原样上抛', async () => {
    let calls = 0;
    const gum = async (): Promise<MediaStream> => {
      calls++;
      throw new DOMException('denied', 'NotAllowedError');
    };
    await expect(getMicStreamWithFallback(base, gum)).rejects.toMatchObject({ name: 'NotAllowedError' });
    expect(calls).toBe(1);
  });

  it('约束本就不含 voiceIsolation 时 OverconstrainedError 不做无谓重试', async () => {
    let calls = 0;
    const gum = async (): Promise<MediaStream> => {
      calls++;
      throw new DOMException('unsatisfiable', 'OverconstrainedError');
    };
    const plain: MediaTrackConstraints = { channelCount: 1 };
    await expect(getMicStreamWithFallback(plain, gum)).rejects.toMatchObject({ name: 'OverconstrainedError' });
    expect(calls).toBe(1);
  });

  it('首次成功不重试，约束原样透传', async () => {
    const fakeStream = {} as MediaStream;
    const seen: Array<Record<string, unknown>> = [];
    const gum = async (audio: MediaTrackConstraints): Promise<MediaStream> => {
      seen.push(audio as Record<string, unknown>);
      return fakeStream;
    };
    const stream = await getMicStreamWithFallback(base, gum);
    expect(stream).toBe(fakeStream);
    expect(seen).toHaveLength(1);
    expect(seen[0].voiceIsolation).toBe(true);
  });
});
