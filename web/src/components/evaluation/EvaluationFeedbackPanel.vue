<!-- web/src/components/evaluation/EvaluationFeedbackPanel.vue -->
<!-- P1-2 负反馈待审查列表：读取 monitor_events(chat.user_feedback, status=warning)，
     元数据自包含（input/output 快照），一键转为评估用例。 -->
<template>
  <q-card v-bind="$attrs" flat class="app-entity-glass-panel evaluation-feedback-panel">
    <q-card-section class="row items-center justify-between">
      <div>
        <div class="text-h6">{{ $t('evaluationPage.feedbackReviewTitle') }}</div>
        <div class="text-caption text-grey-7">{{ $t('evaluationPage.feedbackReviewSubtitle') }}</div>
      </div>
      <q-btn
        outline
        dense
        no-caps
        color="primary"
        icon="refresh"
        :label="$t('common.refresh')"
        :loading="loading"
        @click="onRefresh"
      />
    </q-card-section>
    <q-separator />
    <q-card-section>
      <AppRegistryTable
        :shell="false"
        :data-shell="true"
        :rows="rows"
        :columns="columns"
        row-key="id"
        :loading="loading"
        hide-pagination
        :pagination="{ rowsPerPage: 0 }"
      >
        <template #body-cell-input="props">
          <q-td :props="props">
            <span class="evaluation-feedback-panel__cell ellipsis-2-lines" :title="props.row.input">
              {{ props.row.input || '—' }}
            </span>
          </q-td>
        </template>
        <template #body-cell-output="props">
          <q-td :props="props">
            <span class="evaluation-feedback-panel__cell ellipsis-2-lines" :title="props.row.output">
              {{ props.row.output || '—' }}
            </span>
          </q-td>
        </template>
        <template #body-cell-actions="props">
          <q-td :props="props">
            <q-btn
              flat
              dense
              no-caps
              size="sm"
              color="primary"
              icon="playlist_add_check"
              :label="$t('evaluationPage.feedbackConvertToCase')"
              :disable="!props.row.input"
              @click="convert(props.row)"
            />
          </q-td>
        </template>
        <template #no-data>
          <div class="app-registry-empty app-entity-empty q-pa-md">
            <q-icon name="thumb_up" size="32px" color="grey-6" />
            <div class="text-body2">{{ $t('evaluationPage.feedbackReviewEmpty') }}</div>
          </div>
        </template>
      </AppRegistryTable>
    </q-card-section>
  </q-card>

  <AddEvalCaseDialog
    v-model:open="caseOpen"
    :mode="caseMode"
    :dataset-id="caseDatasetId"
    :dataset-options="caseDatasetOptions"
    :datasets-loading="caseDatasetsLoading"
    :new-dataset-name="caseNewDatasetName"
    :input="caseInput"
    :expected-output="caseExpectedOutput"
    :rubric="caseRubric"
    :submitting="caseSubmitting"
    @update:mode="caseMode = $event"
    @update:dataset-id="caseDatasetId = $event"
    @update:new-dataset-name="caseNewDatasetName = $event"
    @update:input="caseInput = $event"
    @update:expected-output="caseExpectedOutput = $event"
    @update:rubric="caseRubric = $event"
    @submit="submitCase()"
  />
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import AppRegistryTable from '../layout/AppRegistryTable.vue';
import AddEvalCaseDialog from './AddEvalCaseDialog.vue';
import { useAddToEvalDataset } from '../../features/evaluation/useAddToEvalDataset';
import { useFeedbackReview, type FeedbackReviewRow } from '../../features/evaluation/useFeedbackReview';

// Fragment root (card + dialog): keep parent-passed class (q-mt-md) on the card.
defineOptions({ inheritAttrs: false });

const { t } = useI18n();
const $q = useQuasar();
const { loading, loadError, rows, load } = useFeedbackReview();
const {
  open: caseOpen,
  submitting: caseSubmitting,
  datasetsLoading: caseDatasetsLoading,
  datasetOptions: caseDatasetOptions,
  mode: caseMode,
  datasetId: caseDatasetId,
  newDatasetName: caseNewDatasetName,
  input: caseInput,
  expectedOutput: caseExpectedOutput,
  rubric: caseRubric,
  openWith,
  submit: submitCase,
} = useAddToEvalDataset();

const columns = computed(() => [
  { name: 'time', label: t('evaluationPage.feedbackColTime'), field: 'time', align: 'left' as const },
  { name: 'input', label: t('evaluationPage.feedbackColInput'), field: 'input', align: 'left' as const },
  { name: 'output', label: t('evaluationPage.feedbackColOutput'), field: 'output', align: 'left' as const },
  { name: 'comment', label: t('evaluationPage.feedbackColComment'), field: 'comment', align: 'left' as const },
  { name: 'actions', label: t('common.actions'), field: 'actions', align: 'right' as const },
]);

async function onRefresh() {
  await load();
  if (loadError.value) {
    $q.notify({ type: 'negative', message: t('evaluationPage.feedbackLoadFailed') });
  }
}

function convert(row: FeedbackReviewRow) {
  void openWith({
    input: row.input,
    expected_output: row.output,
    source_task_id: row.task_id || undefined,
    source_session_id: row.session_id || undefined,
  });
}

onMounted(onRefresh);
</script>

<style lang="sass" scoped>
.evaluation-feedback-panel__cell
  display: inline-block
  max-width: 320px
  vertical-align: middle
</style>
