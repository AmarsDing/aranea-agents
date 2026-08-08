// web/src/features/evaluation/useFeedbackReview.ts
// P1-2 负反馈待审查：读取 monitor_events(chat.user_feedback, status=warning)，
// 元数据自包含（input/output 快照由反馈写入侧落库，任务删除后仍可转用例）。
import { ref } from 'vue';
import { useMonitorStore } from '../../stores/monitor';

/** 负反馈审查行：从 monitor event metadata_json 解析的自包含快照。 */
export type FeedbackReviewRow = {
  id: string;
  time: string;
  session_id: string;
  task_id: string;
  input: string;
  output: string;
  comment: string;
};

function parseMetadata(raw: string): Record<string, unknown> {
  try {
    const parsed: unknown = JSON.parse(raw || '{}');
    return parsed && typeof parsed === 'object' ? (parsed as Record<string, unknown>) : {};
  } catch {
    return {};
  }
}

export function useFeedbackReview() {
  const monitorStore = useMonitorStore();
  const loading = ref(false);
  const loadError = ref(false);
  const rows = ref<FeedbackReviewRow[]>([]);

  async function load() {
    loading.value = true;
    loadError.value = false;
    try {
      const res = await monitorStore.fetchMonitorEvents({
        event_types: ['chat.user_feedback'],
        status: 'warning',
        limit: 50,
      });
      rows.value = res.items.map((item) => {
        const meta = parseMetadata(item.metadata_json);
        return {
          id: item.id,
          time: item.created_at ? new Date(item.created_at).toLocaleString() : '',
          session_id: String(meta.session_id ?? ''),
          task_id: String(meta.task_id ?? ''),
          input: String(meta.input ?? ''),
          output: String(meta.output ?? ''),
          comment: String(meta.comment ?? ''),
        };
      });
    } catch (e) {
      loadError.value = true;
      console.warn('[evaluation] load feedback review list failed:', e);
    } finally {
      loading.value = false;
    }
  }

  return { loading, loadError, rows, load };
}
