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
    :label="label ?? 'AI 优化'"
    @click="handleRefine"
  >
    <q-tooltip v-if="guide">
      <div class="text-caption" style="max-width: 280px">
        <strong>{{ guide.titleZh }}</strong
        >：{{ guide.purpose }}
        <template v-if="guide.budget.soft"> <br />建议字数：{{ guide.budget.soft }} 字以内 </template>
      </div>
    </q-tooltip>
  </q-btn>

  <!-- Result dialog -->
  <q-dialog v-model="showResult" persistent>
    <q-card class="ai-refine-dialog-card app-dialog-card app-glass-dialog">
      <q-card-section class="row items-center">
        <div class="text-h6">AI 优化结果</div>
        <q-space />
        <span v-if="result" class="refine-chip">
          {{ result.provider }} / {{ result.model }}
          <q-tooltip>模型来源：{{ result.source }}</q-tooltip>
        </span>
        <q-btn flat round dense icon="close" class="q-ml-sm" @click="handleCancel">
          <q-tooltip>关闭（不应用更改）</q-tooltip>
        </q-btn>
      </q-card-section>

      <q-card-section class="q-pt-none">
        <!-- Token delta -->
        <div v-if="result" class="row q-gutter-sm q-mb-sm">
          <span class="refine-chip">优化前 ≈ {{ result.tokensBefore }} tokens</span>
          <span class="refine-chip refine-chip--accent">优化后 ≈ {{ result.tokensAfter }} tokens</span>
        </div>

        <!-- Diff toggle -->
        <div class="row q-mb-sm">
          <q-btn-toggle
            v-model="resultView"
            dense
            flat
            toggle-color="primary"
            :options="[
              { label: '优化结果', value: 'result' },
              { label: '差异对比', value: 'diff' },
            ]"
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
          <span :class="budgetClass">{{ charCount }} 字</span>
          <span class="text-grey-6"> / 软上限 {{ guide.budget.soft }}（硬上限 {{ guide.budget.hard || '无' }}） </span>
        </div>

        <!-- User hint input -->
        <q-input
          v-model="userHint"
          class="q-mt-md"
          dense
          outlined
          label="补充优化说明（可选）"
          placeholder='例如："更正式一些" 或 "补充 KPI 指标"'
          @keyup.enter="handleRefine"
        />
      </q-card-section>

      <q-card-actions align="right" class="q-pa-md">
        <q-btn flat label="取消" @click="handleCancel" />
        <q-btn flat :loading="loading" icon="refresh" label="重新优化" @click="handleRefine" />
        <q-btn color="primary" unelevated rounded label="应用" @click="applyResult" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
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
    emit('error', e instanceof Error ? e.message : 'AI 优化失败，请重试');
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
