/**
 * useVoiceDictation（聊天页听写）单测。
 *
 * 协议客户端 / 麦克风采集均经工厂注入，验证：
 * - toggle 启动：connect + startVoice(mode=dictation) + 采集启动
 * - asr.partial → partial 实时字幕；asr.final → onFinalText 回调（连续听写）
 * - 再次 toggle / voice.error / 通道关闭 → 资源回收 + 错误上报
 */

import { describe, expect, it } from 'vitest';

import type { VoiceError } from '../../companion/types';
import type { AudioCapture, AudioCaptureOptions } from '../../companion/voice/audioCapture';
import type {
  VoiceSessionClient,
  VoiceSessionClientOptions,
  VoiceStartParams,
} from '../../companion/voice/useVoiceSession';
import {
  joinDictationText,
  useVoiceDictation,
  VOICE_MODE_DICTATION,
  type VoiceDictationDeps,
} from '../useVoiceDictation';

class FakeClient {
  opts: VoiceSessionClientOptions;
  connectCalls = 0;
  disconnectCalls = 0;
  startParams: (VoiceStartParams | undefined)[] = [];
  stopCalls = 0;
  audio: ArrayBuffer[] = [];

  constructor(opts: VoiceSessionClientOptions) {
    this.opts = opts;
  }
  connect(): void {
    this.connectCalls++;
  }
  disconnect(): void {
    this.disconnectCalls++;
  }
  startVoice(p?: VoiceStartParams): void {
    this.startParams.push(p);
  }
  stopVoice(): void {
    this.stopCalls++;
  }
  commit(): void {}
  bargeIn(): void {}
  cancel(): void {}
  sendAudio(f: ArrayBuffer): void {
    this.audio.push(f);
  }
  get connected(): boolean {
    return this.connectCalls > 0 && this.disconnectCalls === 0;
  }
}

class FakeCapture {
  opts: AudioCaptureOptions;
  started = false;
  stopped = false;
  failStart = false;

  constructor(opts: AudioCaptureOptions) {
    this.opts = opts;
  }
  async start(): Promise<void> {
    if (this.failStart) throw new Error('denied');
    this.started = true;
  }
  stop(): void {
    this.stopped = true;
  }
  get running(): boolean {
    return this.started && !this.stopped;
  }
  get analyser(): AnalyserNode | null {
    return null;
  }
}

function makeDeps(overrides: Partial<VoiceDictationDeps> & { failCapture?: boolean } = {}) {
  const { failCapture, ...rest } = overrides;
  const clients: FakeClient[] = [];
  const captures: FakeCapture[] = [];
  const finals: string[] = [];
  const errors: VoiceError[] = [];
  const deps: VoiceDictationDeps = {
    sessionId: () => 'sess-1',
    onFinalText: (t) => finals.push(t),
    onError: (e) => errors.push(e),
    clientFactory: (opts) => {
      const c = new FakeClient(opts);
      clients.push(c);
      return c as unknown as VoiceSessionClient;
    },
    captureFactory: (opts) => {
      const c = new FakeCapture(opts);
      c.failStart = failCapture === true;
      captures.push(c);
      return c as unknown as AudioCapture;
    },
    ...rest,
  };
  return { deps, clients, captures, finals, errors };
}

describe('useVoiceDictation — 启动/停止', () => {
  it('toggle 启动听写：connect + startVoice(mode=dictation) + 采集启动', async () => {
    const { deps, clients, captures } = makeDeps();
    const d = useVoiceDictation(deps);
    expect(d.dictating.value).toBe(false);

    await d.toggle();

    expect(clients.length).toBe(1);
    expect(clients[0].connectCalls).toBe(1);
    expect(clients[0].startParams).toEqual([
      { sampleRate: 16000, mode: VOICE_MODE_DICTATION },
    ]);
    expect(captures[0].started).toBe(true);
    expect(d.dictating.value).toBe(true);
  });

  it('采集帧经 client.sendAudio 上行', async () => {
    const { deps, clients, captures } = makeDeps();
    const d = useVoiceDictation(deps);
    await d.toggle();

    const frame = new ArrayBuffer(640);
    captures[0].opts.onVoiceFrame(frame);
    expect(clients[0].audio).toEqual([frame]);
  });

  it('再次 toggle 停止：stopVoice + disconnect + 采集停止，状态复位', async () => {
    const { deps, clients, captures } = makeDeps();
    const d = useVoiceDictation(deps);
    await d.toggle();
    d.partial.value = '识别中';

    await d.toggle();

    expect(clients[0].stopCalls).toBe(1);
    expect(clients[0].disconnectCalls).toBe(1);
    expect(captures[0].stopped).toBe(true);
    expect(d.dictating.value).toBe(false);
    expect(d.partial.value).toBe('');
  });

  it('无会话 ID：拒绝启动并上报 NO_SESSION', async () => {
    const { deps, clients, errors } = makeDeps({ sessionId: () => null });
    const d = useVoiceDictation(deps);
    await d.toggle();

    expect(clients.length).toBe(0);
    expect(errors.map((e) => e.code)).toEqual(['NO_SESSION']);
    expect(d.dictating.value).toBe(false);
  });

  it('麦克风拒绝：上报 MIC_UNAVAILABLE，资源回收', async () => {
    const { deps, clients, errors } = makeDeps({ failCapture: true });
    const d = useVoiceDictation(deps);

    await d.toggle();

    expect(errors.map((e) => e.code)).toEqual(['MIC_UNAVAILABLE']);
    expect(d.dictating.value).toBe(false);
    expect(clients[0].disconnectCalls).toBe(1);
  });
});

describe('useVoiceDictation — 识别事件', () => {
  it('asr.partial 更新实时字幕；asr.final 回调 onFinalText 并清空字幕', async () => {
    const { deps, clients, finals } = makeDeps();
    const d = useVoiceDictation(deps);
    await d.toggle();

    clients[0].opts.handlers.onPartial('你好');
    expect(d.partial.value).toBe('你好');

    clients[0].opts.handlers.onFinal('你好世界', 800);
    expect(finals).toEqual(['你好世界']);
    expect(d.partial.value).toBe('');
  });

  it('连续听写：多个终稿依次回调，dictating 保持 true', async () => {
    const { deps, clients, finals } = makeDeps();
    const d = useVoiceDictation(deps);
    await d.toggle();

    clients[0].opts.handlers.onFinal('第一句', 500);
    clients[0].opts.handlers.onFinal('第二句', 600);
    expect(finals).toEqual(['第一句', '第二句']);
    expect(d.dictating.value).toBe(true);
  });

  it('空终稿不回调 onFinalText', async () => {
    const { deps, clients, finals } = makeDeps();
    const d = useVoiceDictation(deps);
    await d.toggle();

    clients[0].opts.handlers.onFinal('', 100);
    expect(finals).toEqual([]);
  });

  it('服务端 voice.error：停止听写并透传错误', async () => {
    const { deps, clients, errors } = makeDeps();
    const d = useVoiceDictation(deps);
    await d.toggle();

    clients[0].opts.handlers.onVoiceError({ code: 'ASR_UNAVAILABLE', message: 'x', retryable: true });
    expect(errors.map((e) => e.code)).toEqual(['ASR_UNAVAILABLE']);
    expect(d.dictating.value).toBe(false);
    expect(clients[0].disconnectCalls).toBe(1);
  });

  it('通道在听写中关闭：停止并上报 VOICE_CHANNEL_CLOSED', async () => {
    const { deps, clients, errors } = makeDeps();
    const d = useVoiceDictation(deps);
    await d.toggle();

    clients[0].opts.handlers.onClose?.();
    expect(errors.map((e) => e.code)).toEqual(['VOICE_CHANNEL_CLOSED']);
    expect(d.dictating.value).toBe(false);
  });

  it('停止后的迟到 onClose 不再重复报错', async () => {
    const { deps, clients, errors } = makeDeps();
    const d = useVoiceDictation(deps);
    await d.toggle();
    await d.toggle(); // 停止

    clients[0].opts.handlers.onClose?.();
    expect(errors).toEqual([]);
  });

  it('voice.replaced（同会话第二连接替换）：本地停止且不报错', async () => {
    const { deps, clients, errors } = makeDeps();
    const d = useVoiceDictation(deps);
    await d.toggle();

    clients[0].opts.handlers.onReplaced();
    expect(d.dictating.value).toBe(false);
    expect(errors).toEqual([]);
  });
});

describe('joinDictationText', () => {
  it('空输入框直接填入', () => {
    expect(joinDictationText('', '你好')).toBe('你好');
  });

  it('中文/标点结尾直接拼接（不加空格）', () => {
    expect(joinDictationText('你好', '世界')).toBe('你好世界');
    expect(joinDictationText('你好，', '世界')).toBe('你好，世界');
  });

  it('英文/数字结尾且新增以英文/数字开头时补空格', () => {
    expect(joinDictationText('hello', 'world')).toBe('hello world');
    expect(joinDictationText('abc123', 'def')).toBe('abc123 def');
  });

  it('英文结尾 + 中文开头不补空格', () => {
    expect(joinDictationText('hello', '世界')).toBe('hello世界');
  });

  it('新增为空时返回原文', () => {
    expect(joinDictationText('你好', '')).toBe('你好');
  });
});
