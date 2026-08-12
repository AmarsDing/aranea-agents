/**
 * Computer Use 步骤流 composable（75 M1.4 任务 4）。
 *
 * 订阅 WS monitor 通道的 `computeruse.step` MonitorEvent（后端
 * internal/computeruse/step_events.go 发布），按 step_index 聚合为
 * 步骤卡片视图模型；头部急停按钮调 KillComputerUseSession API。
 *
 * 可靠性：事件为 Informational 级（尽力而为），持久化以
 * computer_use_audit 表为准（ListComputerUseSteps 可回补）。
 */
import { ref, onUnmounted, type Ref } from 'vue';
import { createMonitorStream } from '../../realtime/useMonitorStream';
import type { MonitorEvent } from '../../realtime/monitorEvent';
import type { Step } from '../chat/v2Types';
import { GLOBAL_WS_SESSION_ID } from '../../config/runtime';
import { createComputerUseService } from '../../services/index';

/** CuStep 步骤流视图模型（字段对齐 step_events.go Metadata）。 */
export interface CuStep {
  stepIndex: number;
  target: string;
  path: string;
  action: string;
  result: string;
  durationMs: number;
  danger: boolean;
  confirmedBy: string;
  error: string;
  timestamp: string;
}

export type CuKillState = 'active' | 'killing' | 'killed';

function metadataStr(md: Record<string, unknown>, key: string): string {
  const v = md[key];
  return typeof v === 'string' ? v : v == null ? '' : String(v);
}

function metadataNum(md: Record<string, unknown>, key: string): number {
  const v = md[key];
  return typeof v === 'number' && Number.isFinite(v) ? v : 0;
}

/** cuStepFromMonitorEvent 把 computeruse.step MonitorEvent 映射为视图模型；
 * 非该类型或缺 metadata 返回 null（调用方忽略）。 */
export function cuStepFromMonitorEvent(ev: MonitorEvent): CuStep | null {
  if (ev.type !== 'computeruse.step' || !ev.metadata) return null;
  const md = ev.metadata;
  return {
    stepIndex: metadataNum(md, 'step_index'),
    target: metadataStr(md, 'target'),
    path: metadataStr(md, 'path'),
    action: metadataStr(md, 'action'),
    result: metadataStr(md, 'result'),
    durationMs: metadataNum(md, 'duration_ms'),
    danger: md.danger === true,
    confirmedBy: metadataStr(md, 'confirmed_by'),
    error: metadataStr(md, 'error'),
    timestamp: ev.timestamp,
  };
}

/** upsertCuStep 按 step_index 插入/覆盖并保持升序（去重：同 index 后者覆盖）。 */
export function upsertCuStep(list: CuStep[], step: CuStep): CuStep[] {
  const idx = list.findIndex((s) => s.stepIndex === step.stepIndex);
  if (idx >= 0) {
    const next = list.slice();
    next[idx] = step;
    return next;
  }
  return [...list, step].sort((a, b) => a.stepIndex - b.stepIndex);
}

/**
 * cuSessionIdFromSteps 从 turn 的 steps 提取 computeruse 会话 ID——任一
 * computer_use_* 工具的 ToolResult.session_id（后端 tools.go 各工具均返回
 * session_id）。用于聊天气泡内嵌 CuStepStream 的会话绑定；无匹配返回 ''。
 */
export function cuSessionIdFromSteps(steps: Step[]): string {
  for (const step of steps) {
    if (!step.ToolName?.startsWith('computer_use_')) continue;
    const res = step.ToolResult;
    if (res == null || typeof res !== 'object' || Array.isArray(res)) continue;
    const sid = (res as Record<string, unknown>).session_id;
    if (typeof sid === 'string' && sid.trim()) return sid;
  }
  return '';
}

export function useCuStepStream(sessionId: Ref<string>) {
  const steps = ref<CuStep[]>([]);
  const killState = ref<CuKillState>('active');

  const stream = createMonitorStream({
    sessionId: GLOBAL_WS_SESSION_ID,
    channels: ['monitor'],
    autoConnect: true,
    onMonitorEvent: (ev) => {
      if (ev.type !== 'computeruse.step') return;
      // 事件 session_id 即 computeruse 会话 ID；组件绑定具体会话时过滤。
      if (sessionId.value && ev.session_id && ev.session_id !== sessionId.value) return;
      const step = cuStepFromMonitorEvent(ev);
      if (!step) return;
      steps.value = upsertCuStep(steps.value, step);
      if (step.result === 'cancelled') killState.value = 'killed';
    },
  });

  async function kill(): Promise<void> {
    const id = sessionId.value.trim();
    if (!id || killState.value !== 'active') return;
    killState.value = 'killing';
    try {
      await createComputerUseService().KillComputerUseSession({ id });
      killState.value = 'killed';
    } catch (err) {
      // 急停失败必须允许重试（安全功能不可卡死）。
      killState.value = 'active';
      throw err;
    }
  }

  onUnmounted(() => stream.disconnect());

  return { steps, killState, kill, connected: stream.connected };
}
