import { describe, expect, it } from 'vitest';
import {
  CameraDirector,
  canTransition,
  type CameraEvent,
  type CameraState,
} from '../cameraDirector';

describe('cameraDirector 状态机（M3）', () => {
  it('合法转换表：idle→focus/genesis/cruise；flying→arrived/user-interrupt；orbiting→user-interrupt/timeout/focus；genesis→completed/user-interrupt；cruising→user-interrupt/focus', () => {
    const legal: Array<[CameraState, CameraEvent]> = [
      ['idle', 'focus'],
      ['idle', 'genesis'],
      ['idle', 'cruise'],
      ['flying', 'arrived'],
      ['flying', 'user-interrupt'],
      ['orbiting', 'user-interrupt'],
      ['orbiting', 'timeout'],
      ['orbiting', 'focus'],
      ['genesis', 'completed'],
      ['genesis', 'user-interrupt'],
      ['cruising', 'user-interrupt'],
      ['cruising', 'focus'],
    ];
    for (const [s, e] of legal) expect(canTransition(s, e), `${s}+${e}`).toBe(true);
  });

  it('非法转换拒绝：idle+arrived / orbiting+genesis / cruising+completed', () => {
    expect(canTransition('idle', 'arrived')).toBe(false);
    expect(canTransition('orbiting', 'genesis')).toBe(false);
    expect(canTransition('cruising', 'completed')).toBe(false);
  });

  it('dispatch 驱动状态迁移：idle→focus→flying→arrived→orbiting→user-interrupt→idle', () => {
    const d = new CameraDirector();
    expect(d.state).toBe('idle');
    expect(d.dispatch('focus', { target: [10, 0, 0], distance: 60 })).toBe(true);
    expect(d.state).toBe('flying');
    expect(d.dispatch('arrived')).toBe(true);
    expect(d.state).toBe('orbiting');
    expect(d.dispatch('user-interrupt')).toBe(true);
    expect(d.state).toBe('idle');
  });

  it('非法 dispatch 返回 false 且状态不变', () => {
    const d = new CameraDirector();
    expect(d.dispatch('arrived')).toBe(false);
    expect(d.state).toBe('idle');
  });

  it('flying 插值：update(0) 在起点附近，update(1) 距目标 distance 处且看向目标', () => {
    const d = new CameraDirector();
    d.dispatch('focus', { target: [100, 0, 0], distance: 50, from: [0, 0, 400] });
    const p0 = d.update(0);
    const p1 = d.update(1);
    expect(p0.position[2]).toBeCloseTo(400, 1);
    expect(Math.hypot(p1.position[0] - 100, p1.position[1], p1.position[2])).toBeCloseTo(50, 0);
    expect(p1.lookAt).toEqual([100, 0, 0]);
  });

  it('genesis：update 输出 revealT 0→1（供 NodeLayer uRevealT uniform）', () => {
    const d = new CameraDirector();
    d.dispatch('genesis', { duration: 1200 });
    expect(d.update(0).revealT).toBe(0);
    expect(d.update(0.5).revealT).toBeGreaterThan(0);
    expect(d.update(0.5).revealT).toBeLessThan(1);
    expect(d.update(1).revealT).toBe(1);
  });
});
