<template>
  <AppRegistryTable
    table-class="tools-data-table"
    row-key="id"
    :rows="rows"
    :columns="columns"
    :loading="loading"
    :pagination="tablePagination"
    :selected="selected"
    selection="multiple"
    hide-pagination
    @update:selected="$emit('update:selected', $event)"
  >
    <template #body-cell-name="props">
      <q-td :props="props">
        <div class="app-registry-cell-primary">{{ props.row.display_name }}</div>
        <div class="app-registry-cell-sub app-registry-muted-caption">{{ props.row.key }}</div>
      </q-td>
    </template>

    <template #body-cell-category="props">
      <q-td :props="props">
        <q-chip dense color="primary" text-color="white">{{ props.row.category || 'custom' }}</q-chip>
        <q-chip dense outline class="q-ml-xs app-registry-source-chip">{{ props.row.source || 'external' }}</q-chip>
      </q-td>
    </template>

    <template #body-cell-risk="props">
      <q-td :props="props">
        <div class="row items-center no-wrap">
          <q-icon name="circle" size="8px" :color="riskQuasarColor(props.row.risk_level)" class="q-mr-xs" />
          <q-select
            dense
            outlined
            emit-value
            map-options
            :model-value="props.row.risk_level"
            :options="riskLevelOptions()"
            class="tool-risk-inline-select"
            :loading="busyId === props.row.id"
            @update:model-value="$emit('updateRisk', props.row, String($event ?? 'low'))"
          >
            <template #option="scope">
              <q-item v-bind="scope.itemProps">
                <q-item-section avatar class="tool-risk-option-dot">
                  <q-icon name="circle" size="8px" :color="riskQuasarColor(scope.opt.value)" />
                </q-item-section>
                <q-item-section>{{ scope.opt.label }}</q-item-section>
              </q-item>
            </template>
          </q-select>
        </div>
        <q-badge v-if="props.row.requires_confirmation" rounded color="warning" class="q-ml-xs">
          {{ policyChip.requires_confirmation.label }}
          <q-tooltip>{{ policyChip.requires_confirmation.tooltip }}</q-tooltip>
        </q-badge>
      </q-td>
    </template>

    <template #body-cell-runtime="props">
      <q-td :props="props">
        <AppStatusChip :status="props.row.runtime_status" />
        <div class="text-caption app-registry-muted-caption q-mt-xs">{{ runtimeKindHint(props.row) }}</div>
      </q-td>
    </template>

    <template #body-cell-enabled="props">
      <q-td :props="props">
        <q-toggle
          dense
          color="primary"
          :model-value="props.row.enabled"
          :disable="busyId === props.row.id"
          @update:model-value="$emit('toggleEnabled', props.row, Boolean($event))"
        />
      </q-td>
    </template>

    <template #body-cell-overrides="props">
      <q-td :props="props">
        <q-badge
          rounded
          :color="props.row.agent_override_count > 0 ? 'primary' : 'grey'"
          :outline="props.row.agent_override_count <= 0"
        >
          {{ props.row.agent_override_count }}
        </q-badge>
        <q-tooltip>{{ $t('toolsPage.table.overridesTip') }}</q-tooltip>
      </q-td>
    </template>

    <template #body-cell-stats="props">
      <q-td :props="props">
        <div class="text-weight-medium">{{ $t('toolsPage.table.invokeTimes', { count: props.row.invoke_count }) }}</div>
        <div class="text-caption app-registry-muted-caption">24h {{ props.row.invoke_count_24h }}</div>
      </q-td>
    </template>

    <template #body-cell-success_rate="props">
      <q-td :props="props">
        <div class="text-weight-medium" :class="`text-${toolSuccessRateColor(props.row)}`">
          {{ formatToolSuccessRate(props.row) }}
        </div>
        <div class="text-caption app-registry-muted-caption">
          {{ $t('toolsPage.table.successFailLine', { success: props.row.success_count, failure: props.row.failure_count + props.row.blocked_count }) }}
        </div>
        <div v-if="props.row.invoke_count > 0" class="text-caption" :class="`text-${toolArgsFirstPassRateColor(props.row)}`">
          {{ $t('toolsPage.argsQuality.firstPassShort', { rate: formatToolArgsFirstPassRate(props.row) }) }}
          <q-tooltip>
            {{ $t('toolsPage.argsQuality.firstPassTip', { repaired: props.row.repaired_count, invalid: props.row.invalid_count, invoke: props.row.invoke_count }) }}
          </q-tooltip>
        </div>
      </q-td>
    </template>

    <template #body-cell-duration="props">
      <q-td :props="props">
        <div>avg {{ formatInvocationDuration(props.row.avg_duration_ms ?? undefined) }}</div>
        <div class="text-caption app-registry-muted-caption">
          P95 {{ formatInvocationDuration(props.row.p95_duration_ms) }}
        </div>
      </q-td>
    </template>

    <template #body-cell-last="props">
      <q-td :props="props">
        <template v-if="props.row.last_invoked_at">
          <div class="row items-center no-wrap q-gutter-xs">
            <AppStatusChip :status="props.row.last_status" />
          </div>
          <div class="text-caption app-registry-muted-caption q-mt-xs">
            {{ formatInvocationWhen(props.row.last_invoked_at) }}
          </div>
        </template>
        <span v-else class="app-registry-muted-caption">{{ $t('toolsPage.table.notInvoked') }}</span>
      </q-td>
    </template>

    <template #body-cell-actions="props">
      <q-td :props="props">
        <div class="app-registry-cell-actions">
          <q-btn
            flat
            dense
            round
            class="app-registry-icon-btn"
            color="primary"
            icon="visibility"
            :aria-label="$t('toolsPage.table.view')"
            @click="$emit('viewDetail', props.row)"
          >
            <q-tooltip>{{ $t('toolsPage.table.view') }}</q-tooltip>
          </q-btn>
          <q-btn
            flat
            dense
            round
            class="app-registry-icon-btn"
            color="primary"
            icon="edit"
            :aria-label="$t('toolsPage.table.edit')"
            @click="$emit('edit', props.row)"
          >
            <q-tooltip>{{ $t('toolsPage.table.edit') }}</q-tooltip>
          </q-btn>
          <q-btn
            flat
            dense
            round
            class="app-registry-icon-btn"
            color="negative"
            icon="delete"
            :aria-label="$t('toolsPage.table.remove')"
            :disable="props.row.readonly"
            :loading="busyId === props.row.id"
            @click="$emit('remove', props.row)"
          >
            <q-tooltip>{{ props.row.readonly ? $t('toolsPage.table.readonlyNoRemove') : $t('toolsPage.table.remove') }}</q-tooltip>
          </q-btn>
        </div>
      </q-td>
    </template>
  </AppRegistryTable>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import AppRegistryTable from '../layout/AppRegistryTable.vue';
import AppStatusChip from '../common/AppStatusChip.vue';
import type { Tool } from '../../features/tools/types';
import { toolPolicyChipCopy } from '../../features/tools/toolEditorCopy';
import {
  toolTableColumns,
  formatInvocationDuration,
  formatInvocationWhen,
  formatToolSuccessRate,
  formatToolArgsFirstPassRate,
  riskLevelOptions,
  riskQuasarColor,
  runtimeKindHint,
  toolSuccessRateColor,
  toolArgsFirstPassRateColor,
} from './toolUi';

defineProps<{
  rows: Tool[];
  loading: boolean;
  busyId: string;
  selected?: Tool[];
}>();

defineEmits<{
  toggleEnabled: [tool: Tool, value: boolean];
  updateRisk: [tool: Tool, value: string];
  viewDetail: [tool: Tool];
  edit: [tool: Tool];
  remove: [tool: Tool];
  'update:selected': [value: Tool[]];
}>();

const tablePagination = { rowsPerPage: 0 };
const columns = computed(() => toolTableColumns());
const policyChip = computed(() => toolPolicyChipCopy());
</script>

<style scoped>
/* 风险下拉选项的色点列：压缩 Quasar avatar 默认 56px 最小宽度 */
.tool-risk-option-dot {
  min-width: 16px;
  padding-right: 4px;
}
</style>
