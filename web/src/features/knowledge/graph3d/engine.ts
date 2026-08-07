/**
 * engine：G5 物理执行层装配（纯 TS，零 Vue/three 依赖）。
 *
 * - Worker 优先：Vite `?worker` 创建 physics.worker，init 消息 slice 后 transfer
 * - 主线程兜底（NFR-G5-5）：Worker 创建失败 → RAF 跑同一 ForceEngine
 * - 收敛懒惰：alpha<alphaMin → settled，停止驱动；reheat/pin 唤醒
 * - stepFrame 可测试直接驱动（不依赖 RAF 时序）
 */
import { FORCE_DEFAULTS, ForceEngine, type ForceParams } from './forces';
import type { GraphModel } from './model';
import type { InMessage, OutMessage } from './protocol';
import PhysicsWorker from './physics.worker?worker';

/** 可注入的 Worker 抽象（测试用 FakeWorker 替换真实 Worker）。 */
export interface WorkerLike {
  postMessage(msg: InMessage, transfer?: Transferable[]): void;
  addEventListener(type: 'message', cb: (ev: MessageEvent) => void): void;
  removeEventListener(type: 'message', cb: (ev: MessageEvent) => void): void;
  terminate(): void;
}

export interface GraphEngineCallbacks {
  onTick?: (positions: Float32Array, alpha: number) => void;
  onSettled?: () => void;
  onError?: (message: string) => void;
}

export interface GraphEngineDeps {
  workerFactory?: () => WorkerLike;
  /** 覆盖默认力参数（测试/调参）。 */
  params?: Partial<ForceParams>;
  chargeScale?: Float32Array;
}

export class GraphEngine {
  private readonly model: GraphModel;
  private readonly callbacks: GraphEngineCallbacks;
  private readonly deps: GraphEngineDeps;

  private worker: WorkerLike | null = null;
  private fallback: ForceEngine | null = null;
  private rafId: number | null = null;
  private currentPositions: Float32Array;
  private _settled = false;
  private stopped = true;
  private readonly onMessage: (ev: MessageEvent) => void;

  constructor(model: GraphModel, callbacks: GraphEngineCallbacks = {}, deps: GraphEngineDeps = {}) {
    this.model = model;
    this.callbacks = callbacks;
    this.deps = deps;
    this.currentPositions = model.positions;
    this.onMessage = (ev: MessageEvent) => this.handleWorkerMessage(ev.data as OutMessage);
  }

  get positions(): Float32Array {
    return this.currentPositions;
  }

  get settled(): boolean {
    return this._settled;
  }

  get usingWorker(): boolean {
    return this.worker !== null;
  }

  start(): void {
    if (!this.stopped) return;
    this.stopped = false;
    this._settled = false;

    const factory = this.deps.workerFactory ?? (() => new PhysicsWorker() as unknown as WorkerLike);
    let w: WorkerLike | null;
    try {
      w = factory();
    } catch {
      w = null;
    }

    if (w) {
      this.worker = w;
      w.addEventListener('message', this.onMessage);
      // 红线④：init 先 slice 再 transfer（模型原 buffer 不被 detach）
      const positions = this.model.positions.slice();
      const edges = this.model.edges.slice();
      const groupId = this.model.groupId.slice();
      const chargeScale = this.deps.chargeScale ? this.deps.chargeScale.slice() : undefined;
      const transfer: Transferable[] = [
        positions.buffer as ArrayBuffer,
        edges.buffer as ArrayBuffer,
        groupId.buffer as ArrayBuffer,
      ];
      if (chargeScale) transfer.push(chargeScale.buffer as ArrayBuffer);
      const init: InMessage = {
        type: 'init',
        count: this.model.count,
        edges,
        positions,
        params: { ...FORCE_DEFAULTS, ...this.deps.params },
        groupId,
        chargeScale,
      };
      try {
        w.postMessage(init, transfer);
      } catch {
        // postMessage 失败（如结构化克隆不支持）→ 回退主线程
        this.teardownWorker();
        this.startFallback();
      }
    } else {
      this.startFallback();
    }
  }

  stop(): void {
    this.stopped = true;
    this.teardownWorker();
    if (this.rafId !== null) {
      cancelAnimationFrame(this.rafId);
      this.rafId = null;
    }
    this.fallback = null;
  }

  /** 主线程兜底：RAF 每帧驱动物理；测试可直接调用（可选时间戳入参仅作调用约定兼容）。Worker 模式下为 no-op。 */
  stepFrame(_now?: number): void {
    if (!this.fallback || this.stopped || this._settled) return;
    this.fallback.tick();
    this.currentPositions = this.fallback.positions;
    this.callbacks.onTick?.(this.currentPositions, this.fallback.alpha);
    if (this.fallback.settled) {
      this._settled = true;
      this.callbacks.onSettled?.();
      if (this.rafId !== null) {
        cancelAnimationFrame(this.rafId);
        this.rafId = null;
      }
    }
  }

  pin(i: number, x: number, y: number, z: number): void {
    this._settled = false;
    if (this.worker) {
      this.worker.postMessage({ type: 'pin', i, x, y, z });
    } else {
      this.fallback?.pin(i, x, y, z);
      this.scheduleRaf();
    }
  }

  unpin(i: number): void {
    if (this.worker) {
      this.worker.postMessage({ type: 'unpin', i });
    } else {
      this.fallback?.unpin(i);
    }
  }

  reheat(): void {
    this._settled = false;
    if (this.worker) {
      this.worker.postMessage({ type: 'reheat' });
    } else {
      this.fallback?.reheat();
      this.scheduleRaf();
    }
  }

  setParams(params: Partial<ForceParams>): void {
    if (this.worker) {
      this.worker.postMessage({ type: 'setParams', params });
    } else {
      this.fallback?.setParams(params);
    }
  }

  private startFallback(): void {
    this.fallback = new ForceEngine({
      count: this.model.count,
      edges: this.model.edges,
      positions: this.model.positions,
      params: { ...FORCE_DEFAULTS, ...this.deps.params },
      groupId: this.model.groupId,
      chargeScale: this.deps.chargeScale,
    });
    this.currentPositions = this.fallback.positions;
    this.scheduleRaf();
  }

  private scheduleRaf(): void {
    if (this.rafId !== null || this.stopped) return;
    const loop = (): void => {
      this.rafId = null;
      this.stepFrame();
      if (!this._settled && !this.stopped && this.fallback) {
        this.rafId = requestAnimationFrame(loop);
      }
    };
    this.rafId = requestAnimationFrame(loop);
  }

  private handleWorkerMessage(msg: OutMessage): void {
    switch (msg.type) {
      case 'tick':
        this.currentPositions = msg.positions;
        this.callbacks.onTick?.(msg.positions, msg.alpha);
        break;
      case 'stopped':
        if (!this._settled) {
          this._settled = true;
          this.callbacks.onSettled?.();
        }
        break;
      case 'error':
        this.callbacks.onError?.(msg.message);
        break;
    }
  }

  private teardownWorker(): void {
    if (this.worker) {
      try {
        this.worker.postMessage({ type: 'stop' });
      } catch {
        /* 忽略 */
      }
      this.worker.removeEventListener('message', this.onMessage);
      this.worker.terminate();
      this.worker = null;
    }
  }
}
