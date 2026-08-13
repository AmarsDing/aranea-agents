import { afterEach, describe, expect, it } from 'vitest';

import { __resetWakeWordModuleCacheForTest, loadWakeWordDetector, type WakeWordOptions } from '../voice/wakeWord';

/**
 * KWS 封装单测（V10 设计 §16.5/§16.7）：
 * emcc 运行时与 sherpa-onnx-kws.js 经注入的 loadScript 仿真——
 * main.js 加载后异步触发 Module.onRuntimeInitialized，kws.js 挂载全局 createKws。
 */

type FakeResult = { keyword: string };

function makeFakeKws(results: FakeResult[]) {
  const state = { resetCount: 0, kwsFreed: false, streamFreed: false, fedFrames: 0 };
  const stream = {
    acceptWaveform: (_rate: number, samples: Float32Array) => {
      state.fedFrames += samples.length;
    },
    free: () => {
      state.streamFreed = true;
    },
  };
  const kws = {
    createStream: () => stream,
    isReady: () => results.length > 0,
    decode: () => undefined,
    reset: () => {
      state.resetCount++;
    },
    getResult: () => results.shift() ?? { keyword: '' },
    free: () => {
      state.kwsFreed = true;
    },
  };
  return { kws, state };
}

/** 仿真脚本加载器：main.js → 异步初始化运行时；kws.js → 挂全局 createKws。 */
function makeScriptLoader(results: FakeResult[], opts: { initRuntime?: boolean } = {}) {
  const loaded: string[] = [];
  const fake = makeFakeKws(results);
  const loadScript = (src: string): Promise<void> => {
    loaded.push(src);
    const g = globalThis as Record<string, unknown>;
    if (src.endsWith('sherpa-onnx-wasm-kws-main.js')) {
      if (opts.initRuntime !== false) {
        const module = g.Module as { onRuntimeInitialized?: () => void };
        setTimeout(() => module.onRuntimeInitialized?.(), 0);
      }
      return Promise.resolve();
    }
    if (src.endsWith('sherpa-onnx-kws.js')) {
      g.createKws = () => fake.kws;
      return Promise.resolve();
    }
    return Promise.reject(new Error('unexpected src: ' + src));
  };
  return { loadScript, loaded, fake };
}

function load(overrides: Partial<WakeWordOptions>, loadScript: (src: string) => Promise<void>) {
  return loadWakeWordDetector({ onDetect: () => undefined, loadScript, ...overrides });
}

afterEach(() => {
  __resetWakeWordModuleCacheForTest();
});

describe('loadWakeWordDetector — 加载', () => {
  it('加载成功：按序加载 main.js + kws.js，检出配置含「小媛」关键词与 0.25 阈值', async () => {
    const { loadScript, loaded, fake } = makeScriptLoader([]);
    let seenConfig: Record<string, unknown> | null = null;
    const g = globalThis as Record<string, unknown>;
    const wrapped = (src: string) =>
      loadScript(src).then(() => {
        if (src.endsWith('sherpa-onnx-kws.js')) {
          const inner = g.createKws as (m: unknown, c: Record<string, unknown>) => unknown;
          g.createKws = (m: unknown, c: Record<string, unknown>) => {
            seenConfig = c;
            return inner(m, c);
          };
        }
      });
    await load({}, wrapped);
    expect(loaded).toEqual([
      '/kws/sherpa-onnx-wasm-kws-main.js',
      '/kws/sherpa-onnx-kws.js',
    ]);
    expect(fake.state).toBeDefined();
    const cfg = seenConfig as unknown as { keywords: string; keywordsThreshold: number };
    expect(cfg.keywords).toContain('@小媛');
    expect(cfg.keywordsThreshold).toBe(0.25); // spike 实测：0.7 全量漏检，必须 0.25
  });

  it('加载失败（脚本错误）→ reject，且允许后续重试（缓存不固化失败）', async () => {
    const failing = () => Promise.reject(new Error('net error'));
    await expect(load({}, failing)).rejects.toThrow('net error');

    const { loadScript } = makeScriptLoader([]);
    await expect(load({}, loadScript)).resolves.toBeDefined();
  });

  it('加载超时（运行时未初始化）→ reject', async () => {
    const { loadScript } = makeScriptLoader([], { initRuntime: false });
    await expect(load({ loadTimeoutMs: 20 }, loadScript)).rejects.toThrow(/timeout/);
  });
});

describe('loadWakeWordDetector — 检出与生命周期', () => {
  it('检出关键词 → 触发 onDetect 并重置流（防同一音频重复检出）', async () => {
    const { loadScript, fake } = makeScriptLoader([{ keyword: '小媛' }]);
    let detected = 0;
    const detector = await load({ onDetect: () => detected++ }, loadScript);
    detector.acceptWaveform(new Float32Array(320));
    expect(detected).toBe(1);
    expect(fake.state.resetCount).toBe(1);
  });

  it('无命中（空 keyword）→ 不触发 onDetect、不重置', async () => {
    const { loadScript, fake } = makeScriptLoader([{ keyword: '' }]);
    let detected = 0;
    const detector = await load({ onDetect: () => detected++ }, loadScript);
    detector.acceptWaveform(new Float32Array(320));
    expect(detected).toBe(0);
    expect(fake.state.resetCount).toBe(0);
  });

  it('dispose 释放 stream 与 kws 句柄', async () => {
    const { loadScript, fake } = makeScriptLoader([]);
    const detector = await load({}, loadScript);
    detector.dispose();
    expect(fake.state.streamFreed).toBe(true);
    expect(fake.state.kwsFreed).toBe(true);
  });

  it('全局缓存：重复 load 不重复加载脚本（wasm 仅下载一次）', async () => {
    const { loadScript, loaded } = makeScriptLoader([]);
    const d1 = await load({}, loadScript);
    const d2 = await load({}, loadScript);
    expect(d1).not.toBe(d2);
    expect(loaded.length).toBe(2); // main.js + kws.js 各一次
    d1.dispose();
    d2.dispose();
  });
});
