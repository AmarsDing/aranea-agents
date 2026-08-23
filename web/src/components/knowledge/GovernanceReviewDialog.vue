<template>
  <q-dialog :model-value="open" @update:model-value="$emit('update:open', !!$event)">
    <q-card class="app-dialog-card app-dialog-card--md app-glass-dialog kb-gov-review">
      <q-card-section class="app-glass-dialog__head row items-center justify-between">
        <div class="app-glass-dialog__title">{{ t('knowledgePage.workbench.commands.review-governance') }}</div>
      </q-card-section>
      <div class="app-glass-dialog__scroll">
        <q-card-section class="app-dialog-body app-glass-dialog__body">
      <div v-if="!homeIsCurrent" class="kb-gov-review__banner">
        {{ t('knowledgePage.workbench.writebackHomeHint', { name: homeName }) }}
        <q-btn
          flat
          dense
          no-caps
          color="primary"
          class="q-ml-sm"
          :label="t('knowledgePage.workbench.writebackHomeSwitch')"
          @click="$emit('switch-home')"
        />
      </div>
      <q-list dense class="kb-gov-review__list">
        <q-item v-for="row in rows" :key="row.item.id" class="kb-gov-review__item">
          <q-item-section>
            <q-item-label class="kb-gov-review__kind">{{ kindLabel(row.item.kind, row.payload) }}</q-item-label>
            <q-item-label>{{ row.summary }}</q-item-label>
            <q-item-label v-if="row.caption" caption>{{ row.caption }}</q-item-label>
          </q-item-section>
          <q-item-section side>
            <div class="kb-gov-review__btns">
              <q-btn
                v-for="decision in row.decisions"
                :key="decision"
                dense
                unelevated
                no-caps
                :color="decisionColor(decision)"
                :flat="decision === 'rejected'"
                :disable="loadingId === row.item.id"
                :loading="loadingId === row.item.id"
                :label="decisionLabel(decision, row.item.kind, row.payload)"
                @click="$emit('resolve', { id: row.item.id, decision })"
              />
            </div>
          </q-item-section>
        </q-item>
      </q-list>
        </q-card-section>
      </div>
      <q-card-actions align="right" class="app-glass-dialog__actions">
        <q-btn flat no-caps :label="t('common.cancel')" @click="$emit('update:open', false)" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { GovernanceProposalItem } from '../../features/knowledge/types';
import {
  decisionsForProposal,
  isFactConflict,
  parseProposalPayload,
  proposalSummary,
  type GovernanceDecision,
} from '../../features/knowledge/governance';

const props = defineProps<{
  open: boolean;
  items: GovernanceProposalItem[];
  homeName: string;
  homeIsCurrent: boolean;
  loadingId?: number;
}>();

defineEmits<{
  'update:open': [open: boolean];
  resolve: [payload: { id: number; decision: GovernanceDecision }];
  'switch-home': [];
}>();

const { t } = useI18n();

const rows = computed(() =>
  props.items.map((item) => {
    const payload = parseProposalPayload(item.payload_json);
    return {
      item,
      payload,
      summary: proposalSummary(item.kind, payload),
      caption: proposalCaption(item.kind, payload),
      decisions: decisionsForProposal(item.kind, payload),
    };
  }),
);

function kindLabel(kind: string, payload: Record<string, string>): string {
  if (isFactConflict(kind, payload)) return t('knowledgePage.workbench.govKindFact');
  if (kind === 'orphan') return t('knowledgePage.workbench.govKindOrphan');
  if (kind === 'conflict') return t('knowledgePage.workbench.govKindDoc');
  return kind;
}

function proposalCaption(kind: string, payload: Record<string, string>): string {
  if (isFactConflict(kind, payload)) {
    return [payload.rel_path, payload.reason].filter(Boolean).join(' · ');
  }
  if (kind === 'orphan') {
    const days = payload.last_access_days;
    return days ? t('knowledgePage.workbench.govOrphanDays', { n: days }) : payload.doc_id;
  }
  return payload.context || '';
}

function decisionLabel(decision: GovernanceDecision, kind: string, payload: Record<string, string>): string {
  if (decision === 'keep_old') return t('knowledgePage.workbench.govKeepOld');
  if (decision === 'keep_new') return t('knowledgePage.workbench.govKeepNew');
  if (decision === 'rejected') return t('knowledgePage.workbench.govReject');
  if (kind === 'orphan') return t('knowledgePage.workbench.govApplyOrphan');
  if (kind === 'conflict' && !isFactConflict(kind, payload)) return t('knowledgePage.workbench.govApplyDoc');
  return t('knowledgePage.workbench.govApply');
}

function decisionColor(decision: GovernanceDecision): string {
  if (decision === 'rejected') return 'grey';
  if (decision === 'keep_old') return 'secondary';
  return 'primary';
}
</script>

<style lang="sass" scoped>
.kb-gov-review
  &__banner
    font-size: 12px
    color: var(--color-text-secondary)
    margin-bottom: 12px

  &__list
    max-height: 420px
    overflow: auto
    margin: 0 -4px

  &__item
    align-items: flex-start
    padding-top: 10px
    padding-bottom: 10px

  &__kind
    font-size: 11px
    letter-spacing: 0.04em
    color: var(--color-text-secondary)
    margin-bottom: 2px

  &__btns
    display: flex
    flex-wrap: wrap
    gap: 6px
    justify-content: flex-end
    max-width: 280px
</style>
