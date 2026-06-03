// Container: approved — feature-local panel; data from Page composable via props.
<template>
  <q-card flat bordered class="memory-card">
    <q-card-section class="row items-center q-col-gutter-md">
      <div class="col">
        <div class="text-h6">Cascade 审核</div>
        <div class="text-body2 text-grey-7">L4 实体更名冲突门控：预览影响 → 确认执行。支持 Saga 步骤追踪与补偿。</div>
      </div>
      <div class="col-auto">
        <q-btn flat icon="refresh" label="刷新" :loading="loading" @click="$emit('refresh')" />
      </div>
    </q-card-section>

    <q-card-section v-if="!agentId" class="text-grey-7">请先选择 Agent。</q-card-section>

    <q-card-section v-else-if="!rows.length && !loading">
      <q-banner rounded class="memory-info-banner">暂无 Cascade 提议。</q-banner>
    </q-card-section>

    <q-card-section v-else class="q-pt-none">
      <AppRegistryTable
        :shell="false"
        :rows="rows"
        :columns="CASCADE_SAGA_TABLE_COLUMNS"
        row-key="id"
        :loading="loading"
        hide-pagination
        :pagination="{ rowsPerPage: 0 }"
      >
        <template #body-cell-change="props">
          <q-td :props="props">
            <AppRegistryHoverTip :text="props.row.rationale">
              <div class="min-width-0">
                <div class="app-registry-cell-primary">{{ props.row.old_value }} → {{ props.row.new_value }}</div>
                <div class="app-registry-cell-sub ellipsis">{{ props.row.trigger_entity_name }}</div>
              </div>
            </AppRegistryHoverTip>
          </q-td>
        </template>
        <template #body-cell-status="props">
          <q-td :props="props">
            <q-badge :color="statusColor(props.row.status)" :label="props.row.status" />
          </q-td>
        </template>
        <template #body-cell-risk="props">
          <q-td :props="props">
            <q-badge :color="riskColor(props.row.risk_level)" :label="props.row.risk_level || 'unknown'" />
          </q-td>
        </template>
        <template #body-cell-affected="props">
          <q-td :props="props">
            <q-chip dense square color="blue-grey" text-color="white">{{
              props.row.affected_entities?.length ?? 0
            }}</q-chip>
          </q-td>
        </template>
        <template #body-cell-actions="props">
          <q-td :props="props">
            <div class="app-registry-cell-actions">
              <q-btn
                v-if="props.row.status === 'pending'"
                dense
                flat
                round
                color="info"
                icon="visibility"
                @click="$emit('preview', props.row)"
              >
                <q-tooltip>Dry-Run 预览</q-tooltip>
              </q-btn>
              <q-btn
                v-if="props.row.status === 'pending'"
                dense
                flat
                round
                color="positive"
                icon="check"
                :loading="actingId === props.row.id"
                @click="$emit('approve', props.row)"
              >
                <q-tooltip>批准</q-tooltip>
              </q-btn>
              <q-btn
                v-if="props.row.status === 'pending'"
                dense
                flat
                round
                color="negative"
                icon="close"
                :loading="actingId === props.row.id"
                @click="$emit('reject', props.row)"
              >
                <q-tooltip>拒绝</q-tooltip>
              </q-btn>
              <q-btn
                v-if="props.row.status === 'partial'"
                dense
                flat
                round
                color="warning"
                icon="replay"
                :loading="actingId === props.row.id"
                @click="$emit('retry', props.row)"
              >
                <q-tooltip>重试</q-tooltip>
              </q-btn>
              <q-btn
                v-if="props.row.status === 'partial' || props.row.status === 'failed'"
                dense
                flat
                round
                color="deep-orange"
                icon="undo"
                :loading="actingId === props.row.id"
                @click="$emit('compensate', props.row)"
              >
                <q-tooltip>补偿回滚</q-tooltip>
              </q-btn>
              <q-btn
                v-if="props.row.status !== 'pending'"
                dense
                flat
                round
                icon="account_tree"
                @click="$emit('saga', props.row)"
              >
                <q-tooltip>Saga 步骤</q-tooltip>
              </q-btn>
            </div>
          </q-td>
        </template>
      </AppRegistryTable>
    </q-card-section>

    <q-dialog v-model="previewOpen" persistent>
      <q-card class="app-dialog-card app-dialog-card--lg app-glass-dialog">
        <q-card-section class="app-glass-dialog__head row items-start justify-between no-wrap">
          <div class="col min-width-0">
            <div class="app-glass-dialog__title">Dry-Run 预览</div>
            <div class="app-glass-dialog__subtitle">只读预览批准后的变更，不会实际执行。</div>
          </div>
          <q-btn v-close-popup flat dense round icon="close" />
        </q-card-section>
        <q-separator />
        <div class="app-glass-dialog__scroll">
          <q-card-section class="app-dialog-body app-glass-dialog__body">
            <div v-if="previewLoading" class="row justify-center q-py-lg">
              <q-spinner-dots size="32px" color="primary" />
            </div>
            <template v-else-if="preview">
              <div class="row q-col-gutter-md q-mb-md">
                <div class="col-6">
                  <div class="text-caption text-grey-7">受影响实体</div>
                  <div class="text-h6">{{ preview.affected_entities_count }}</div>
                </div>
                <div class="col-6">
                  <div class="text-caption text-grey-7">受影响 Facts</div>
                  <div class="text-h6">{{ preview.affected_facts_count }}</div>
                </div>
              </div>

              <div v-if="preview.entity_renames.length" class="q-mb-md">
                <div class="text-subtitle2 q-mb-sm">实体更名</div>
                <q-list dense bordered class="rounded-borders" separator>
                  <q-item v-for="r in preview.entity_renames" :key="r.entity_id">
                    <q-item-section>
                      <q-item-label>
                        <span class="text-grey-7">{{ r.entity_type }}</span>
                        <span class="q-mx-xs">·</span>
                        <span>{{ r.old_name }}</span>
                        <span class="q-mx-xs text-grey-7">→</span>
                        <span class="text-weight-medium">{{ r.new_name }}</span>
                      </q-item-label>
                    </q-item-section>
                  </q-item>
                </q-list>
              </div>

              <div v-if="preview.fact_diffs.length">
                <div class="text-subtitle2 q-mb-sm">Fact 语句变更</div>
                <q-list dense bordered class="rounded-borders" separator>
                  <q-item v-for="d in preview.fact_diffs" :key="d.fact_id">
                    <q-item-section>
                      <q-item-label caption>{{ d.scope }} · {{ d.fact_id.slice(0, 8) }}</q-item-label>
                      <q-item-label class="text-grey-7" style="text-decoration: line-through">{{
                        d.before_statement
                      }}</q-item-label>
                      <q-item-label class="text-weight-medium">{{ d.after_statement }}</q-item-label>
                    </q-item-section>
                  </q-item>
                </q-list>
              </div>

              <q-banner
                v-if="!preview.entity_renames.length && !preview.fact_diffs.length"
                rounded
                class="memory-info-banner q-mt-sm"
              >
                预览无变更，批准操作不会产生副作用。
              </q-banner>
            </template>
          </q-card-section>
        </div>
        <q-separator />
        <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions">
          <q-btn v-close-popup flat label="取消" />
          <q-btn
            unelevated
            label="确认批准"
            color="primary"
            :loading="actingId === previewProposalId"
            @click="onConfirmFromPreview"
          />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <q-drawer v-model="sagaDrawerOpen" side="right" :width="480" overlay bordered class="memory-drawer">
      <div class="q-pa-md">
        <div class="row items-center justify-between q-mb-md">
          <div class="text-h6">Saga 步骤</div>
          <q-btn flat dense round icon="close" @click="sagaDrawerOpen = false" />
        </div>

        <div v-if="sagaLoading" class="row justify-center q-py-lg">
          <q-spinner-dots size="32px" color="primary" />
        </div>

        <q-list v-else-if="sagaSteps.length" separator>
          <q-item v-for="step in sagaSteps" :key="step.id" class="q-px-sm">
            <q-item-section avatar>
              <q-icon :name="stepIcon(step.state)" :color="stepColor(step.state)" size="sm" />
            </q-item-section>
            <q-item-section>
              <q-item-label>
                <span class="text-weight-medium">{{ stepDisplayName(step.step_name) }}</span>
                <q-badge v-if="step.is_critical" color="deep-orange" label="关键" class="q-ml-xs" />
              </q-item-label>
              <q-item-label caption> 状态: {{ step.state }} · 尝试: {{ step.attempts }} </q-item-label>
              <q-item-label v-if="step.started_at" caption>
                {{ step.started_at ? formatStepTime(step.started_at) : '' }}
                <template v-if="step.finished_at"> → {{ formatStepTime(step.finished_at) }}</template>
              </q-item-label>
              <q-item-label v-if="step.error" class="text-negative">
                {{ step.error }}
              </q-item-label>
            </q-item-section>
          </q-item>
        </q-list>

        <q-banner v-else rounded class="memory-info-banner">暂无 Saga 步骤记录。</q-banner>
      </div>
    </q-drawer>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import AppRegistryTable from '../../components/layout/AppRegistryTable.vue';
import AppRegistryHoverTip from '../../components/layout/AppRegistryHoverTip.vue';

import type { CascadePreview, CascadeProposal, CascadeSagaStep } from './types';
import { CASCADE_SAGA_TABLE_COLUMNS, memoryCascadeStatusColor as statusColor } from './memoryTableUi';

const props = defineProps<{
  agentId: string | null;
  rows: CascadeProposal[];
  loading: boolean;
  actingId: string | null;
  previewOpen: boolean;
  previewLoading: boolean;
  preview: CascadePreview | null;
  previewProposalId: string | null;
  sagaDrawerOpen: boolean;
  sagaLoading: boolean;
  sagaSteps: CascadeSagaStep[];
}>();

const emit = defineEmits<{
  refresh: [];
  approve: [row: CascadeProposal];
  reject: [row: CascadeProposal];
  preview: [row: CascadeProposal];
  confirmPreview: [proposalId: string];
  saga: [row: CascadeProposal];
  retry: [row: CascadeProposal];
  compensate: [row: CascadeProposal];
  'update:previewOpen': [value: boolean];
  'update:sagaDrawerOpen': [value: boolean];
}>();

const sagaDrawerOpen = computed({
  get: () => props.sagaDrawerOpen,
  set: (v) => emit('update:sagaDrawerOpen', v),
});

const previewOpen = computed({
  get: () => props.previewOpen,
  set: (v) => emit('update:previewOpen', v),
});

function onConfirmFromPreview() {
  if (props.previewProposalId) {
    emit('confirmPreview', props.previewProposalId);
  }
}

function riskColor(level?: string) {
  switch ((level || '').toLowerCase()) {
    case 'high':
      return 'negative';
    case 'medium':
      return 'warning';
    default:
      return 'grey-7';
  }
}

function stepIcon(state: string) {
  switch (state) {
    case 'succeeded':
      return 'check_circle';
    case 'running':
      return 'sync';
    case 'failed':
      return 'error';
    case 'compensated':
      return 'undo';
    case 'skipped':
      return 'skip_next';
    default:
      return 'radio_button_unchecked';
  }
}

function stepColor(state: string) {
  switch (state) {
    case 'succeeded':
      return 'positive';
    case 'running':
      return 'primary';
    case 'failed':
      return 'negative';
    case 'compensated':
      return 'deep-orange';
    case 'skipped':
      return 'grey-7';
    default:
      return 'grey-6';
  }
}

function stepDisplayName(name: string) {
  const map: Record<string, string> = {
    upsert_entity: '更新实体',
    touch_affected: '触碰受影响节点',
    replace_facts: '替换 Fact 语句',
    sync_index: '同步索引',
  };
  return map[name] || name;
}

function formatStepTime(value: string) {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString('zh-CN', { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}
</script>
