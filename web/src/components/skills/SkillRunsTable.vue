<template>
  <AppRegistryTable
    table-class="skill-runs-table"
    row-key="id"
    :rows="rows"
    :columns="SKILL_RUNS_TABLE_COLUMNS"
    :loading="loading"
    hide-pagination
    @row-click="onRowClick"
  >
    <template #body-cell-time="props">
      <q-td :props="props">
        <div>{{ formatDate(props.row.started_at) }}</div>
        <div class="text-caption text-grey-7">{{ formatDuration(props.row.duration_ms) }}</div>
      </q-td>
    </template>
    <template #body-cell-skill="props">
      <q-td :props="props">
        <AppRegistryHoverTip :text="props.row.input_preview" empty-label="无输入摘要">
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
          empty-label="无输出摘要"
        >
          <q-badge rounded :color="props.row.status === 'success' ? 'positive' : 'negative'">
            {{ props.row.status === 'success' ? '成功' : '失败' }}
          </q-badge>
        </AppRegistryHoverTip>
      </q-td>
    </template>
  </AppRegistryTable>

  <!-- FN-3：行点击查看运行详情（错误信息可发现、可复制） -->
  <q-dialog v-model="detailOpen">
    <q-card v-if="detailRow" class="app-dialog-card skill-run-detail-card">
      <q-card-section class="row items-center q-pb-sm">
        <div class="text-h6">运行详情</div>
        <q-space />
        <q-badge rounded :color="detailRow.status === 'success' ? 'positive' : 'negative'">
          {{ detailRow.status === 'success' ? '成功' : '失败' }}
        </q-badge>
        <q-btn v-close-popup flat round dense icon="close" class="q-ml-sm" />
      </q-card-section>

      <q-card-section class="q-pt-none">
        <div class="skill-run-detail-grid">
          <div class="text-caption text-grey-7">Skill</div>
          <div>{{ skillDisplay(detailRow) }}</div>
          <div class="text-caption text-grey-7">版本</div>
          <div>{{ detailRow.skill_version || '-' }}</div>
          <div class="text-caption text-grey-7">Agent</div>
          <div>{{ detailRow.agent_display_name || detailRow.agent_id || '-' }}</div>
          <div class="text-caption text-grey-7">开始时间</div>
          <div>{{ formatDate(detailRow.started_at) }}（耗时 {{ formatDuration(detailRow.duration_ms) }}）</div>
          <div v-if="detailRow.session_id" class="text-caption text-grey-7">会话</div>
          <div v-if="detailRow.session_id" class="text-mono">{{ detailRow.session_id }}</div>
          <div v-if="detailRow.source" class="text-caption text-grey-7">来源</div>
          <div v-if="detailRow.source">{{ detailRow.source }}</div>
          <template v-if="detailRow.status !== 'success' && detailRow.error_code">
            <div class="text-caption text-grey-7">错误码</div>
            <div class="text-negative text-mono">{{ detailRow.error_code }}</div>
          </template>
        </div>

        <template v-if="detailRow.status !== 'success' && detailRow.error_message">
          <div class="text-caption text-grey-7 q-mt-md q-mb-xs">错误信息</div>
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

        <div class="text-caption text-grey-7 q-mt-md q-mb-xs">输入摘要</div>
        <q-input
          :model-value="detailRow.input_preview || '无输入摘要'"
          type="textarea"
          readonly
          outlined
          dense
          autogrow
          class="skill-run-detail-textarea"
        />

        <div v-if="detailRow.status === 'success'" class="text-caption text-grey-7 q-mt-md q-mb-xs">输出摘要</div>
        <q-input
          v-if="detailRow.status === 'success'"
          :model-value="detailRow.output_preview || '无输出摘要'"
          type="textarea"
          readonly
          outlined
          dense
          autogrow
          class="skill-run-detail-textarea"
        />
      </q-card-section>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import AppRegistryTable from '../layout/AppRegistryTable.vue';
import AppRegistryHoverTip from '../layout/AppRegistryHoverTip.vue';
import type { SkillInvocation } from '../../features/skills/types';
import { SKILL_RUNS_TABLE_COLUMNS } from './skillTableUi';

defineProps<{
  rows: SkillInvocation[];
  loading: boolean;
}>();

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
.skill-run-detail-card {
  min-width: min(560px, 92vw);
}

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
