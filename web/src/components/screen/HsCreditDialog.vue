<template>
  <AppDialog
    :model-value="modelValue"
    size="sm"
    icon="toll"
    :title="t('harnessScreen.credits.dialogTitle', { name: draft.title })"
    :subtitle="t('harnessScreen.credits.dialogSub')"
    :loading="saving"
    @update:model-value="(v: boolean) => $emit('update:modelValue', v)"
  >
    <form class="hs-credit-form" @submit.prevent="submit">
      <q-input v-model="draft.plan" dense outlined :label="t('harnessScreen.credits.fPlan')" maxlength="40" />
      <div class="hs-credit-form__pair">
        <q-input
          v-model.number="draft.total"
          dense
          outlined
          type="number"
          min="0"
          step="any"
          inputmode="decimal"
          :label="t('harnessScreen.credits.fTotal')"
          :hint="t('harnessScreen.credits.fBlank')"
        />
        <q-input
          v-model.number="draft.remaining"
          dense
          outlined
          type="number"
          min="0"
          step="any"
          inputmode="decimal"
          :label="t('harnessScreen.credits.fRemaining')"
          :hint="t('harnessScreen.credits.fBlank')"
        />
      </div>
      <q-select
        v-model="draft.unit"
        dense
        outlined
        emit-value
        map-options
        :options="unitOptions"
        :label="t('harnessScreen.credits.fUnit')"
      />
      <q-input v-model="draft.note" dense outlined :label="t('harnessScreen.credits.fNote')" maxlength="80" />
      <p v-if="message" class="hs-credit-form__error" role="alert">{{ message }}</p>
      <button type="submit" hidden />
    </form>
    <template #actions>
      <q-btn flat no-caps :label="t('harnessScreen.credits.cancel')" @click="$emit('update:modelValue', false)" />
      <q-btn
        unelevated
        no-caps
        color="primary"
        :loading="saving"
        :label="t('harnessScreen.credits.save')"
        @click="submit"
      />
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import AppDialog from '../common/AppDialog.vue';
import type { HsCreditForm } from './hsTypes';

const props = defineProps<{ modelValue: boolean; form: HsCreditForm; saving: boolean; error: string }>();
const emit = defineEmits<{ 'update:modelValue': [open: boolean]; save: [form: HsCreditForm] }>();
const { t } = useI18n();

const draft = ref<HsCreditForm>({ ...props.form });
const invalid = ref('');
watch(
  () => [props.modelValue, props.form] as const,
  ([open]) => {
    if (open) {
      draft.value = { ...props.form };
      invalid.value = '';
    }
  },
);

const unitOptions = computed(() => [
  { value: 'credits', label: t('harnessScreen.credits.unitCredits') },
  { value: 'CNY', label: 'CNY ¥' },
  { value: 'USD', label: 'USD $' },
]);
const message = computed(() => invalid.value || props.error);

// 空输入框 v-model.number 给的是 ''，统一成 null（= 不知道）
const num = (v: unknown) => (v === '' || v == null || Number.isNaN(Number(v)) ? null : Number(v));

function submit() {
  const total = num(draft.value.total);
  const remaining = num(draft.value.remaining);
  if ((total !== null && total < 0) || (remaining !== null && remaining < 0)) {
    invalid.value = t('harnessScreen.credits.errNegative');
    return;
  }
  if (total !== null && remaining !== null && remaining > total) {
    invalid.value = t('harnessScreen.credits.errOver');
    return;
  }
  invalid.value = '';
  emit('save', { ...draft.value, total, remaining, plan: draft.value.plan.trim(), note: draft.value.note.trim() });
}
</script>
