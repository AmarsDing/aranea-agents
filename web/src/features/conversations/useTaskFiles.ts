// 任务的资料：「你给的」= 资料袋会话里的文件；「交出来的」= 各次执行（本平台会话）的产物，按执行先后。
// 同一份文件（sha256 相同）只算在第一次出现的地方——交给智能体时复制过去的资料不会被当成它交出来的。
import { ref, watch, type Ref } from 'vue';
import { listArtifacts } from '../artifact/api';
import type { ArtifactMeta } from '../artifact/types';
import { flat, type ConvTask, type Execution } from './model';

export interface ExecOutputs {
  exec: Execution;
  files: ArtifactMeta[];
}

const PER_SESSION = 100;

export function useTaskFiles(task: Ref<ConvTask | undefined>) {
  const inputs = ref<ArtifactMeta[]>([]);
  const outputs = ref<ExecOutputs[]>([]);
  const loading = ref(false);
  const failed = ref(false);
  let seq = 0;

  async function load() {
    const t = task.value;
    const mine = ++seq;
    if (!t) {
      inputs.value = [];
      outputs.value = [];
      return;
    }
    loading.value = true;
    const sessions = flat(t).filter((e) => e.kind === 'session');
    const ids = [t.recordId ?? '', ...sessions.map((e) => e.refId)];
    const lists = await Promise.allSettled(
      ids.map((id) => (id ? listArtifacts({ session_id: id, limit: PER_SESSION }) : Promise.resolve({ items: [] }))),
    );
    if (mine !== seq) return;
    failed.value = lists.some((r) => r.status === 'rejected');
    const got = lists.map((r) => (r.status === 'fulfilled' ? r.value.items : []));
    const seen = new Set<string>();
    const fresh = (xs: ArtifactMeta[]) =>
      [...xs]
        .sort((a, b) => a.created_at.localeCompare(b.created_at))
        .filter((a) => {
          const k = a.sha256 || a.id;
          if (seen.has(k)) return false;
          seen.add(k);
          return true;
        });
    inputs.value = fresh(got[0] ?? []);
    outputs.value = sessions.map((exec, i) => ({ exec, files: fresh(got[i + 1] ?? []) }));
    loading.value = false;
  }

  // 换任务、执行数变了（交给了下一位）才重读
  watch(
    () => (task.value ? `${task.value.id}|${flat(task.value).length}` : ''),
    () => void load(),
    { immediate: true },
  );

  return { inputs, outputs, loading, failed, reload: load };
}
