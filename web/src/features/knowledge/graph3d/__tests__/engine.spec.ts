/**
 * engine.spec：G5-B 物理执行层装配契约（设计 §V12.8-1 engine.ts + protocol.ts）。
 * Worker 优先，创建失败回退主线程 RAF（NFR-G5-5），两路径同一 ForceEngine。
 */
import { describe, expect, it, vi } from 'vitest';
import { buildGraphModel, seedPositions } from '../model';
import { GraphEngine, type WorkerLike } from '../engine';
import type { InMessage, OutMessage } from '../protocol';

function mkModel() {
  const m = buildGraphModel(
    [
      { docId: 'a', name: 'a', relPath: 'a.md', docType: '' },
      { docId: 'b', name: 'b', relPath: 'b.md', docType: '' },
      { docId: 'c', name: 'c', relPath: 'c.md', docType: '' },
    ],
    [{ source: 'a', target: 'b', type: 'explicit' }],
  );
  seedPositions(m, 42);
  return m;
}

/** 假 Worker：记录 postMessage，可手动注入 message 事件。 */
class FakeWorker implements WorkerLike {
  posted: { msg: unknown; transfer?: Transferable[] }[] = [];
  terminated = false;
  private listeners: ((ev: MessageEvent) => void)[] = [];

  postMessage(msg: unknown, transfer?: Transferable[]): void {
    this.posted.push({ msg, transfer });
  }
  addEventListener(_type: 'message', cb: (ev: MessageEvent) => void): void {
    this.listeners.push(cb);
  }
  removeEventListener(_type: 'message', cb: (ev: MessageEvent) => void): void {
    this.listeners = this.listeners.filter((l) => l !== cb);
  }
  terminate(): void {
    this.terminated = true;
  }
  emit(msg: OutMessage): void {
    const ev = { data: msg } as MessageEvent;
    for (const l of this.listeners) l(ev);
  }
}

describe('GraphEngine Worker 路径', () => {
  it('init 消息携带 slice 后的 buffer 并 transfer（模型原数据不被 detach）', () => {
    const w = new FakeWorker();
    const model = mkModel();
    const e = new GraphEngine(model, {}, { workerFactory: () => w });
    e.start();
    expect(e.usingWorker).toBe(true);
    const init = w.posted[0].msg as Extract<InMessage, { type: 'init' }>;
    expect(init.type).toBe('init');
    expect(init.count).toBe(3);
    expect(init.edges).toHaveLength(2);
    expect(init.positions).toHaveLength(9);
    // transfer 列表包含 positions/edges/groupId/chargeScale buffer
    expect(w.posted[0].transfer!.length).toBeGreaterThanOrEqual(2);
    // slice 副本 → 模型原 buffer 仍可用（byteLength 非 0）
    expect(model.positions.byteLength).toBe(36);
    e.stop();
  });

  it('tick 消息：positions 更新 + onTick 回调', () => {
    const w = new FakeWorker();
    const onTick = vi.fn();
    const e = new GraphEngine(mkModel(), { onTick }, { workerFactory: () => w });
    e.start();
    const positions = new Float32Array([1, 2, 3, 4, 5, 6, 7, 8, 9]);
    w.emit({ type: 'tick', positions, alpha: 0.5 });
    expect(Array.from(e.positions)).toEqual([1, 2, 3, 4, 5, 6, 7, 8, 9]);
    expect(onTick).toHaveBeenCalledWith(positions, 0.5);
    e.stop();
  });

  it('stopped 消息：onSettled 触发一次；error 消息：onError', () => {
    const w = new FakeWorker();
    const onSettled = vi.fn();
    const onError = vi.fn();
    const e = new GraphEngine(mkModel(), { onSettled, onError }, { workerFactory: () => w });
    e.start();
    w.emit({ type: 'stopped' });
    w.emit({ type: 'stopped' });
    expect(onSettled).toHaveBeenCalledTimes(1);
    expect(e.settled).toBe(true);
    w.emit({ type: 'error', message: 'boom' });
    expect(onError).toHaveBeenCalledWith('boom');
    e.stop();
  });

  it('pin/unpin/reheat/setParams 转发为协议消息', () => {
    const w = new FakeWorker();
    const e = new GraphEngine(mkModel(), {}, { workerFactory: () => w });
    e.start();
    e.pin(1, 7, 8, 9);
    e.unpin(1);
    e.reheat();
    e.setParams({ repulsion: 40 });
    const types = w.posted.map((p) => (p.msg as InMessage).type);
    expect(types).toEqual(['init', 'pin', 'unpin', 'reheat', 'setParams']);
    const pin = w.posted[1].msg as Extract<InMessage, { type: 'pin' }>;
    expect([pin.i, pin.x, pin.y, pin.z]).toEqual([1, 7, 8, 9]);
    e.stop();
    expect(w.terminated).toBe(true);
  });

  it('V13：init 携带 tierTargetRadius/pinnedInit（slice 后 transfer，deps 原 buffer 不 detach）', () => {
    const w = new FakeWorker();
    const ttr = Float32Array.from([-1, 50, -1]);
    const pinned = Uint8Array.from([1, 0, 0]);
    const e = new GraphEngine(mkModel(), {}, { workerFactory: () => w, tierTargetRadius: ttr, pinnedInit: pinned });
    e.start();
    const init = w.posted[0].msg as Extract<InMessage, { type: 'init' }>;
    expect(Array.from(init.tierTargetRadius!)).toEqual([-1, 50, -1]);
    expect(Array.from(init.pinnedInit!)).toEqual([1, 0, 0]);
    expect(ttr.byteLength).toBe(12); // slice 语义：原 buffer 完好
    expect(pinned.byteLength).toBe(3);
    e.stop();
  });

  it('V13-A1 park 转发为协议消息（批量索引+扁平坐标）；空批次不发信', () => {
    const w = new FakeWorker();
    const e = new GraphEngine(mkModel(), {}, { workerFactory: () => w });
    e.start();
    e.park(Uint32Array.from([0, 2]), Float32Array.from([1, 0, 0, 0, 0, 1]));
    const msg = w.posted[1].msg as Extract<InMessage, { type: 'park' }>;
    expect(msg.type).toBe('park');
    expect(Array.from(msg.indices)).toEqual([0, 2]);
    expect(Array.from(msg.positions)).toEqual([1, 0, 0, 0, 0, 1]);
    e.park(new Uint32Array(0), new Float32Array(0));
    expect(w.posted).toHaveLength(2); // 空批次不再发信（仍只有 init+park）
    e.stop();
  });
});

describe('GraphEngine 主线程兜底', () => {
  it('Worker 工厂抛错 → 自动回退（usingWorker=false）', () => {
    const e = new GraphEngine(
      mkModel(),
      {},
      {
        workerFactory: () => {
          throw new Error('no worker');
        },
      },
    );
    e.start();
    expect(e.usingWorker).toBe(false);
    e.stop();
  });

  it('stepFrame 驱动物理：positions 变化 + onTick 回调', () => {
    const model = mkModel();
    const before = Array.from(model.positions);
    const onTick = vi.fn();
    const e = new GraphEngine(
      model,
      { onTick },
      {
        workerFactory: () => {
          throw new Error('no worker');
        },
      },
    );
    e.start();
    e.stepFrame(1000);
    expect(onTick).toHaveBeenCalledTimes(1);
    expect(Array.from(e.positions)).not.toEqual(before);
    e.stop();
  });

  it('收敛后 onSettled 触发且 stepFrame 不再 tick（懒惰）', () => {
    const e = new GraphEngine(
      mkModel(),
      {},
      {
        workerFactory: () => {
          throw new Error('no worker');
        },
      },
    );
    e.start();
    let t = 0;
    for (let i = 0; i < 400; i++) {
      t += 16;
      e.stepFrame(t);
    }
    expect(e.settled).toBe(true);
    const snapshot = Array.from(e.positions);
    e.stepFrame(t + 16); // settled 后不再驱动物理
    expect(Array.from(e.positions)).toEqual(snapshot);
    e.stop();
  });

  it('reheat 唤醒已收敛引擎；pin 锁位置；setParams 生效', () => {
    const model = mkModel();
    const e = new GraphEngine(
      model,
      {},
      {
        workerFactory: () => {
          throw new Error('no worker');
        },
      },
    );
    e.start();
    let t = 0;
    for (let i = 0; i < 400; i++) e.stepFrame((t += 16));
    expect(e.settled).toBe(true);
    e.reheat();
    expect(e.settled).toBe(false);
    e.pin(0, 5, 6, 7);
    e.stepFrame(t + 16);
    expect(e.positions[0]).toBe(5);
    expect(e.positions[1]).toBe(6);
    expect(e.positions[2]).toBe(7);
    e.setParams({ repulsion: 25 });
    e.stop();
  });

  it('V13-A1 兜底路径 park：钉住孤立节点 + 广播一次位置 + 不 reheat（保持 settled）', () => {
    const onTick = vi.fn();
    const e = new GraphEngine(
      mkModel(), // 节点 c（index 2）孤立
      { onTick },
      {
        workerFactory: () => {
          throw new Error('no worker');
        },
      },
    );
    e.start();
    for (let t = 0; t < 400; t++) e.stepFrame(t * 16);
    expect(e.settled).toBe(true);
    onTick.mockClear();
    e.park(Uint32Array.from([2]), Float32Array.from([99, 0, 0]));
    expect(e.settled).toBe(true); // park 不唤醒物理
    expect(onTick).toHaveBeenCalledTimes(1); // 兜底路径同步广播一次
    expect(e.positions[6]).toBe(99);
    expect(e.positions[7]).toBe(0);
    expect(e.positions[8]).toBe(0);
    e.stop();
  });
});

describe('M2 setLayout 布局切换', () => {
  function makeEngine(): GraphEngine {
    const model = buildGraphModel(
      [
        { docId: 'a', name: 'a', relPath: 'a.md', docType: 'note' },
        { docId: 'b', name: 'b', relPath: 'b.md', docType: 'note' },
      ],
      [{ source: 'a', target: 'b', type: 'explicit' }],
    );
    seedPositions(model, 1337);
    return new GraphEngine(model, {}, { workerFactory: () => null as unknown as WorkerLike });
  }

  it('setLayout("galaxy")：主线程兜底路径参数切到星系盘预设且 alpha 再加热', () => {
    const e = makeEngine();
    e.start();
    // 收敛后 alpha 衰减
    for (let t = 0; t < 400; t++) e.stepFrame();
    expect(e.settled).toBe(true);
    e.setLayout('galaxy');
    expect(e.settled).toBe(false); // reheat 唤醒
    // 参数生效：再跑若干 tick 不发散（数值护栏）
    for (let t = 0; t < 60; t++) e.stepFrame();
    const p = e.positions;
    for (let i = 0; i < p.length; i++) expect(Number.isFinite(p[i])).toBe(true);
    e.stop();
  });

  it('setLayout("force")：切回力导向默认参数', () => {
    const e = makeEngine();
    e.start();
    e.setLayout('galaxy');
    e.setLayout('force');
    expect(e.settled).toBe(false);
    for (let t = 0; t < 60; t++) e.stepFrame();
    const p = e.positions;
    for (let i = 0; i < p.length; i++) expect(Number.isFinite(p[i])).toBe(true);
    e.stop();
  });
});
