// Container: approved — feature-local panel; data from Page composable via props.
<template>
  <q-card flat bordered class="memory-card">
    <q-card-section class="row items-center q-col-gutter-md">
      <div class="col">
        <div class="text-h6">{{ t('memory.cascade.title') }}</div>
        <div class="text-body2 text-grey-7">{{ t('memory.cascade.subtitle') }}</div>
      </div>
      <div class="col-auto">
        <q-btn flat icon="refresh" :label="t('memory.cascade.refresh')" :loading="loading" @click="$emit('refresh')" />
      </div>
    </q-card-section>

    <q-card-section v-if="!agentId" class="memory-cascade-empty">
      <q-icon name="smart_toy" size="26px" class="memory-cascade-empty__icon" />
      <div>{{ t('memory.cascade.selectAgentFirst') }}</div>
    </q-card-section>

    <q-card-section v-else-if="!rows.length && !loading" class="memory-cascade-empty">
      <q-icon name="account_tree" size="26px" class="memory-cascade-empty__icon" />
      <div>{{ t('memory.cascade.empty') }}</div>
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
        <template #body-cell-change="slotProps">
          <q-td :props="slotProps">
            <AppRegistryHoverTip :text="slotProps.row.rationale">
              <div class="min-width-0">
                <div class="app-registry-cell-primary">
                  {{ slotProps.row.old_value }} → {{ slotProps.row.new_value }}
                </div>
                <div class="app-registry-cell-sub ellipsis">{{ slotProps.row.trigger_entity_name }}</div>
              </div>
            </AppRegistryHoverTip>
          </q-td>
        </template>
        <template #body-cell-status="slotProps">
          <q-td :props="slotProps">
            <q-badge :color="statusColor(slotProps.row.status)" :label="slotProps.row.status" />
          </q-td>
        </template>
        <template #body-cell-risk="slotProps">
          <q-td :props="slotProps">
            <q-badge :color="riskColor(slotProps.row.risk_level)" :label="slotProps.row.risk_level || 'unknown'" />
          </q-td>
        </template>
        <template #body-cell-affected="slotProps">
          <q-td :props="slotProps">
            <q-chip dense square color="blue-grey" text-color="white">{{
              slotProps.row.affected_entities?.length ?? 0
            }}</q-chip>
          </q-td>
        </template>
        <template #body-cell-actions="slotProps">
          <q-td :props="slotProps">
            <div class="app-registry-cell-actions">
              <q-btn
                v-if="slotProps.row.status === 'pending'"
                dense
                flat
                round
                color="info"
                icon="visibility"
                @click="$emit('preview', slotProps.row)"
              >
                <q-tooltip>{{ t('memory.cascade.tooltips.dryRun') }}</q-tooltip>
              </q-btn>
              <q-btn
                v-if="slotProps.row.status === 'pending'"
                dense
                flat
                round
                color="positive"
                icon="check"
                :loading="actingId === slotProps.row.id"
                @click="$emit('approve', slotProps.row)"
              >
                <q-tooltip>{{ t('memory.cascade.tooltips.approve') }}</q-tooltip>
              </q-btn>
              <q-btn
                v-if="slotProps.row.status === 'pending'"
                dense
                flat
                round
                color="negative"
                icon="close"
                :loading="actingId === slotProps.row.id"
                @click="$emit('reject', slotProps.row)"
              >
                <q-tooltip>{{ t('memory.cascade.tooltips.reject') }}</q-tooltip>
              </q-btn>
              <q-btn
                v-if="slotProps.row.status === 'partial'"
                dense
                flat
                round
                color="warning"
                icon="replay"
                :loading="actingId === slotProps.row.id"
                @click="$emit('retry', slotProps.row)"
              >
                <q-tooltip>{{ t('memory.cascade.tooltips.retry') }}</q-tooltip>
              </q-btn>
              <q-btn
                v-if="slotProps.row.status === 'partial' || slotProps.row.status === 'failed'"
                dense
                flat
                round
                color="deep-orange"
                icon="undo"
                :loading="actingId === slotProps.row.id"
                @click="$emit('compensate', slotProps.row)"
              >
                <q-tooltip>{{ t('memory.cascade.tooltips.compensate') }}</q-tooltip>
              </q-btn>
              <q-btn
                v-if="slotProps.row.status !== 'pending'"
                dense
                flat
                round
                icon="account_tree"
                @click="$emit('saga', slotProps.row)"
              >
                <q-tooltip>{{ t('memory.cascade.tooltips.saga') }}</q-tooltip>
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
            <div class="app-glass-dialog__title">{{ t('memory.cascade.preview.title') }}</div>
            <div class="app-glass-dialog__subtitle">{{ t('memory.cascade.preview.subtitle') }}</div>
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
                  <div class="text-caption text-grey-7">{{ t('memory.cascade.preview.affectedEntities') }}</div>
                  <div class="text-h6">{{ preview.affected_entities_count }}</div>
                </div>
                <div class="col-6">
                  <div class="text-caption text-grey-7">{{ t('memory.cascade.preview.affectedFacts') }}</div>
                  <div class="text-h6">{{ preview.affected_facts_count }}</div>
                </div>
              </div>

              <div v-if="preview.entity_renames.length" class="q-mb-md">
                <div class="text-subtitle2 q-mb-sm">{{ t('memory.cascade.preview.entityRenames') }}</div>
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
                <div class="text-subtitle2 q-mb-sm">{{ t('memory.cascade.preview.factDiffs') }}</div>
                <q-list dense bordered class="rounded-borders" separator>
                  <q-item v-for="d in preview.fact_diffs" :key="d.fact_id">
                    <q-item-section>
                      <q-item-label caption>{{ d.scope }} · {{ d.fact_id.slice(0, 8) }}</q-item-label>
                      <q-item-label class="text-grey-7 memory-cascade-diff__before">{{
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
                {{ t('memory.cascade.preview.noChanges') }}
              </q-banner>
            </template>
          </q-card-section>
        </div>
        <q-separator />
        <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions">
          <q-btn v-close-popup flat :label="t('memory.cascade.preview.cancel')" />
          <q-btn
            unelevated
            :label="t('memory.cascade.preview.confirm')"
            color="primary"
            :loading="actingId === previewProposalId"
            @click="onConfirmFromPreview"
          />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import AppRegistryTable from '../../components/layout/AppRegistryTable.vue';
import AppRegistryHoverTip from '../../components/layout/AppRegistryHoverTip.vue';

import type { CascadePreview, CascadeProposal } from './types';
import { buildCascadeSagaTableColumns, memoryCascadeStatusColor as statusColor } from './memoryTableUi';

const { t } = useI18n();

const CASCADE_SAGA_TABLE_COLUMNS = computed(() => buildCascadeSagaTableColumns(t));

const props = defineProps<{
  agentId: string | null;
  rows: CascadeProposal[];
  loading: boolean;
  actingId: string | null;
  previewOpen: boolean;
  previewLoading: boolean;
  preview: CascadePreview | null;
  previewProposalId: string | null;
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
}>();

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
</script>

<style scoped lang="sass">
.memory-cascade-empty
  display: flex
  flex-direction: column
  align-items: center
  gap: var(--space-2)
  padding: var(--space-8) var(--space-4)
  font-size: var(--text-sm)
  color: var(--color-text-secondary)

  &__icon
    color: var(--color-icon-muted)
    opacity: 0.7

.memory-cascade-diff__before
  text-decoration: line-through
</style>
