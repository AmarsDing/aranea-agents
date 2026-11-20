/**
 * 流程健康页（/ceo/oncall）的运维两块：死信（可重放）与 NATS 消费者积压。
 * 后端早有接口（ListDeadLetters / ReplayDeadLetter 需 ceoboard.ops，ListConsumers 需 ceoboard.view），此前没有界面。
 * 页签切到这一块时才读；没权限时如实说明，不当成故障。
 */
import { computed, ref } from 'vue';
import { createCeoboardService } from '../../services';
import { isForbiddenApiError } from '../../utils/kratosError';
import type { DeadLetter } from './types';

export type FlowConsumer = {
  key: string;
  node: string;
  platform: string;
  stream: string;
  durable: string;
  subject: string;
  pending: number;
  connected: boolean;
  as_of: string;
  error: string;
};

type LoadState = { loading: boolean; error: string; forbidden: boolean };

function message(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

export function useFlowOps() {
  const svc = createCeoboardService();
  const deadLetters = ref<DeadLetter[]>([]);
  const deadPending = ref(0);
  const deadState = ref<LoadState>({ loading: false, error: '', forbidden: false });
  const consumers = ref<FlowConsumer[]>([]);
  const consumerState = ref<LoadState>({ loading: false, error: '', forbidden: false });
  const replaying = ref<number | null>(null);

  async function loadDeadLetters() {
    deadState.value = { ...deadState.value, loading: true };
    try {
      const res = await svc.ListDeadLetters({ limit: 200 });
      deadPending.value = Number(res.pending ?? 0);
      deadLetters.value = (res.items ?? []).map((d) => ({
        id: Number(d.id ?? 0),
        kind: d.kind ?? '',
        source: d.source ?? '',
        ref_key: d.refKey ?? '',
        payload: d.payload ?? '',
        reason: d.reason ?? '',
        failed_at: String(d.failedAt ?? ''),
        replayed_at: d.replayedAt ? String(d.replayedAt) : undefined,
      }));
      deadState.value = { loading: false, error: '', forbidden: false };
    } catch (err) {
      deadState.value = { loading: false, error: message(err), forbidden: isForbiddenApiError(err) };
    }
  }

  async function loadConsumers() {
    consumerState.value = { ...consumerState.value, loading: true };
    try {
      const res = await svc.ListConsumers({ platform: undefined, workflowId: undefined, nodeId: undefined });
      consumers.value = (res.items ?? [])
        .map((c) => ({
          key: `${c.stream ?? ''}/${c.durable ?? ''}`,
          node: c.nodeKey ?? '',
          platform: c.platform ?? '',
          stream: c.stream ?? '',
          durable: c.durable ?? '',
          subject: c.subject ?? '',
          pending: Number(c.numPending ?? 0),
          connected: Boolean(c.connected),
          as_of: String(c.asOf ?? ''),
          error: c.sampleError ?? '',
        }))
        // 断开的、积压多的排前面
        .sort((a, b) => Number(a.connected) - Number(b.connected) || b.pending - a.pending);
      consumerState.value = { loading: false, error: '', forbidden: false };
    } catch (err) {
      consumerState.value = { loading: false, error: message(err), forbidden: isForbiddenApiError(err) };
    }
  }

  /** 重放一条死信；返回给页面一句结果（已重放 / 已存在未重复投递）。 */
  async function replay(id: number): Promise<{ ok: boolean; deduped: boolean; error?: string }> {
    replaying.value = id;
    try {
      const res = await svc.ReplayDeadLetter({ id });
      await loadDeadLetters();
      return { ok: Boolean(res.replayed), deduped: Boolean(res.deduped) };
    } catch (err) {
      return { ok: false, deduped: false, error: message(err) };
    } finally {
      replaying.value = null;
    }
  }

  const backlog = computed(() => consumers.value.reduce((sum, c) => sum + c.pending, 0));
  const disconnected = computed(() => consumers.value.filter((c) => !c.connected).length);

  return {
    deadLetters,
    deadPending,
    deadState,
    consumers,
    consumerState,
    backlog,
    disconnected,
    replaying,
    loadDeadLetters,
    loadConsumers,
    replay,
  };
}
