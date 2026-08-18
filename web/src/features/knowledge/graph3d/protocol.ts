/**
 * protocol：G5 物理 Worker 消息协议（移植 fast-graph，设计 §V12.8-1 protocol.ts）。
 *
 * 入向（主线程→Worker）：init（slice 后 transfer）/setParams/pin/unpin/reheat/stop
 * 出向（Worker→主线程）：tick{positions,alpha}（transfer 零拷贝）/stopped/error
 */
import type { ForceParams } from './forces';

export interface InitMessage {
  type: 'init';
  count: number;
  edges: Int32Array;
  positions: Float32Array;
  params: ForceParams;
  groupId?: Uint16Array;
  chargeScale?: Float32Array;
  /** V13-B：per-node 目标半径壳层（stratify>0 时生效；<0 = 不分层）。 */
  tierTargetRadius?: Float32Array;
  /** V13-A1：初始即钉住的节点（孤立节点先冻在播种球壳上，收敛后统一停泊环）。 */
  pinnedInit?: Uint8Array;
}

export interface SetParamsMessage {
  type: 'setParams';
  params: Partial<ForceParams>;
}

export interface PinMessage {
  type: 'pin';
  i: number;
  x: number;
  y: number;
  z: number;
}

export interface UnpinMessage {
  type: 'unpin';
  i: number;
}

/**
 * V13-A1 批量停泊：把孤立节点钉到停泊环坐标（不 reheat）。
 * Worker 收信后补发一次 tick 广播最终坐标；loop 不因停泊重启。
 */
export interface ParkMessage {
  type: 'park';
  /** 批量停泊节点索引。 */
  indices: Uint32Array;
  /** 与 indices 对齐的扁平 xyz（长度 = indices.length * 3）。 */
  positions: Float32Array;
}

export interface ReheatMessage {
  type: 'reheat';
}

export interface StopMessage {
  type: 'stop';
}

export type InMessage =
  | InitMessage
  | SetParamsMessage
  | PinMessage
  | UnpinMessage
  | ParkMessage
  | ReheatMessage
  | StopMessage;

export interface TickMessage {
  type: 'tick';
  positions: Float32Array;
  alpha: number;
}

export interface StoppedMessage {
  type: 'stopped';
}

export interface ErrorMessage {
  type: 'error';
  message: string;
}

export type OutMessage = TickMessage | StoppedMessage | ErrorMessage;
