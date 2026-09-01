<template>
  <div class="model-router-rules-editor column q-gutter-sm">
    <div class="row items-center justify-between">
      <div class="text-subtitle2">{{ label }}</div>
      <q-btn flat dense no-caps color="primary" icon="add" :label="t('plugins.rulesAdd')" @click="addRule" />
    </div>
    <q-card v-for="rule in rules" :key="rule.id" flat bordered class="q-pa-sm">
      <div class="row q-col-gutter-sm items-start">
        <div class="col-12 col-md-4">
          <q-input
            v-model="rule.model"
            :label="t('plugins.rulesTargetModel')"
            dense
            outlined
            :hint="t('plugins.rulesTargetModelHint')"
            @update:model-value="emitRules"
          />
        </div>
        <div class="col-6 col-md-2">
          <q-input
            v-model.number="rule.priority"
            type="number"
            :label="t('plugins.rulesPriority')"
            dense
            outlined
            :hint="t('plugins.rulesPriorityHint')"
            @update:model-value="emitRules"
          />
        </div>
        <div class="col-6 col-md-2">
          <q-input
            v-model.number="rule.min_chars"
            type="number"
            :label="t('plugins.rulesMinChars')"
            dense
            outlined
            :hint="t('plugins.rulesMinCharsHint')"
            @update:model-value="emitRules"
          />
        </div>
        <div class="col-12 col-md-4 row justify-end">
          <q-btn flat dense round icon="delete" color="negative" @click="removeRule(rule.id)" />
        </div>
        <div class="col-12">
          <q-input
            :model-value="containsText(rule)"
            :label="t('plugins.rulesContains')"
            type="textarea"
            autogrow
            dense
            outlined
            @update:model-value="setContains(rule, String($event ?? ''))"
          />
        </div>
        <div class="col-12">
          <q-input
            v-model="rule.regex"
            :label="t('plugins.rulesRegex')"
            dense
            outlined
            :error="!!regexError(rule)"
            :error-message="regexError(rule)"
            @update:model-value="emitRules"
          />
        </div>
      </div>
    </q-card>
    <div v-if="rules.length === 0" class="text-caption text-grey-7">
      {{ t('plugins.rulesEmptyHint') }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { randomUUID } from '../../utils/uuid';

export type ModelRouterRule = {
  id: string;
  model: string;
  contains: string[];
  regex: string;
  min_chars: number;
  priority: number;
};

export type ModelRouterRulePayload = Omit<ModelRouterRule, 'id'>;

const props = defineProps<{
  modelValue: ModelRouterRulePayload[];
  label?: string;
}>();

const emit = defineEmits<{
  'update:modelValue': [value: ModelRouterRulePayload[]];
}>();

const { t } = useI18n();
const rules = ref<ModelRouterRule[]>([]);

function newRuleId() {
  return randomUUID();
}

function normalizeRule(raw: Partial<ModelRouterRule>, fallbackId?: string): ModelRouterRule {
  return {
    id: String(raw.id ?? fallbackId ?? newRuleId()),
    model: String(raw.model ?? ''),
    contains: Array.isArray(raw.contains) ? raw.contains.map(String) : [],
    regex: String(raw.regex ?? ''),
    min_chars: Number(raw.min_chars ?? 0) || 0,
    priority: Number(raw.priority ?? 0) || 0,
  };
}

watch(
  () => props.modelValue,
  (val) => {
    rules.value = Array.isArray(val)
      ? val.map((r, idx) => normalizeRule(r as Partial<ModelRouterRule>, `rule-${idx}`))
      : [];
  },
  { immediate: true, deep: true },
);

function regexError(rule: ModelRouterRule): string {
  const pat = rule.regex.trim();
  if (!pat) return '';
  try {
    new RegExp(pat);
    return '';
  } catch {
    return t('plugins.rulesRegexInvalid');
  }
}

function emitRules() {
  if (rules.value.some((rule) => regexError(rule))) {
    return;
  }
  emit(
    'update:modelValue',
    rules.value.map((r) => ({
      model: r.model.trim(),
      contains: r.contains.map((s) => s.trim()).filter(Boolean),
      regex: r.regex.trim(),
      min_chars: r.min_chars > 0 ? r.min_chars : 0,
      priority: r.priority || 0,
    })),
  );
}

function addRule() {
  rules.value.push({
    id: newRuleId(),
    model: '',
    contains: [],
    regex: '',
    min_chars: 0,
    priority: 0,
  });
  // 立即同步空规则到 modelValue，避免 UI 与上游数据不一致
  emitRules();
}

function removeRule(id: string) {
  rules.value = rules.value.filter((rule) => rule.id !== id);
  emitRules();
}

function containsText(rule: ModelRouterRule) {
  return (rule.contains ?? []).join('\n');
}

function setContains(rule: ModelRouterRule, text: string) {
  rule.contains = text
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean);
  emitRules();
}
</script>
