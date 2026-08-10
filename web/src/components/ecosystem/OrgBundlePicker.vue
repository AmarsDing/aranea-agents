<template>
  <div class="org-bundle-picker row q-col-gutter-md">
    <div class="col-12 col-md-7">
      <div class="text-caption text-grey-7 q-mb-xs">{{ t('shopPage.publish.orgPickHint') }}</div>
      <q-tree
        v-model:ticked="ticked"
        v-model:expanded="expanded"
        :nodes="treeNodes"
        node-key="key"
        tick-strategy="leaf"
        default-expand-all
        class="org-bundle-picker__tree"
      >
        <template #default-header="prop">
          <div class="row items-center q-gutter-xs">
            <q-icon :name="nodeIcon(prop.node)" size="16px" :color="nodeColor(prop.node)" />
            <span :class="{ 'text-weight-bold': prop.node.level === 0 }">{{ prop.node.label }}</span>
            <q-chip v-if="prop.node.level === 1" dense size="sm" outline class="q-ml-xs">
              {{ t('shopPage.publish.orgAgentCount', { count: prop.node.children?.length ?? 0 }) }}
            </q-chip>
          </div>
        </template>
      </q-tree>
    </div>
    <div class="col-12 col-md-5">
      <q-card flat class="app-glass-panel q-pa-md org-bundle-picker__summary">
        <div class="text-weight-bold q-mb-sm">{{ t('shopPage.publish.orgSummary') }}</div>
        <div class="row q-gutter-sm q-mb-md">
          <q-chip dense icon="corporate_fare" color="primary" text-color="white">{{ summary.departments }}</q-chip>
          <q-chip dense icon="badge" color="teal" text-color="white">{{ summary.positions }}</q-chip>
          <q-chip dense icon="smart_toy" color="deep-purple" text-color="white">{{ summary.agents }}</q-chip>
        </div>
        <q-list dense class="org-bundle-picker__preview">
          <q-item v-for="line in previewLines" :key="line" dense class="q-px-none">
            <q-item-section class="text-caption">{{ line }}</q-item-section>
          </q-item>
        </q-list>
        <div v-if="ticked.length === 0" class="text-caption text-grey-6">{{ t('shopPage.publish.orgPickEmpty') }}</div>
      </q-card>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type { OrgPickNode } from '../../features/ecosystem/types';

const props = defineProps<{
  nodes: OrgPickNode[];
}>();

const emit = defineEmits<{
  change: [summary: { departments: number; positions: number; agents: number }];
}>();

const { t } = useI18n();

interface TreeNode {
  key: string;
  label: string;
  level: number;
  children?: TreeNode[];
}

const treeNodes = computed<TreeNode[]>(() =>
  props.nodes.map((dept) => ({
    key: dept.key,
    label: dept.label,
    level: 0,
    children: (dept.children ?? []).map((pos) => ({
      key: pos.key,
      label: pos.label,
      level: 1,
      children: (pos.agents ?? []).map((a) => ({ key: `${pos.key}/${a}`, label: a, level: 2 })),
    })),
  })),
);

const ticked = ref<string[]>([]);
const expanded = ref<string[]>(props.nodes.map((n) => n.key));

const summary = computed(() => {
  const posSet = new Set<string>();
  const deptSet = new Set<string>();
  for (const key of ticked.value) {
    const [posKey] = key.split('/');
    posSet.add(posKey);
    const dept = props.nodes.find((d) => (d.children ?? []).some((p) => p.key === posKey));
    if (dept) deptSet.add(dept.key);
  }
  return { departments: deptSet.size, positions: posSet.size, agents: ticked.value.length };
});

const previewLines = computed(() => {
  const lines: string[] = [];
  for (const dept of props.nodes) {
    const posLines: string[] = [];
    for (const pos of dept.children ?? []) {
      const picked = (pos.agents ?? []).filter((a) => ticked.value.includes(`${pos.key}/${a}`));
      if (picked.length > 0) posLines.push(`  ${pos.label}（${picked.length}）`);
    }
    if (posLines.length > 0) lines.push(dept.label, ...posLines);
  }
  return lines;
});

watch(summary, (s) => emit('change', s), { deep: true });

function nodeIcon(node: TreeNode): string {
  return node.level === 0 ? 'corporate_fare' : node.level === 1 ? 'badge' : 'smart_toy';
}

function nodeColor(node: TreeNode): string {
  return node.level === 0 ? 'primary' : node.level === 1 ? 'teal' : 'deep-purple';
}
</script>

<style scoped>
.org-bundle-picker__tree {
  border: 1px solid var(--glass-border);
  border-radius: 12px;
  padding: 8px;
  max-height: 420px;
  overflow-y: auto;
}

.org-bundle-picker__summary {
  position: sticky;
  top: 76px;
  border-radius: 12px;
}

.org-bundle-picker__preview {
  max-height: 240px;
  overflow-y: auto;
  font-family: monospace;
}
</style>
