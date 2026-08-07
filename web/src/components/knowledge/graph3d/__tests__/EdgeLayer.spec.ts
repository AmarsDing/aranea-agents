/**
 * EdgeLayer.spec：G5-C 边层契约（设计 §V12.8-1 C-2）。
 */
import { describe, expect, it } from 'vitest';
import { EDGE_HOVER_BOOST, EDGE_REST_DIM, EDGE_SEGMENTS, EdgeLayer, hash01 } from '../render/EdgeLayer';

const VERTS = EDGE_SEGMENTS * 2;

function mkLayer(): { l: EdgeLayer; positions: Float32Array } {
  // 边 0：(0,0,0)-(10,0,0)；边 1：(10,0,0)-(10,10,0)
  const edges = Int32Array.from([0, 1, 1, 2]);
  const colors = new Float32Array([1, 0, 0, 0, 1, 0]);
  const l = new EdgeLayer(edges, colors);
  const positions = new Float32Array([0, 0, 0, 10, 0, 0, 10, 10, 0]);
  return { l, positions };
}

function posAttr(l: EdgeLayer): Float32Array {
  return (l.object.geometry.getAttribute('position') as { array: Float32Array }).array;
}

function colAttr(l: EdgeLayer): Float32Array {
  return (l.object.geometry.getAttribute('color') as { array: Float32Array }).array;
}

describe('hash01', () => {
  it('稳定且在 [0,1)', () => {
    expect(hash01('3->7')).toBe(hash01('3->7'));
    expect(hash01('3->7')).not.toBe(hash01('7->3'));
    for (const k of ['0->1', '12->993', '5->5']) {
      expect(hash01(k)).toBeGreaterThanOrEqual(0);
      expect(hash01(k)).toBeLessThan(1);
    }
  });
});

describe('EdgeLayer', () => {
  it('缓冲结构：每边 6 段 12 顶点', () => {
    const { l } = mkLayer();
    expect(posAttr(l)).toHaveLength(2 * VERTS * 3);
    expect(colAttr(l)).toHaveLength(2 * VERTS * 3);
    l.dispose();
  });

  it('边首顶点=源点、末顶点=终点（贝塞尔端点精确）', () => {
    const { l, positions } = mkLayer();
    l.updatePositions(positions);
    const arr = posAttr(l);
    // 边 0 首顶点
    expect([arr[0], arr[1], arr[2]]).toEqual([0, 0, 0]);
    // 边 0 末顶点（最后一段的第二顶点）
    const last = (VERTS - 1) * 3;
    expect([arr[last], arr[last + 1], arr[last + 2]]).toEqual([10, 0, 0]);
    l.dispose();
  });

  it('微弯：中点偏离直线（bow>0），同边弯向确定', () => {
    const { l, positions } = mkLayer();
    l.updatePositions(positions);
    const arr = posAttr(l);
    // 边 0 中间段顶点应偏离 x 轴（y 或 z 非零）
    const midOff = Math.floor(EDGE_SEGMENTS / 2) * 6;
    const my = arr[midOff + 1];
    const mz = arr[midOff + 2];
    expect(Math.abs(my) + Math.abs(mz)).toBeGreaterThan(0.01);
    // 确定性：重算结果一致
    const snapshot = Array.from(arr.slice(0, VERTS * 3));
    l.updatePositions(positions);
    expect(Array.from(posAttr(l).slice(0, VERTS * 3))).toEqual(snapshot);
    l.dispose();
  });

  it('rest=×0.32，hover 关联边=×0.9', () => {
    const { l } = mkLayer();
    let col = colAttr(l);
    expect(col[0]).toBeCloseTo(EDGE_REST_DIM, 5); // 边0 红通道 rest
    expect(col[VERTS * 3 + 1]).toBeCloseTo(EDGE_REST_DIM, 5); // 边1 绿通道 rest
    l.setHighlight(new Set([1]));
    col = colAttr(l);
    expect(col[VERTS * 3 + 1]).toBeCloseTo(EDGE_HOVER_BOOST, 5);
    expect(col[0]).toBeCloseTo(EDGE_REST_DIM, 5); // 非关联边不变
    l.setHighlight(null);
    expect(colAttr(l)[VERTS * 3 + 1]).toBeCloseTo(EDGE_REST_DIM, 5);
    l.dispose();
  });

  it('updateEdgesFor 只重写指定边', () => {
    const { l, positions } = mkLayer();
    l.updatePositions(positions);
    // 移动节点 0 但只重写边 0；边 1 顶点应保持旧值
    positions[0] = 5;
    const before1 = Array.from(posAttr(l).slice(VERTS * 3, VERTS * 3 * 2));
    l.updateEdgesFor([0], positions);
    expect(posAttr(l)[0]).toBe(5); // 边 0 首顶点已更新
    expect(Array.from(posAttr(l).slice(VERTS * 3, VERTS * 3 * 2))).toEqual(before1); // 边 1 未动
    l.dispose();
  });

  it('端点重合退化为零长线段（无 NaN）', () => {
    const edges = Int32Array.from([0, 1]);
    const l = new EdgeLayer(edges, new Float32Array([1, 1, 1]));
    const positions = new Float32Array([3, 3, 3, 3, 3, 3]);
    l.updatePositions(positions);
    for (const v of posAttr(l)) expect(Number.isFinite(v)).toBe(true);
    l.dispose();
  });
});
