<template>
  <AppRegistryTable
    table-class="skill-runs-table"
    row-key="id"
    :rows="rows"
    :columns="columns"
    :loading="loading"
    :error="error"
    :empty-title="$t('shell.runs.emptyTitle')"
    :empty-description="$t('shell.runs.emptyDesc')"
    empty-icon="history"
    :empty-action-label="$t('shell.runs.emptyAction')"
    empty-action-icon="restart_alt"
    hide-pagination
    :pagination="{ rowsPerPage: 0 }"
    @row-click="onRowClick"
    @retry="$emit('retry')"
    @empty-action="$emit('reset')"
  >
    <template #body-cell-time="props">
      <q-td :props="props">
        <div>{{ formatDate(props.row.started_at) }}</div>
        <div class="text-caption text-grey-7">{{ formatDuration(props.row.duration_ms) }}</div>
      </q-td>
    </template>
    <template #body-cell-skill="props">
      <q-td :props="props">
        <AppRegistryHoverTip :text="props.row.input_preview" :empty-label="$t('shell.runs.noInput')">
          <div class="min-width-0">
            <div class="app-registry-cell-primary">{{ skillDisplay(props.row) }}</div>
            <!-- 版本为空时不渲染 unknown 徽章（UI-5）：空值徽章是纯视觉噪音 -->
            <q-chip v-if="props.row.skill_version" dense size="sm" color="primary" text-color="white">
              {{ props.row.skill_version }}
            </q-chip>
          </div>
        </AppRegistryHoverTip>
      </q-td>
    </template>
    <template #body-cell-agent="props">
      <q-td :props="props">{{ props.row.agent_display_name || props.row.agent_id || '-' }}</q-td>
    </template>
    <template #body-cell-status="props">
      <q-td :props="props">
        <AppRegistryHoverTip
          :text="props.row.status === 'success' ? props.row.output_preview : props.row.error_message"
          :empty-label="$t('shell.runs.noOutput')"
        >
          <AppStatusChip :status="props.row.status === 'pending' ? 'running' : props.row.status" />
        </AppRegistryHoverTip>
      </q-td>
    </template>
  </AppRegistryTable>

  <!-- FN-3：行点击查看运行详情（错误信息可发现、可复制） -->
  <AppDialog v-model="detailOpen" :title="$t('skillsPage.runDetailTitle')">
    <template #header-prefix>
      <AppStatusChip
        v-if="detailRow"
        :status="detailRow.status === 'pending' ? 'running' : detailRow.status"
      />
    </template>
    <div v-if="detailRow">
      <div class="skill-run-detail-grid">
        <div class="text-caption text-grey-7">Skill</div>
        <div>{{ skillDisplay(detailRow) }}</div>
        <div class="text-caption text-grey-7">{{ $t('skillsPage.runDetailVersion') }}</div>
        <div>{{ detailRow.skill_version || '-' }}</div>
        <div class="text-caption text-grey-7">Agent</div>
        <div>{{ detailRow.agent_display_name || detailRow.agent_id || '-' }}</div>
        <div class="text-caption text-grey-7">{{ $t('skillsPage.runDetailStartedAt') }}</div>
        <div>
          {{ formatDate(detailRow.started_at)
          }}{{ $t('skillsPage.runDetailDuration', { duration: formatDuration(detailRow.duration_ms) }) }}
        </div>
        <div v-if="detailRow.session_id" class="text-caption text-grey-7">
          {{ $t('skillsPage.runDetailSession') }}
        </div>
        <div v-if="detailRow.session_id" class="text-mono">{{ detailRow.session_id }}</div>
        <div v-if="detailRow.source" class="text-caption text-grey-7">{{ $t('skillsPage.runDetailSource') }}</div>
        <div v-if="detailRow.source">{{ detailRow.source }}</div>
        <template v-if="detailRow.status !== 'success' && detailRow.error_code">
          <div class="text-caption text-grey-7">{{ $t('skillsPage.runDetailErrorCode') }}</div>
          <div class="text-negative text-mono">{{ detailRow.error_code }}</div>
        </template>
      </div>

      <template v-if="detailRow.status !== 'success' && detailRow.error_message">
        <div class="text-caption text-grey-7 q-mt-md q-mb-xs">{{ $t('skillsPage.runDetailErrorMessage') }}</div>
        <q-input
          :model-value="detailRow.error_message"
          type="textarea"
          readonly
          outlined
          dense
          autogrow
          class="skill-run-detail-textarea"
        />
      </template>

      <div class="text-caption text-grey-7 q-mt-md q-mb-xs">{{ $t('skillsPage.runDetailInputPreview') }}</div>
      <q-input
        :model-value="detailRow.input_preview || $t('skillsPage.runDetailInputEmpty')"
        type="textarea"
        readonly
        outlined
        dense
        autogrow
        class="skill-run-detail-textarea"
      />

      <div v-if="detailRow.status === 'success'" class="text-caption text-grey-7 q-mt-md q-mb-xs">
        {{ $t('skillsPage.runDetailOutputPreview') }}
      </div>
      <q-input
        v-if="detailRow.status === 'success'"
        :model-value="detailRow.output_preview || $t('skillsPage.runDetailOutputEmpty')"
        type="textarea"
        readonly
        outlined
        dense
        autogrow
        class="skill-run-detail-textarea"
      />
    </div>
  </AppDialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import AppRegistryTable from '../layout/AppRegistryTable.vue';
import AppRegistryHoverTip from '../layout/AppRegistryHoverTip.vue';
import AppStatusChip from '../common/AppStatusChip.vue';
import AppDialog from '../common/AppDialog.vue';
import type { SkillInvocation } from '../../features/skills/types';
import { createSkillRunsTableColumns } from './skillTableUi';

defineProps<{
  rows: SkillInvocation[];
  loading: boolean;
  error?: string;
}>();

defineEmits<{
  retry: [];
  reset: [];
}>();

const { t } = useI18n();
const columns = computed(() => createSkillRunsTableColumns(t));

const detailOpen = ref(false);
const detailRow = ref<SkillInvocation | null>(null);

function onRowClick(_evt: unknown, row: SkillInvocation) {
  if (!row.permissions?.can_view_detail) return;
  detailRow.value = row;
  detailOpen.value = true;
}

/** FN-5：失败记录 skillId/skillName 可能为空（如 filesystem_reconcile），
 * 主显示位 fallback 到输入摘要，避免整行只剩徽章。 */
function skillDisplay(row: SkillInvocation): string {
  const name = row.skill_name || row.skill_id;
  if (name) return name;
  const preview = (row.input_preview ?? '').trim();
  if (!preview) return '-';
  return preview.length > 40 ? preview.slice(0, 40) + '…' : preview;
}

function formatDate(value?: string) {
  if (!value) return '-';
  return new Date(value).toLocaleString();
}

function formatDuration(value: number) {
  if (value < 1000) return `${Math.round(value)}ms`;
  return `${(value / 1000).toFixed(1)}s`;
}
</script>

<style scoped>
.skill-run-detail-grid {
  display: grid;
  grid-template-columns: 88px 1fr;
  gap: 6px 12px;
  align-items: baseline;
}

.text-mono {
  font-family: ui-monospace, monospace;
  word-break: break-all;
}

.skill-run-detail-textarea :deep(textarea) {
  font-size: 12px;
}
</style>
