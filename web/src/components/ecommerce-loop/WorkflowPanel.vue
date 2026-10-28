<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type { DemoWorkflow, WorkflowAction, WorkflowActionPayload } from '../../features/ecommerce-loop/types';
import ReferenceIcon from './ReferenceIcon.vue';
const props = withDefaults(
  defineProps<{ workflow: DemoWorkflow; editing: boolean; shopName: string; embedded?: boolean }>(),
  { embedded: false },
);
const emit = defineEmits<{
  (event: 'close', dirty: boolean): void;
  (event: 'action', payload: WorkflowActionPayload): void;
  (event: 'dirty', value: boolean): void;
}>();
const { t } = useI18n();
const note = ref('');
const initialNote = ref('');
const reviewedVersion = ref(1);
const publishConfirmed = ref(false);
watch(
  () => [props.editing, props.workflow.version, props.workflow.kind, props.shopName],
  () => {
    reviewedVersion.value = props.workflow.version;
    initialNote.value = props.workflow.note;
    note.value = props.workflow.note;
    publishConfirmed.value = false;
  },
  { immediate: true },
);
const dirty = computed(() => props.editing && note.value !== initialNote.value);
watch(dirty, (value) => emit('dirty', value), { immediate: true, flush: 'sync' });
onUnmounted(() => emit('dirty', false));
function onAction(action: WorkflowAction) {
  emit('action', { action, version: reviewedVersion.value, note: note.value });
}
</script>

<template>
  <q-card :class="embedded ? 'ec-task-panel' : 'app-dialog-card app-glass-dialog ec-workflow-drawer'">
    <q-card-section class="app-glass-dialog__head row items-center justify-between"
      ><div>
        <h2 class="app-glass-dialog__title" data-task-heading tabindex="-1">
          {{ t(`ecommerceLoop.workflow.${workflow.kind}`) }}
        </h2>
        <p>{{ shopName }} · {{ t('ecommerceLoop.workflow.version', { version: workflow.version }) }}</p>
      </div>
      <q-btn
        v-if="!embedded"
        flat
        round
        dense
        icon="close"
        :aria-label="t('ecommerceLoop.close')"
        @click="emit('close', dirty)"
    /></q-card-section>
    <q-separator />
    <q-card-section class="app-glass-dialog__scroll ec-workflow-body">
      <p class="ec-workflow-notice">{{ t('ecommerceLoop.workflow.demo') }}</p>
      <div class="ec-workflow-status">
        <q-icon :name="workflow.phase === 'closed' ? 'task_alt' : 'pending_actions'" /><strong>{{
          t(`ecommerceLoop.phases.${workflow.phase}`)
        }}</strong>
      </div>
      <p v-if="workflow.note && !editing">{{ t('ecommerceLoop.workflow.approvedNote', { note: workflow.note }) }}</p>
      <p v-if="workflow.note && workflow.phase === 'awaiting_decision' && !editing" class="ec-workflow-notice">
        {{ t('ecommerceLoop.workbench.revisionNotice') }}
      </p>
      <template v-if="editing"
        ><q-input
          v-model="note"
          outlined
          type="textarea"
          autogrow
          counter
          :maxlength="2000"
          :label="t('ecommerceLoop.workflow.note')"
          :hint="t('ecommerceLoop.workflow.noteHint')"
          :rules="[
            (value: string) => !!value.trim() || t('ecommerceLoop.workflow.noteRequired'),
            (value: string) => value.length <= 2000 || t('ecommerceLoop.workflow.noteMax'),
          ]"
      /></template>
      <template v-else>
        <p>
          {{ t(`ecommerceLoop.workbench.intent.${workflow.kind}`) }}
        </p>
        <div
          v-if="['awaiting_decision', 'rejected', 'approved', 'drafting'].includes(workflow.phase)"
          class="ec-task-scope"
        >
          <q-icon name="info_outline" /><span>{{ t('ecommerceLoop.workbench.scope') }}</span>
        </div>
        <div v-if="workflow.phase === 'awaiting_publish_approval'" class="ec-task-scope">
          <q-icon name="info_outline" /><span>{{
            t('ecommerceLoop.workbench.publishTarget', { shop: shopName, count: workflow.drafts.length })
          }}</span>
        </div>
        <p v-if="workflow.phase === 'rejected'">{{ t('ecommerceLoop.workflow.rejected') }}</p>
        <p v-if="workflow.phase === 'rework_required'" class="text-negative">
          {{ t('ecommerceLoop.workflow.verificationFailed') }}
        </p>
        <h3>{{ t('ecommerceLoop.workflow.drafts') }}</h3>
        <p v-if="!workflow.drafts.length" class="text-caption">{{ t('ecommerceLoop.workflow.noDrafts') }}</p>
        <div v-for="draft in workflow.drafts" :key="draft.id" class="ec-draft-preview">
          <ReferenceIcon
            kind="candle"
            :tone="draft.productId === 'grapefruit' ? 'danger' : draft.productId === 'lime' ? 'lime' : 'ai'"
          />
          <div>
            <b>{{ t(`ecommerceLoop.products.${draft.titleKey}`) }}</b>
            <p>{{ t('ecommerceLoop.workflow.draftDescription') }}</p>
            <p class="ec-workflow-notice">{{ t('ecommerceLoop.workbench.draftBoundary') }}</p>
            <p v-if="draft.scentKey !== draft.productId" class="ec-negative-text">
              {{
                t('ecommerceLoop.workbench.scentMismatch', {
                  expected: t(`ecommerceLoop.products.${draft.productId}`),
                  actual: t(`ecommerceLoop.products.${draft.scentKey}`),
                })
              }}
            </p>
            <small
              >{{ t(`ecommerceLoop.products.${draft.scentKey}`) }} ·
              {{
                t(draft.published ? 'ecommerceLoop.workflow.published' : 'ecommerceLoop.workflow.unpublished')
              }}</small
            >
          </div>
        </div>
        <q-checkbox
          v-if="workflow.phase === 'awaiting_publish_approval'"
          v-model="publishConfirmed"
          :label="t('ecommerceLoop.workbench.publishConfirm')"
        />
        <p v-if="['awaiting_feedback', 'closed'].includes(workflow.phase)">
          {{ t('ecommerceLoop.workflow.verified') }}
        </p>
        <p v-if="workflow.phase === 'closed'">
          {{ t(workflow.kind === 'launch' ? 'ecommerceLoop.launch.week' : 'ecommerceLoop.workflow.feedbackTraffic') }}
        </p>
        <p v-else-if="workflow.phase === 'awaiting_feedback'" class="ec-workflow-notice">
          {{ t('ecommerceLoop.workbench.feedbackWaiting') }}
        </p>
        <details class="ec-task-history">
          <summary>{{ t('ecommerceLoop.workflow.history') }} · {{ workflow.history.length }}</summary>
          <p class="ec-workflow-notice">{{ t('ecommerceLoop.workbench.historyNotice') }}</p>
          <ol class="ec-workflow-history">
            <li v-for="(phase, index) in workflow.history" :key="`${index}-${phase}`">
              {{ t(`ecommerceLoop.phases.${phase}`) }}
            </li>
          </ol>
        </details>
      </template>
    </q-card-section>
    <q-separator />
    <q-card-actions align="right" class="app-actions-bar ec-workflow-actions">
      <template v-if="editing"
        ><q-btn
          unelevated
          class="app-btn-primary"
          :label="t('ecommerceLoop.workflow.saveRevision')"
          :disable="!note.trim() || note.length > 2000"
          @click="onAction('revise')" /><q-btn flat :label="t('ecommerceLoop.cancel')" @click="onAction('cancel-edit')"
      /></template>
      <template v-else-if="workflow.phase === 'awaiting_decision'"
        ><q-btn flat :label="t('ecommerceLoop.traffic.reject')" @click="onAction('rejected')" /><q-btn
          flat
          :label="t('ecommerceLoop.traffic.revise')"
          @click="onAction('revise')" /><q-btn
          unelevated
          class="app-btn-primary"
          :label="t(`ecommerceLoop.workbench.approve.${workflow.kind}`)"
          @click="onAction('approved')"
      /></template>
      <q-btn
        v-else-if="['approved', 'drafting', 'rework_required'].includes(workflow.phase)"
        unelevated
        class="app-btn-primary"
        :label="
          t(
            `ecommerceLoop.workflow.${workflow.phase === 'approved' ? 'dispatch' : workflow.phase === 'drafting' ? 'draft' : 'rework'}`,
          )
        "
        @click="onAction('advance')"
      />
      <template v-else-if="workflow.phase === 'awaiting_publish_approval'">
        <q-btn flat :label="t('ecommerceLoop.traffic.revise')" @click="onAction('revise')" />
        <q-btn
          unelevated
          class="app-btn-primary"
          :disable="!publishConfirmed"
          :label="t('ecommerceLoop.workbench.publish', { count: workflow.drafts.length })"
          @click="onAction('publish')"
        />
      </template>
      <template v-else-if="workflow.phase === 'verification_pending'">
        <q-btn
          flat
          :label="t('ecommerceLoop.workflow.simulateFailure')"
          @click="onAction('simulate-verification-failure')"
        />
        <q-btn
          unelevated
          class="app-btn-primary"
          :label="t('ecommerceLoop.workflow.verify')"
          @click="onAction('verify')"
        />
      </template>
      <q-btn
        v-else-if="workflow.phase === 'awaiting_feedback'"
        unelevated
        class="app-btn-primary"
        :label="t(`ecommerceLoop.workflow.${workflow.kind === 'launch' ? 'feedback' : 'feedbackNextRound'}`)"
        @click="onAction('feedback')"
      />
      <q-btn
        v-else-if="workflow.phase === 'rejected'"
        flat
        :label="t('ecommerceLoop.traffic.revise')"
        @click="onAction('revise')"
      />
    </q-card-actions>
  </q-card>
</template>
