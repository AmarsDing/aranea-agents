import { describe, expect, it } from 'vitest';
import { buildRunTrack } from '../runTrack';

/** 最小 span 形状（与 monitor_trace_spans 的扁平协议一致）。 */
function span(partial: Record<string, unknown>) {
  return { id: 'x', name: 'x', status: 'ok', start_ms: 0, duration_ms: 10, ...partial };
}

describe('buildRunTrack', () => {
  it('空输入返回空轨迹，总时长回退 1 避免除零', () => {
    const track = buildRunTrack([]);

    expect(track.steps).toHaveLength(0);
    expect(track.totalMs).toBe(1);
    expect(track.firstFailedId).toBe('');
    expect(track.slowestId).toBe('');
    expect(track.failedCount).toBe(0);
  });

  it('按 start_ms 全局时间顺序排列，同起点按名称', () => {
    const track = buildRunTrack([
      span({ id: 'c', name: 'third', start_ms: 300, duration_ms: 20 }),
      span({ id: 'b', name: 'second', start_ms: 100, duration_ms: 20 }),
      span({ id: 'a2', name: 'beta', start_ms: 100, duration_ms: 5 }),
      span({ id: 'a1', name: 'alpha', start_ms: 100, duration_ms: 5 }),
    ]);

    expect(track.steps.map((s) => s.id)).toEqual(['a1', 'a2', 'b', 'c']);
  });

  it('offsetPct / widthPct 按整条运行归一，宽度有可见下限', () => {
    const track = buildRunTrack([
      span({ id: 'a', start_ms: 0, duration_ms: 100 }),
      span({ id: 'b', start_ms: 900, duration_ms: 100 }),
      span({ id: 'tiny', start_ms: 500, duration_ms: 1 }),
    ]);

    expect(track.totalMs).toBe(1000);
    expect(track.steps.find((s) => s.id === 'b')?.offsetPct).toBe(90);
    expect(track.steps.find((s) => s.id === 'b')?.widthPct).toBe(10);
    // 1ms/1000ms = 0.1% 仍要能看见 → 抬到 0.6% 下限
    expect(track.steps.find((s) => s.id === 'tiny')?.widthPct).toBe(0.6);
  });

  it('未关闭（duration_ms=0）的步骤按「到运行结束」占满剩余区间', () => {
    const track = buildRunTrack([
      span({ id: 'long', start_ms: 0, duration_ms: 800 }),
      span({ id: 'open', start_ms: 600, duration_ms: 0 }),
    ]);

    const open = track.steps.find((s) => s.id === 'open');
    expect(open?.widthPct).toBe(25);
  });

  it('firstFailedId 定位自身失败的步骤，后代失败上卷不误判', () => {
    const track = buildRunTrack([
      span({ id: 'root', name: 'agent.run', start_ms: 0, duration_ms: 500, status: 'ok' }),
      span({ id: 'child', parent_id: 'root', name: 'tool.call', start_ms: 100, duration_ms: 50, status: 'error', error: 'boom' }),
      span({ id: 'later', parent_id: 'root', name: 'tool.retry', start_ms: 200, duration_ms: 50, status: 'error' }),
    ]);

    expect(track.firstFailedId).toBe('child');
    // root 自身没失败，只是后代失败上卷
    expect(track.steps.find((s) => s.id === 'root')?.failed).toBe(false);
    expect(track.steps.find((s) => s.id === 'root')?.hasError).toBe(true);
    expect(track.failedCount).toBe(2);
  });

  it('slowestId 取耗时最长的步骤，并列取先出现者', () => {
    const track = buildRunTrack([
      span({ id: 'a', start_ms: 0, duration_ms: 400 }),
      span({ id: 'b', start_ms: 10, duration_ms: 700 }),
      span({ id: 'c', start_ms: 20, duration_ms: 700 }),
    ]);

    expect(track.slowestId).toBe('b');
  });

  it('保留 depth 供缩进、kind 缺失回退固定标签', () => {
    const track = buildRunTrack([
      span({ id: 'root', kind: 'agent', start_ms: 0, duration_ms: 100 }),
      span({ id: 'child', parent_id: 'root', start_ms: 10, duration_ms: 20 }),
    ]);

    expect(track.steps.find((s) => s.id === 'child')?.depth).toBe(1);
    expect(track.steps.find((s) => s.id === 'child')?.kind).toBe('');
  });

  it('错误信息透出到步骤，供 hover/详情断言', () => {
    const track = buildRunTrack([span({ id: 'bad', status: 'error', error: { message: 'permission denied' } })]);

    expect(track.steps[0].error).toBe('permission denied');
    expect(track.steps[0].tone).toBe('error');
  });
});
