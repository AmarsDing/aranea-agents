<template>
  <AppDialog
    :model-value="modelValue"
    :title="t('selfImprovementPage.rules.title')"
    :subtitle="t('selfImprovementPage.rules.subtitle')"
    size="sm"
    persistent
    :loading="saving"
    @update:model-value="$emit('update:modelValue', $event)"
  >
    <q-input
      v-model.number="lowMaxLines"
      type="number"
      dense
      outlined
      :min="0"
      :label="t('selfImprovementPage.rules.lowMax')"
      :hint="t('selfImprovementPage.rules.lowMaxHint', { value: effective.lowMaxLines })"
    />
    <q-input
      v-model.number="mediumMaxLines"
      type="number"
      dense
      outlined
      class="q-mt-md"
      :min="0"
      :label="t('selfImprovementPage.rules.mediumMax')"
      :hint="t('selfImprovementPage.rules.mediumMaxHint', { value: effective.mediumMaxLines })"
    />
    <q-input
      v-model.number="dailyAutoQuota"
      type="number"
      dense
      outlined
      class="q-mt-md"
      :min="0"
      :label="t('selfImprovementPage.rules.quota')"
      :hint="t('selfImprovementPage.rules.quotaHint', { value: effective.dailyAutoQuota })"
    />
    <q-input
      v-model="globsText"
      type="textarea"
      dense
      outlined
      class="q-mt-md"
      autogrow
      :label="t('selfImprovementPage.rules.globs')"
      :hint="t('selfImprovementPage.rules.globsHint')"
    />

    <div class="q-mt-md q-pa-sm app-glass-inset-preview">
      <div class="text-caption text-weight-bold">{{ t('selfImprovementPage.rules.effective') }}</div>
      <div class="text-caption q-mt-xs">
        {{
          t('selfImprovementPage.rules.effectiveSummary', {
            low: effective.lowMaxLines,
            medium: effective.mediumMaxLines,
            quota: effective.dailyAutoQuota,
          })
        }}
      </div>
      <div class="text-caption q-mt-xs app-break-all">
        {{ effective.corePathGlobs.join(', ') }}
      </div>
    </div>

    <div v-if="validationError" class="text-caption text-negative q-mt-sm">{{ validationError }}</div>
    <template #actions>
      <q-btn
        flat
        rounded
        no-caps
        :label="t('selfImprovementPage.rules.cancel')"
        @click="$emit('update:modelValue', false)"
      />
      <q-btn
        unelevated
        rounded
        no-caps
        color="primary"
        :label="t('selfImprovementPage.rules.save')"
        :disable="validationError !== ''"
        :loading="saving"
        @click="onSubmit"
      />
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import AppDialog from '../common/AppDialog.vue';
import type { SIRiskRules } from '../../features/self-improvement/types';

// Presentational risk-rules editor (73-self-iteration-v3, design §七).
// Configured values are editable; 0 / empty means "inherit the code default"
// and the effective (normalized) view is shown read-only for reference.

const { t } = useI18n();

const props = defineProps<{
  modelValue: boolean;
  configured: SIRiskRules;
  effective: SIRiskRules;
  saving?: boolean;
}>();

const emit = defineEmits<{
  'update:modelValue': [v: boolean];
  submit: [payload: SIRiskRules];
}>();

const lowMaxLines = ref(0);
const mediumMaxLines = ref(0);
const dailyAutoQuota = ref(0);
const globsText = ref('');

watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      lowMaxLines.value = props.configured.lowMaxLines;
      mediumMaxLines.value = props.configured.mediumMaxLines;
      dailyAutoQuota.value = props.configured.dailyAutoQuota;
      globsText.value = props.configured.corePathGlobs.join('\n');
    }
  },
);

const validationError = computed(() => {
  const nums = [lowMaxLines.value, mediumMaxLines.value, dailyAutoQuota.value];
  if (nums.some((v) => Number(v) < 0)) {
    return t('selfImprovementPage.rules.errNegative');
  }
  const low = Number(lowMaxLines.value) || 0;
  const medium = Number(mediumMaxLines.value) || 0;
  if (low > 0 && medium > 0 && low > medium) {
    return t('selfImprovementPage.rules.errLowGtMedium');
  }
  return '';
});

function onSubmit() {
  emit('submit', {
    lowMaxLines: Math.max(0, Number(lowMaxLines.value) || 0),
    mediumMaxLines: Math.max(0, Number(mediumMaxLines.value) || 0),
    dailyAutoQuota: Math.max(0, Number(dailyAutoQuota.value) || 0),
    corePathGlobs: globsText.value
      .split('\n')
      .map((s) => s.trim())
      .filter((s) => s.length > 0),
  });
}
</script>
