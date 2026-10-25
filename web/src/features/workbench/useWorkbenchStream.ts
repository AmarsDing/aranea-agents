/**
 * 作战总台实时归并 composable：globalWsHub 单通道消费（设计方案 §5.1 实时增量层）。
 *
 * 事件流 → 三块现场：
 *   step.*        → AgentLiveState（角色行状态词 / 走廊点亮 / ACTIVITY ticker / 日志卷宗）
 *   confirm 类 step → 等人事件（角色行转金 + poke YOU 队列直刷）
 *   task/team_run completed → 心跳计数（今日已完成，本屏在线期间口径）
 *
 * 只消费、不建通道（globalWsHub 单例；chat/system 频道由枢纽自动附加）。
 */
import { computed, onUnmounted, reactive, ref } from 'vue';
import { acquireGlobalWsConsumer, releaseGlobalWsConsumer, globalWsHubConnected } from '../../realtime/globalWsHub';
import type { V2WsEnvelope, Step } from '../chat/v2Types';
import {
  liveKindFromStep,
  makeLogLine,
  newLiveState,
  stepToToolBeat,
  toolBeatText,
  LOG_LINES_CAP,
  RECENT_TOOLS_CAP,
} from './logic';
import type { AgentLiveState, CompletionBeat, LogLine } from './types';

const RATE_WINDOW_MS = 60_000;
/** WS 静默超过此时长 → 信任态提示 stale（信任栏展示用） */
export const WS_STALE_MS = 5 * 60_000;

let seq = 0;
function nextId(prefix: string): string {
  seq += 1;
  return `${prefix}:${seq}`;
}

function payloadStep(envelope: V2WsEnvelope): Step | null {
  const p = envelope.payload as { Step?: Step } | undefined;
  return p?.Step ?? null;
}

export function useWorkbenchStream(opts: { onConfirmActivity?: () => void } = {}) {
  /** agentKey → 现场（reactive Map：角色行直接索引） */
  const liveAgents = reactive(new Map<string, AgentLiveState>());
  /** 等批归因集（confirm step 未了结即转金；签字回执/失败即熄） */
  const waitingAgents = reactive(new Set<string>());
  const logLines = ref<LogLine[]>([]);
  const completions = ref<CompletionBeat[]>([]);
  const eventRatePerMin = ref(0);
  const lastEventAt = ref('');
  const connected = ref(globalWsHubConnected());

  const eventTimes: number[] = [];
  let consumerId = '';
  let rateTimer: ReturnType<typeof setInterval> | null = null;

  const completionsToday = computed(() => completions.value.length);

  function pruneRateWindow(now: number): void {
    while (eventTimes.length > 0 && now - eventTimes[0] > RATE_WINDOW_MS) eventTimes.shift();
    eventRatePerMin.value = eventTimes.length;
  }

  function pushLog(kind: LogLine['kind'], agentKey: string, text: string, at?: string): void {
    const line = makeLogLine(nextId('log'), at ?? new Date().toISOString(), kind, agentKey, text);
    logLines.value = [line, ...logLines.value].slice(0, LOG_LINES_CAP);
  }

  /** 签收绿动词行进卷宗（页面在 seal 回执到达时调用） */
  function pushSealLog(text: string): void {
    pushLog('you', '', text);
  }

  /** 签批驳回留痕：金日志（轮到人语义仍在，不欺骗——条目未被后端落账） */
  function pushGoldLog(text: string): void {
    pushLog('gold', '', text);
  }

  /**
   * decision 时间窗回填（冷开 30s 非空保底 / 刷新后『你』落点仍在的三处同源之一）。
   * 行已是 LogLine 形态（调用方按 actor 类型渲染措辞有界化）。
   */
  function backfill(lines: LogLine[], humanDoneCount: number): void {
    logLines.value = [...lines, ...logLines.value].slice(0, LOG_LINES_CAP);
    for (let i = 0; i < humanDoneCount; i += 1) {
      completions.value.push({ kind: 'decision', at: '', label: '' });
    }
  }

  function touchAgent(agentKey: string): AgentLiveState {
    let st = liveAgents.get(agentKey);
    if (!st) {
      st = newLiveState(agentKey);
      liveAgents.set(agentKey, st);
    }
    return st;
  }

  function applyStep(step: Step, envelope: V2WsEnvelope): void {
    const agentKey = (step.AuthorAgentKey ?? '').trim();
    if (!agentKey) return;
    const st = touchAgent(agentKey);
    const beat = stepToToolBeat(step);
    const isConfirm = step.Kind === 'confirm';
    const failed = envelope.kind === 'step.failed' || step.Status === 'failed';
    const done = envelope.kind === 'step.completed' || step.Status === 'completed';

    st.lastEventAt = step.UpdatedAt ?? step.CompletedAt ?? step.StartedAt ?? new Date().toISOString();
    st.statusKind = liveKindFromStep(step.Kind, step.Status, step.ToolErrorCode ?? '', isConfirm && !done && !failed);
    if (beat) {
      st.lastToolName = beat.toolName;
      st.lastDurationMs = beat.durationMs;
      st.lastErrorCode = beat.errorCode;
      st.recentTools = [beat, ...st.recentTools].slice(0, RECENT_TOOLS_CAP);
      pushLog(failed || beat.errorCode ? 'error' : 'agent', agentKey, toolBeatText(beat), beat.at || undefined);
    }
    if (step.Kind === 'thinking') {
      const raw = (step.Reasoning || step.Content || '').trim();
      if (raw) st.lastThinking = raw.slice(0, 120);
    }

    // confirm 转金：created/updated 且未了结 → 加入等批集；completed/failed → 熄灭
    if (isConfirm) {
      if (done || failed) {
        waitingAgents.delete(agentKey);
      } else {
        waitingAgents.add(agentKey);
        opts.onConfirmActivity?.();
      }
    }
  }

  function handleEnvelope(envelope: V2WsEnvelope): void {
    lastEventAt.value = new Date().toISOString();
    eventTimes.push(Date.now());
    pruneRateWindow(Date.now());

    switch (envelope.kind) {
      case 'step.created':
      case 'step.updated':
      case 'step.completed':
      case 'step.failed': {
        const step = payloadStep(envelope);
        if (step) applyStep(step, envelope);
        break;
      }
      case 'task.completed':
      case 'team_run.completed': {
        completions.value.push({
          kind: envelope.kind === 'task.completed' ? 'task' : 'team_run',
          at: lastEventAt.value,
          label: '',
        });
        break;
      }
      default:
        break;
    }
  }

  function connect(): void {
    if (consumerId) return;
    consumerId = acquireGlobalWsConsumer({
      channels: ['team', 'graph'],
      logEnabled: false,
      onV2Event: handleEnvelope,
      onConnected: () => {
        connected.value = true;
      },
      onDisconnected: () => {
        connected.value = false;
      },
    });
  }

  rateTimer = setInterval(() => pruneRateWindow(Date.now()), 10_000);
  if (globalWsHubConnected()) connected.value = true;
  connect();

  onUnmounted(() => {
    if (rateTimer) clearInterval(rateTimer);
    if (consumerId) releaseGlobalWsConsumer(consumerId);
  });

  return {
    liveAgents,
    waitingAgents,
    logLines,
    completions,
    completionsToday,
    eventRatePerMin,
    lastEventAt,
    connected,
    pushSealLog,
    pushGoldLog,
    backfill,
  };
}
