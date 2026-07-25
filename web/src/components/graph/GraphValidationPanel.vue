<template>
  <transition name="graph-dock">
    <div v-if="open" class="graph-validation-dock">
      <div class="graph-validation-dock__header">
        <q-icon name="fact_check" size="16px" class="graph-validation-dock__header-icon" />
        <span class="graph-validation-dock__title">{{ t('graphs.validationPanelTitle') }}</span>
        <div class="graph-validation-dock__filters">
          <button
            v-for="f in filterOptions"
            :key="f.value"
            type="button"
            :data-testid="`filter-${f.value}`"
            :class="['graph-validation-dock__filter', { 'graph-validation-dock__filter--active': levelFilter === f.value }]"
            @click="levelFilter = f.value"
          >
            {{ t(f.labelKey) }} · {{ f.count }}
          </button>
        </div>
        <q-space />
        <q-btn flat dense round icon="refresh" size="sm" :loading="validating" data-testid="revalidate-btn" @click="$emit('revalidate')">
          <q-tooltip>{{ t('graphs.validationRevalidate') }} (F)</q-tooltip>
        </q-btn>
        <q-btn flat dense round icon="close" size="sm" data-testid="close-btn" @click="$emit('close')">
          <q-tooltip>{{ t('graphs.validationClose') }} (Esc)</q-tooltip>
        </q-btn>
      </div>
      <div class="graph-validation-dock__list">
        <div
          v-for="(issue, idx) in filteredIssues"
          :key="`${issue.level}-${issue.code}-${issue.nodeId}-${idx}`"
          :class="['graph-validation-dock__row', `graph-validation-dock__row--${issue.level}`]"
          @click="onRowClick(issue)"
        >
          <q-icon
            :name="issue.level === 'error' ? 'error' : 'warning'"
            size="14px"
            :class="`graph-validation-dock__row-icon graph-validation-dock__row-icon--${issue.level}`"
          />
          <div class="graph-validation-dock__row-main">
            <div class="graph-validation-dock__row-head">
              <span class="graph-validation-dock__node">{{ issue.nodeLabel || t('graphs.validationGraphLevel') }}</span>
              <code class="graph-validation-dock__code">{{ issue.code }}</code>
            </div>
            <div class="graph-validation-dock__message">{{ issue.message }}</div>
            <div v-if="suggestionOf(issue)" class="graph-validation-dock__suggestion">
              <q-icon name="lightbulb" size="12px" />
              <span>{{ suggestionOf(issue) }}</span>
            </div>
          </div>
          <q-btn
            v-if="issue.nodeId"
            flat
            dense
            no-caps
            size="sm"
            icon="my_location"
            :label="t('graphs.validationLocate')"
            data-testid="locate-btn"
            @click.stop="$emit('locate', issue.nodeId)"
          />
        </div>
        <div v-if="filteredIssues.length === 0" class="graph-validation-dock__empty">
          {{ t('graphs.validationFilterEmpty') }}
        </div>
      </div>
    </div>
  </transition>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { ValidationIssue } from '../../features/graph/types';
import { validationSuggestionKey } from '../../features/graph/validationIssues';

const props = defineProps<{
  open: boolean;
  issues: ValidationIssue[];
  validating?: boolean;
}>();

const emit = defineEmits<{
  locate: [nodeId: string];
  close: [];
  revalidate: [];
}>();

const { t } = useI18n();

type LevelFilter = 'all' | 'error' | 'warning';
const levelFilter = ref<LevelFilter>('all');

const counts = computed(() => ({
  all: props.issues.length,
  error: props.issues.filter((i) => i.level === 'error').length,
  warning: props.issues.filter((i) => i.level === 'warning').length,
}));

const filterOptions = computed(() => [
  { value: 'all' as const, labelKey: 'graphs.validationFilterAll', count: counts.value.all },
  { value: 'error' as const, labelKey: 'graphs.validationFilterErrors', count: counts.value.error },
  { value: 'warning' as const, labelKey: 'graphs.validationFilterWarnings', count: counts.value.warning },
]);

const filteredIssues = computed(() => {
  if (levelFilter.value === 'all') return props.issues;
  return props.issues.filter((i) => i.level === levelFilter.value);
});

function suggestionOf(issue: ValidationIssue): string {
  const key = validationSuggestionKey(issue.code);
  return key ? t(key) : '';
}

function onRowClick(issue: ValidationIssue) {
  if (issue.nodeId) {
    emit('locate', issue.nodeId);
  }
}
</script>
