<template>
  <div>
    <div v-if="task" class="l1-budget q-mb-sm">
      <div class="row items-center justify-between text-caption text-grey-7">
        <span>{{ t('memory.sessions.l1Budget') }}</span>
        <span>{{ usedLabel }}</span>
      </div>
      <q-linear-progress rounded size="8px" :value="budgetRatio" :color="budgetColor" class="q-mt-xs" />
    </div>
    <q-tree
      v-if="nodes.length"
      :nodes="nodes"
      node-key="id"
      default-expand-all
      no-connectors
    >
      <template #default-header="prop">
        <div class="row items-center no-wrap q-gutter-xs l1-leaf">
          <span class="text-weight-medium">{{ prop.node.label }}</span>
          <q-chip v-if="prop.node.pinned" dense square size="sm" color="primary" text-color="white">
            {{ t('memory.sessions.l1Pinned') }}
          </q-chip>
          <q-chip v-if="prop.node.revision" dense square size="sm" color="blue-grey" text-color="white">
            v{{ prop.node.revision }}
          </q-chip>
          <span v-if="prop.node.tokens" class="text-caption text-grey-6">{{ prop.node.tokens }} tok</span>
        </div>
        <div v-if="prop.node.preview" class="text-caption text-grey-7 ellipsis">{{ prop.node.preview }}</div>
      </template>
    </q-tree>
    <div v-else class="text-center text-grey-7 q-pa-md">
      <q-icon name="account_tree" size="32px" />
      <div class="q-mt-sm">{{ emptyLabel }}</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { L1Task } from '../../features/memory/types';
import { taskBudgetRatio, type L1FieldTreeNode } from '../../features/memory/l1FieldTree';

const props = defineProps<{
  task: L1Task | null;
  nodes: L1FieldTreeNode[];
  emptyLabel: string;
}>();

const { t } = useI18n();

const budgetRatio = computed(() => taskBudgetRatio(props.task?.used_tokens ?? 0, props.task?.budget_tokens ?? 0));
const budgetColor = computed(() => {
  if (budgetRatio.value >= 0.8) return 'negative';
  if (budgetRatio.value >= 0.6) return 'warning';
  return 'positive';
});
const usedLabel = computed(() =>
  t('memory.sessions.l1BudgetUsed', {
    used: (props.task?.used_tokens ?? 0).toLocaleString(),
    budget: (props.task?.budget_tokens ?? 0).toLocaleString(),
  }),
);
</script>

<style scoped>
.l1-leaf {
  min-width: 0;
}
</style>
