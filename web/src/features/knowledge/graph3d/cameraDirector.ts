/**
 * cameraDirector：M3 电影感镜头状态机（AS-FSM-01 显式状态机，纯 TS 零 three 依赖）。
 *
 * 5 状态：idle（用户自由操控）/ flying（飞往目标节点）/ orbiting（绕目标缓转）
 *         / cruising（全局巡游）/ genesis（创世绽放，驱动 NodeLayer uRevealT）
 * 用户交互（user-interrupt）任何进行态 → idle（镜头让位手控）。
 * update(progress01) 输出当前帧相机指令（位置/看向/revealT），由 Canvas RAF 驱动。
 */

export type CameraState = 'idle' | 'flying' | 'orbiting' | 'cruising' | 'genesis';
export type CameraEvent = 'focus' | 'genesis' | 'cruise' | 'arrived' | 'timeout' | 'completed' | 'user-interrupt';

export type Vec3 = [number, number, number];

export interface CameraPose {
  position: Vec3;
  lookAt: Vec3;
  /** 创世进度（非 genesis 状态恒 1）。 */
  revealT: number;
}

export interface FocusPayload {
  target: Vec3;
  distance: number;
  from?: Vec3;
}

export interface GenesisPayload {
  duration: number;
}

/** 合法转换表：state → 允许的事件集。 */
const TRANSITIONS: Record<CameraState, ReadonlySet<CameraEvent>> = {
  idle: new Set(['focus', 'genesis', 'cruise']),
  flying: new Set(['arrived', 'user-interrupt']),
  orbiting: new Set(['user-interrupt', 'timeout', 'focus']),
  cruising: new Set(['user-interrupt', 'focus']),
  genesis: new Set(['completed', 'user-interrupt']),
};

/** 事件 → 目标状态。 */
const EVENT_TARGET: Record<CameraEvent, CameraState> = {
  focus: 'flying',
  genesis: 'genesis',
  cruise: 'cruising',
  arrived: 'orbiting',
  timeout: 'idle',
  completed: 'idle',
  'user-interrupt': 'idle',
};

export function canTransition(from: CameraState, event: CameraEvent): boolean {
  return TRANSITIONS[from].has(event);
}

function easeInOutQuad(t: number): number {
  return t < 0.5 ? 2 * t * t : 1 - (-2 * t + 2) ** 2 / 2;
}

export class CameraDirector {
  private _state: CameraState = 'idle';
  private focusTarget: FocusPayload | null = null;
  private orbitAngle = 0;

  get state(): CameraState {
    return this._state;
  }

  dispatch(event: CameraEvent, payload?: FocusPayload | GenesisPayload): boolean {
    if (!canTransition(this._state, event)) return false;
    if (event === 'focus') this.focusTarget = payload as FocusPayload;
    this._state = EVENT_TARGET[event];
    return true;
  }

  /** progress01：当前状态内的归一化进度（flying/genesis 用；orbiting 内部累计角度）。 */
  update(progress01: number): CameraPose {
    const t = Math.min(1, Math.max(0, progress01));
    if (this._state === 'flying' && this.focusTarget) {
      const { target, distance, from = [0, 0, 400] } = this.focusTarget;
      const e = easeInOutQuad(t);
      // 二次贝塞尔甩镜：控制点侧向抬升（电影感弧度）
      const mid: Vec3 = [(from[0] + target[0]) / 2, (from[1] + target[1]) / 2 + distance * 0.6, (from[2] + target[2]) / 2];
      const quad = (a: number, c: number, b: number): number => (1 - e) * ((1 - e) * a + e * c) + e * ((1 - e) * c + e * b);
      const end: Vec3 = [target[0], target[1], target[2] + distance];
      return {
        position: [quad(from[0], mid[0], end[0]), quad(from[1], mid[1], end[1]), quad(from[2], mid[2], end[2])],
        lookAt: [from[0] + (target[0] - from[0]) * e, from[1] + (target[1] - from[1]) * e, from[2] + (target[2] - from[2]) * e],
        revealT: 1,
      };
    }
    if (this._state === 'orbiting' && this.focusTarget) {
      this.orbitAngle += 0.004;
      const { target, distance } = this.focusTarget;
      return {
        position: [target[0] + Math.sin(this.orbitAngle) * distance, target[1] + distance * 0.25, target[2] + Math.cos(this.orbitAngle) * distance],
        lookAt: target,
        revealT: 1,
      };
    }
    if (this._state === 'genesis') {
      return { position: [0, 0, 400], lookAt: [0, 0, 0], revealT: t === 1 ? 1 : easeInOutQuad(t) };
    }
    // idle/cruising：不干预（Canvas 忽略输出）
    return { position: [0, 0, 0], lookAt: [0, 0, 0], revealT: 1 };
  }

  reset(): void {
    this._state = 'idle';
    this.focusTarget = null;
    this.orbitAngle = 0;
  }
}
