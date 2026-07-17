// Container: approved — feature-local panel/dialog; data from Page composable via props.
<template>
  <q-drawer
    :model-value="modelValue"
    side="right"
    overlay
    bordered
    :width="480"
    class="memory-drawer"
    @update:model-value="$emit('update:modelValue', $event)"
  >
    <q-scroll-area class="fit">
      <div class="q-pa-md">
        <div class="row items-center justify-between q-mb-md">
          <div class="text-h6">{{ t('memory.cascade.saga.title') }}</div>
          <q-btn flat dense round icon="close" @click="$emit('update:modelValue', false)" />
        </div>

        <div v-if="loading" class="row justify-center q-py-lg">
          <q-spinner-dots size="32px" color="primary" />
        </div>

        <q-list v-else-if="steps.length" separator>
          <q-item v-for="step in steps" :key="step.id" class="q-px-sm">
            <q-item-section avatar>
              <q-icon :name="stepIcon(step.state)" :color="stepColor(step.state)" size="sm" />
            </q-item-section>
            <q-item-section>
              <q-item-label>
                <span class="text-weight-medium">{{ stepDisplayName(step.step_name) }}</span>
                <q-badge
                  v-if="step.is_critical"
                  color="deep-orange"
                  :label="t('memory.cascade.saga.critical')"
                  class="q-ml-xs"
                />
              </q-item-label>
              <q-item-label caption>
                {{ t('memory.cascade.saga.stateLabel', { state: step.state }) }} ·
                {{ t('memory.cascade.saga.attemptsLabel', { attempts: step.attempts }) }}
              </q-item-label>
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

        <q-banner v-else rounded class="memory-info-banner">{{ t('memory.cascade.saga.empty') }}</q-banner>
      </div>
    </q-scroll-area>
  </q-drawer>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { CascadeSagaStep } from './types';

const { t, te, locale } = useI18n();

defineProps<{
  modelValue: boolean;
  loading: boolean;
  steps: CascadeSagaStep[];
}>();

defineEmits<{
  'update:modelValue': [value: boolean];
}>();

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
  const key = `memory.cascade.saga.stepNames.${name}`;
  return te(key) ? t(key) : name;
}

function formatStepTime(value: string) {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString(locale.value, { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}
</script>
