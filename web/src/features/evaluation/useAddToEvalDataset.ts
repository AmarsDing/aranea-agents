// web/src/features/evaluation/useAddToEvalDataset.ts
//
// P1-1 对话→用例一键转化：对话框状态 + 提交逻辑。
// 数据流：TaskCard 气泡菜单 → 页面 workspace.evalCase.openFromTask(task)
//   → 本 composable 预填 input/expected_output → 选数据集（或新建）
//   → evaluationStore.importCases（复用 P0-001 UploadCases 通道）。
import { ref } from 'vue';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import { useEvaluationStore } from '../../stores/evaluation';

export type AddToEvalCasePayload = {
  /** 用户消息（case input） */
  input: string;
  /** Agent 最终回复（case expected_output，可为空） */
  expected_output: string;
  /** 溯源元数据（写入 case metadata_json.source_*） */
  source_task_id?: string;
  source_session_id?: string;
};

export function useAddToEvalDataset() {
  const $q = useQuasar();
  const { t } = useI18n();
  const evaluationStore = useEvaluationStore();

  const open = ref(false);
  const submitting = ref(false);
  const datasetsLoading = ref(false);
  const datasetOptions = ref<{ label: string; value: string }[]>([]);
  /** existing = 选已有数据集；new = 内联新建 */
  const mode = ref<'existing' | 'new'>('existing');
  const datasetId = ref('');
  const newDatasetName = ref('');
  const input = ref('');
  const expectedOutput = ref('');
  /** P3-2：用例级 llm_as_judge 评分标准（写入 metadata_json.rubric）。 */
  const rubric = ref('');
  let sourceMeta: Record<string, string> = {};

  async function openWith(payload: AddToEvalCasePayload) {
    input.value = payload.input;
    expectedOutput.value = payload.expected_output;
    sourceMeta = {};
    if (payload.source_task_id) sourceMeta.task_id = payload.source_task_id;
    if (payload.source_session_id) sourceMeta.session_id = payload.source_session_id;
    mode.value = 'existing';
    datasetId.value = '';
    newDatasetName.value = '';
    rubric.value = '';
    open.value = true;
    datasetsLoading.value = true;
    try {
      const res = await evaluationStore.loadDatasets({ limit: 200 });
      datasetOptions.value = res.items.map((d) => ({
        label: t('evaluationPage.datasetOptionLabel', { name: d.name, count: d.case_count }),
        value: d.id,
      }));
      datasetId.value = res.items[0]?.id ?? '';
      if (!res.items.length) mode.value = 'new';
    } catch (e) {
      // 数据集列表加载失败不阻断：降级为新建模式。
      console.warn('[evaluation] load datasets for add-case dialog failed:', e);
      datasetOptions.value = [];
      mode.value = 'new';
    } finally {
      datasetsLoading.value = false;
    }
  }

  async function submit(): Promise<boolean> {
    const trimmedInput = input.value.trim();
    if (!trimmedInput) {
      $q.notify({ type: 'warning', message: t('evaluationPage.addCaseInputRequired') });
      return false;
    }
    let targetId = datasetId.value;
    if (mode.value === 'new') {
      const name = newDatasetName.value.trim();
      if (!name) {
        $q.notify({ type: 'warning', message: t('evaluationPage.addCaseDatasetRequired') });
        return false;
      }
      targetId = '';
    } else if (!targetId) {
      $q.notify({ type: 'warning', message: t('evaluationPage.addCaseDatasetRequired') });
      return false;
    }
    submitting.value = true;
    try {
      if (mode.value === 'new') {
        const ds = await evaluationStore.addDataset({ name: newDatasetName.value.trim() });
        targetId = ds.id;
      }
      const caseObj: Record<string, string> = {
        input: trimmedInput,
        expected_output: expectedOutput.value.trim(),
      };
      const meta: Record<string, unknown> = {};
      if (Object.keys(sourceMeta).length) {
        meta.source = 'chat';
        Object.assign(meta, sourceMeta);
      }
      const trimmedRubric = rubric.value.trim();
      if (trimmedRubric) {
        meta.rubric = trimmedRubric;
      }
      if (Object.keys(meta).length) {
        caseObj.metadata_json = JSON.stringify(meta);
      }
      const n = await evaluationStore.importCases(targetId, JSON.stringify([caseObj]));
      open.value = false;
      $q.notify({ type: 'positive', message: t('evaluationPage.addCaseOk', { n }) });
      return true;
    } catch (e) {
      $q.notify({
        type: 'negative',
        message: e instanceof Error ? e.message : t('evaluationPage.addCaseFailed'),
      });
      return false;
    } finally {
      submitting.value = false;
    }
  }

  return {
    open,
    submitting,
    datasetsLoading,
    datasetOptions,
    mode,
    datasetId,
    newDatasetName,
    input,
    expectedOutput,
    rubric,
    openWith,
    submit,
  };
}
