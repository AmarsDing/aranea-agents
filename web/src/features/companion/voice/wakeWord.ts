/**
 * 唤醒词检测（V10 设计 §16.5）：sherpa-onnx WASM KWS 本地封装。
 *
 * 资产经 `web/public/kws/` 分发（emcc 非 MODULARIZE loader + .data 内嵌 int8
 * 模型），经 <script> 注入加载一次、globalThis 缓存——重复进入语音模式不重复下载。
 * 音频不出设备（本地检出），加载失败 reject：调用方降级自动唤醒（设计 §16.1
 * 「退化为进入即聆听」），不阻塞语音模式。
 *
 * 关键词表（音素序列 @显示文本，modelingUnit=cjkchar）：
 * 「小媛」双音节误触发率高于四音节，叠词「小媛小媛」作兜底线（设计 §16.8）。
 * 阈值 0.25 为 spike 实测值（test/kws-spike/verify-kws.js）——0.7 全量漏检不可用。
 */

/** sherpa-onnx-kws.js 暴露的最小 Kws 接口（全局 createKws 的返回类型）。 */
type KwsStream = {
  acceptWaveform(sampleRate: number, samples: Float32Array): void;
  free(): void;
};

type KwsInstance = {
  createStream(): KwsStream;
  isReady(stream: KwsStream): boolean;
  decode(stream: KwsStream): void;
  reset(stream: KwsStream): void;
  getResult(stream: KwsStream): { keyword?: string };
  free(): void;
};

type EmscriptenModule = {
  locateFile(file: string): string;
  print(...args: unknown[]): void;
  printErr(...args: unknown[]): void;
  onRuntimeInitialized?: () => void;
};

export type WakeWordDetector = {
  /** 喂入 16kHz Float32 单声道帧；检出关键词触发 onDetect 并自动重置流。 */
  acceptWaveform(frame: Float32Array): void;
  /** 重置检测流（重新进入待命时清理残留状态）。 */
  reset(): void;
  /** 释放 Kws/Stream wasm 句柄（全局 Module 保留供下次复用）。 */
  dispose(): void;
};

export type WakeWordOptions = {
  /** 检出回调（唤醒词命中）。 */
  onDetect: () => void;
  /** 静态资产根路径（默认 '/kws/'）。 */
  baseUrl?: string;
  /** 检出阈值（默认 0.25，spike 验证值）。 */
  keywordsThreshold?: number;
  /** wasm 加载超时 ms（默认 30000）。 */
  loadTimeoutMs?: number;
  /** 测试注入：脚本加载器（默认 <script> 标签注入）。 */
  loadScript?: (src: string) => Promise<void>;
};

/** 关键词表：「小媛」+ 叠词兜底（设计 §16.1；模型配置字面值，非 UI 文案）。 */
const KWS_KEYWORDS = 'x iǎo y uán @小媛\nx iǎo y uán x iǎo y uán @小媛小媛'; // i18n-exempt
const DEFAULT_BASE_URL = '/kws/';
const DEFAULT_THRESHOLD = 0.25;
const DEFAULT_LOAD_TIMEOUT_MS = 30000;

type KwsGlobals = {
  module: EmscriptenModule;
  createKws: (module: EmscriptenModule, config: Record<string, unknown>) => KwsInstance;
};

/** 全局缓存：wasm 模块只加载一次（16MB 资产重复下载不可接受）。 */
let cachedGlobals: Promise<KwsGlobals> | null = null;

function defaultLoadScript(src: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const el = document.createElement('script');
    el.src = src;
    el.onload = () => resolve();
    el.onerror = () => reject(new Error('kws script load failed: ' + src));
    document.head.appendChild(el);
  });
}

function loadKwsGlobals(
  baseUrl: string,
  loadScript: (src: string) => Promise<void>,
  timeoutMs: number,
): Promise<KwsGlobals> {
  if (cachedGlobals) return cachedGlobals;
  const promise = (async (): Promise<KwsGlobals> => {
    const g = globalThis as unknown as Record<string, unknown>;
    // emcc 非 MODULARIZE 产物：脚本内 `var Module` 读全局对象——先挂配置再注入脚本。
    await new Promise<void>((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error('kws wasm load timeout')), timeoutMs);
      g.Module = {
        locateFile: (file: string) => baseUrl + file,
        print: () => undefined,
        printErr: () => undefined,
        onRuntimeInitialized: () => {
          clearTimeout(timer);
          resolve();
        },
      } satisfies EmscriptenModule;
      loadScript(baseUrl + 'sherpa-onnx-wasm-kws-main.js').catch((err: unknown) => {
        clearTimeout(timer);
        reject(err instanceof Error ? err : new Error(String(err)));
      });
    });
    await loadScript(baseUrl + 'sherpa-onnx-kws.js');
    if (typeof g.createKws !== 'function') {
      throw new Error('kws createKws missing after script load');
    }
    return {
      module: g.Module as EmscriptenModule,
      createKws: g.createKws as KwsGlobals['createKws'],
    };
  })();
  // 失败不固化缓存（允许重试）；成功则常驻。
  promise.catch(() => {
    if (cachedGlobals === promise) cachedGlobals = null;
  });
  cachedGlobals = promise;
  return promise;
}

/** 测试专用：清空模块缓存与全局注入（spec 间隔离）。 */
export function __resetWakeWordModuleCacheForTest(): void {
  cachedGlobals = null;
  const g = globalThis as unknown as Record<string, unknown>;
  delete g.Module;
  delete g.createKws;
}

export async function loadWakeWordDetector(opts: WakeWordOptions): Promise<WakeWordDetector> {
  const { module, createKws } = await loadKwsGlobals(
    opts.baseUrl ?? DEFAULT_BASE_URL,
    opts.loadScript ?? defaultLoadScript,
    opts.loadTimeoutMs ?? DEFAULT_LOAD_TIMEOUT_MS,
  );

  // 模型文件名为 .data 内嵌路径（int8 权重沿用标准文件名打包，spike 已验证）。
  const kws = createKws(module, {
    featConfig: { samplingRate: 16000, featureDim: 80 },
    modelConfig: {
      transducer: {
        encoder: './encoder-epoch-12-avg-2-chunk-16-left-64.onnx',
        decoder: './decoder-epoch-12-avg-2-chunk-16-left-64.onnx',
        joiner: './joiner-epoch-12-avg-2-chunk-16-left-64.onnx',
      },
      tokens: './tokens.txt',
      provider: 'cpu',
      modelType: '',
      numThreads: 1,
      debug: 0,
      modelingUnit: 'cjkchar',
      bpeVocab: '',
    },
    maxActivePaths: 4,
    numTrailingBlanks: 1,
    keywordsScore: 1.0,
    keywordsThreshold: opts.keywordsThreshold ?? DEFAULT_THRESHOLD,
    keywords: KWS_KEYWORDS,
  });
  const stream = kws.createStream();

  return {
    acceptWaveform(frame: Float32Array): void {
      stream.acceptWaveform(16000, frame);
      while (kws.isReady(stream)) {
        kws.decode(stream);
        const r = kws.getResult(stream);
        if (r && typeof r.keyword === 'string' && r.keyword.length > 0) {
          kws.reset(stream); // 检出即重置，防同一音频重复触发
          opts.onDetect();
        }
      }
    },
    reset(): void {
      kws.reset(stream);
    },
    dispose(): void {
      stream.free();
      kws.free();
    },
  };
}
