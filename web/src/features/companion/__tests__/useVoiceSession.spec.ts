import { describe, expect, it } from 'vitest';
import { createVoiceSessionClient, type VoiceDownstreamHandlers, type VoiceState } from '../voice/useVoiceSession';

class FakeWebSocket {
  static OPEN = 1;
  readyState = 0; // CONNECTING
  sent: (string | ArrayBuffer)[] = [];
  closed = false;
  onopen: (() => void) | null = null;
  onmessage: ((ev: { data: unknown }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: ((e: unknown) => void) | null = null;

  send(data: string | ArrayBuffer): void {
    this.sent.push(data);
  }
  close(): void {
    this.closed = true;
    this.readyState = 3;
    this.onclose?.();
  }
  // 测试辅助
  open(): void {
    this.readyState = 1;
    this.onopen?.();
  }
  receiveJson(msg: Record<string, unknown>): void {
    this.onmessage?.({ data: JSON.stringify(msg) });
  }
  receiveBinary(buf: ArrayBuffer): void {
    this.onmessage?.({ data: buf });
  }
}

function makeClient(handlers: Partial<VoiceDownstreamHandlers> = {}) {
  const sockets: FakeWebSocket[] = [];
  const client = createVoiceSessionClient({
    url: 'ws://test/v1/voice?session_id=s1',
    socketFactory: () => {
      const s = new FakeWebSocket();
      sockets.push(s);
      return s as unknown as WebSocket;
    },
    handlers: {
      onState: () => undefined,
      onPartial: () => undefined,
      onFinal: () => undefined,
      onTurnAccepted: () => undefined,
      onTtsStart: () => undefined,
      onTtsAudio: () => undefined,
      onTtsEnd: () => undefined,
      onVoiceError: () => undefined,
      onReplaced: () => undefined,
      ...handlers,
    },
  });
  return { client, sockets };
}

describe('createVoiceSessionClient — 上行', () => {
  it('queues control frames before open and flushes on open', () => {
    const { client, sockets } = makeClient();
    client.connect();
    client.startVoice({ language: 'zh-CN', dialogMode: 'chat', agentKey: 'jarvis' });
    expect(sockets[0].sent.length).toBe(0); // 未 open 前不发
    sockets[0].open();
    expect(sockets[0].sent.length).toBe(1);
    const msg = JSON.parse(sockets[0].sent[0] as string);
    expect(msg).toEqual({
      type: 'voice.start',
      sample_rate: 16000,
      language: 'zh-CN',
      dialog_mode: 'chat',
      agent_key: 'jarvis',
      team_id: '',
      mode: '',
    });
  });

  it('startVoice carries mode=dictation for chat composer dictation', () => {
    const { client, sockets } = makeClient();
    client.connect();
    client.startVoice({ mode: 'dictation' });
    sockets[0].open();
    const msg = JSON.parse(sockets[0].sent[0] as string);
    expect(msg).toEqual({
      type: 'voice.start',
      sample_rate: 16000,
      language: '',
      dialog_mode: '',
      agent_key: '',
      team_id: '',
      mode: 'dictation',
    });
  });

  it('sends stop/commit/barge_in/cancel as JSON control frames', () => {
    const { client, sockets } = makeClient();
    client.connect();
    sockets[0].open();
    client.commit();
    client.bargeIn(230);
    client.cancel();
    client.stopVoice();
    const types = sockets[0].sent.map((raw) => JSON.parse(raw as string));
    expect(types).toEqual([
      { type: 'voice.commit' },
      { type: 'voice.barge_in', detect_ms: 230 },
      { type: 'voice.cancel' },
      { type: 'voice.stop' },
    ]);
  });

  it('sends audio frames as binary only when open', () => {
    const { client, sockets } = makeClient();
    client.connect();
    const frame = new ArrayBuffer(640);
    client.sendAudio(frame); // 未 open：实时音频丢弃不排队
    sockets[0].open();
    client.sendAudio(frame);
    expect(sockets[0].sent.length).toBe(1);
    expect(sockets[0].sent[0]).toBe(frame);
  });

  it('disconnect closes the socket and suppresses reconnect-less reuse', () => {
    const { client, sockets } = makeClient();
    client.connect();
    sockets[0].open();
    expect(client.connected).toBe(true);
    client.disconnect();
    expect(sockets[0].closed).toBe(true);
    expect(client.connected).toBe(false);
  });
});

describe('createVoiceSessionClient — 下行分发', () => {
  it('dispatches voice.state with known states only', () => {
    const states: VoiceState[] = [];
    const { client, sockets } = makeClient({ onState: (s) => states.push(s) });
    client.connect();
    sockets[0].open();
    sockets[0].receiveJson({ type: 'voice.state', state: 'listening' });
    sockets[0].receiveJson({ type: 'voice.state', state: 'speaking' });
    sockets[0].receiveJson({ type: 'voice.state', state: 'bogus' });
    expect(states).toEqual(['listening', 'speaking']);
  });

  it('dispatches asr.partial / asr.final / turn.accepted', () => {
    const partials: string[] = [];
    const finals: { text: string; durationMs: number | undefined }[] = [];
    const turns: string[] = [];
    const { client, sockets } = makeClient({
      onPartial: (t) => partials.push(t),
      onFinal: (text, durationMs) => finals.push({ text, durationMs }),
      onTurnAccepted: (id) => turns.push(id),
    });
    client.connect();
    sockets[0].open();
    sockets[0].receiveJson({ type: 'asr.partial', text: '你好' });
    sockets[0].receiveJson({ type: 'asr.final', text: '你好世界', duration_ms: 1200 });
    sockets[0].receiveJson({ type: 'turn.accepted', turn_id: 'vt-1' });
    expect(partials).toEqual(['你好']);
    expect(finals).toEqual([{ text: '你好世界', durationMs: 1200 }]);
    expect(turns).toEqual(['vt-1']);
  });

  it('routes binary frames to onTtsAudio and tts.start/end to handlers', () => {
    const audio: ArrayBuffer[] = [];
    const starts: { encoding: string; sampleRate: number }[] = [];
    const ends: boolean[] = [];
    const { client, sockets } = makeClient({
      onTtsAudio: (pcm) => audio.push(pcm),
      onTtsStart: (info) => starts.push(info),
      onTtsEnd: (interrupted) => ends.push(interrupted),
    });
    client.connect();
    sockets[0].open();
    sockets[0].receiveJson({ type: 'tts.start', encoding: 'pcm_f32le_16k', sample_rate: 16000 });
    const chunk = new ArrayBuffer(128);
    sockets[0].receiveBinary(chunk);
    sockets[0].receiveJson({ type: 'tts.end' });
    sockets[0].receiveJson({ type: 'tts.end', interrupted: true });
    expect(starts).toEqual([{ encoding: 'pcm_f32le_16k', sampleRate: 16000 }]);
    expect(audio).toEqual([chunk]);
    expect(ends).toEqual([false, true]);
  });

  it('dispatches voice.error / voice.replaced / ignores pong and malformed frames', () => {
    const errors: { code: string; retryable: boolean }[] = [];
    let replaced = 0;
    const { client, sockets } = makeClient({
      onVoiceError: (e) => errors.push({ code: e.code, retryable: e.retryable }),
      onReplaced: () => replaced++,
    });
    client.connect();
    sockets[0].open();
    sockets[0].receiveJson({ type: 'voice.error', code: 'ASR_UNAVAILABLE', message: 'x', retryable: true });
    sockets[0].receiveJson({ type: 'voice.replaced' });
    sockets[0].receiveJson({ type: 'pong' });
    sockets[0].onmessage?.({ data: 'not-json{{' });
    expect(errors).toEqual([{ code: 'ASR_UNAVAILABLE', retryable: true }]);
    expect(replaced).toBe(1);
  });
});
