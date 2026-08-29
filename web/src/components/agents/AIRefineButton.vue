<template>
  <q-btn
    v-bind="$attrs"
    :loading="loading"
    :disable="loading || !canRefine"
    :color="btnColor"
    :flat="flat"
    :outline="outline"
    :rounded="rounded ?? true"
    icon="auto_awesome"
    :label="label ?? t('agentSettings.aiRefine.buttonLabel')"
    @click="handleRefine"
  >
    <q-tooltip v-if="guide">
      <div class="text-caption app-tooltip-text">
        <strong>{{ guide.titleZh }}</strong
        >：{{ guide.purpose }}
        <template v-if="guide.budget.soft">
          <br />{{ $t('agentSettings.aiRefine.guideBudget', { count: guide.budget.soft }) }}
        </template>
      </div>
    </q-tooltip>
  </q-btn>

  <!-- Result dialog -->
  <q-dialog v-model="showResult" persistent>
    <q-card class="ai-refine-dialog-card app-dialog-card app-glass-dialog">
      <q-card-section class="row items-center">
        <div class="text-h6">{{ $t('agentSettings.aiRefine.dialogTitle') }}</div>
        <q-space />
        <span v-if="result" class="refine-chip">
          {{ result.provider }} / {{ result.model }}
          <q-tooltip>{{ $t('agentSettings.aiRefine.modelSource', { source: result.source }) }}</q-tooltip>
        </span>
        <q-btn flat round dense icon="close" class="q-ml-sm" @click="handleCancel">
          <q-tooltip>{{ $t('agentSettings.aiRefine.closeTooltip') }}</q-tooltip>
        </q-btn>
      </q-card-section>

      <q-card-section class="q-pt-none">
        <!-- Token delta -->
        <div v-if="result" class="row q-gutter-sm q-mb-sm">
          <span class="refine-chip">{{
            $t('agentSettings.aiRefine.tokensBefore', { count: result.tokensBefore })
          }}</span>
          <span class="refine-chip refine-chip--accent">{{
            $t('agentSettings.aiRefine.tokensAfter', { count: result.tokensAfter })
          }}</span>
        </div>

        <!-- Diff toggle -->
        <div class="row q-mb-sm">
          <q-btn-toggle
            v-model="resultView"
            dense
            flat
            toggle-color="primary"
            :options="viewOptions"
          />
        </div>

        <q-input
          v-if="resultView === 'result'"
          v-model="editedResult"
          type="textarea"
          outlined
          autogrow
          :rows="12"
          class="app-markdown-editor"
        />
        <pre v-else class="diff-view q-pa-sm">{{ result?.diff ?? '' }}</pre>

        <!-- Char budget indicator -->
        <div v-if="guide && editedResult" class="q-mt-xs text-caption">
          <span :class="budgetClass">{{
            $t('agentSettings.aiRefine.charCount', { count: charCount })
          }}</span>
          <span class="text-grey-6">
            {{
              $t('agentSettings.aiRefine.charBudget', {
                soft: guide.budget.soft,
                hard: guide.budget.hard || $t('agentSettings.aiRefine.charBudgetNone'),
              })
            }}
          </span>
        </div>

        <!-- User hint input -->
        <q-input
          v-model="userHint"
          class="q-mt-md"
          dense
          outlined
          :label="$t('agentSettings.aiRefine.hintLabel')"
          :placeholder="$t('agentSettings.aiRefine.hintPlaceholder')"
          @keyup.enter="handleRefine"
        />
      </q-card-section>

      <q-card-actions align="right" class="q-pa-md">
        <q-btn flat :label="$t('agentSettings.aiRefine.cancel')" @click="handleCancel" />
        <q-btn
          flat
          :loading="loading"
          icon="refresh"
          :label="$t('agentSettings.aiRefine.refineAgain')"
          @click="handleRefine"
        />
        <q-btn
          color="primary"
          unelevated
          rounded
          :label="$t('agentSettings.aiRefine.apply')"
          @click="applyResult"
        />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { FieldScope, FieldGuide } from '../../features/agents/fieldGuides';
import { getFieldGuide } from '../../features/agents/fieldGuides';
import type { RefineResponse } from '../../features/agents/aiRefine';

// ──────────────────────────────────────────────────────────────────────────────
// Props
// ──────────────────────────────────────────────────────────────────────────────

const props = withDefaults(
  defineProps<{
    scope: FieldScope;
    fileName?: string; // only for scope='agent.file'
    resourceId?: string;
    text: string; // current field content to refine
    targetMode?: string;
    label?: string;
    color?: string;
    flat?: boolean;
    outline?: boolean;
    rounded?: boolean;
    refineFn: (params: {
      scope: FieldScope;
      fileName?: string;
      resourceId?: string;
      originalText: string;
      userHint: string;
      targetMode: string;
    }) => Promise<RefineResponse>;
  }>(),
  {
    fileName: undefined,
    resourceId: undefined,
    targetMode: 'complete',
    label: undefined,
    color: undefined,
    flat: false,
    outline: true,
  },
);

const emit = defineEmits<{
  (e: 'apply', refined: string): void;
  (e: 'error', message: string): void;
}>();

const { t } = useI18n();

// ──────────────────────────────────────────────────────────────────────────────
// State
// ──────────────────────────────────────────────────────────────────────────────

const loading = ref(false);
const showResult = ref(false);
const resultView = ref<'result' | 'diff'>('result');
const result = ref<RefineResponse | null>(null);
const editedResult = ref('');
const userHint = ref('');

// ──────────────────────────────────────────────────────────────────────────────
// Computed
// ──────────────────────────────────────────────────────────────────────────────

const guide = computed<FieldGuide | undefined>(() => getFieldGuide(props.scope, props.fileName));

const canRefine = computed(() => props.text.trim().length > 0);

const charCount = computed(() => [...(editedResult.value ?? '')].length);

const budgetClass = computed(() => {
  const g = guide.value;
  if (!g) return 'text-grey-7';
  const n = charCount.value;
  if (g.budget.hard > 0 && n > g.budget.hard) return 'text-negative';
  if (g.budget.soft > 0 && n > g.budget.soft) return 'text-warning';
  return 'text-grey-7';
});

const btnColor = computed(() => props.color ?? 'primary');

const viewOptions = computed(() => [
  { label: t('agentSettings.aiRefine.viewResult'), value: 'result' },
  { label: t('agentSettings.aiRefine.viewDiff'), value: 'diff' },
]);

// ──────────────────────────────────────────────────────────────────────────────
// Actions
// ──────────────────────────────────────────────────────────────────────────────

async function handleRefine() {
  loading.value = true;
  try {
    const res = await props.refineFn({
      scope: props.scope,
      fileName: props.fileName,
      resourceId: props.resourceId,
      originalText: props.text,
      userHint: userHint.value,
      targetMode: props.targetMode,
    });
    result.value = res;
    editedResult.value = res.refined;
    showResult.value = true;
  } catch (e: unknown) {
    emit('error', e instanceof Error ? e.message : t('agentSettings.aiRefine.failed'));
  } finally {
    loading.value = false;
  }
}

function applyResult() {
  emit('apply', editedResult.value);
  showResult.value = false;
  userHint.value = '';
}

function handleCancel() {
  showResult.value = false;
  userHint.value = '';
}
</script>

<style scoped>
.ai-refine-dialog-card {
  min-width: min(620px, 92vw);
  max-width: 92vw;
}

/* Token-aware chip replacement: glass tokens keep day/night themes consistent
   (Quasar palette chips like grey-3/blue-1 break on the dark glass dialog). */
.refine-chip {
  display: inline-flex;
  align-items: center;
  padding: 2px 10px;
  border-radius: 999px;
  font-size: var(--text-xs, 12px);
  line-height: 1.6;
  color: var(--text-secondary);
  background: var(--glass-surface);
  border: 1px solid var(--glass-border);
  white-space: nowrap;
}

.refine-chip--accent {
  color: var(--color-accent);
  border-color: color-mix(in srgb, var(--color-accent) 40%, transparent);
}

.diff-view {
  font-family: monospace;
  font-size: var(--text-xs);
  background: var(--glass-surface);
  border-radius: 6px;
  max-height: 320px;
  overflow-y: auto;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
</style>
