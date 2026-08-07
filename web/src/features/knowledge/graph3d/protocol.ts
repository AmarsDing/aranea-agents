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

export interface ReheatMessage {
  type: 'reheat';
}

export interface StopMessage {
  type: 'stop';
}

export type InMessage = InitMessage | SetParamsMessage | PinMessage | UnpinMessage | ReheatMessage | StopMessage;

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
