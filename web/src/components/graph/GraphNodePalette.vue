<template>
  <div :class="['graph-node-palette', { 'is-dark': isDark }]">
    <div class="graph-node-palette__head">
      <div class="graph-node-palette__title">{{ t('graphs.paletteTitle') }}</div>
      <div class="graph-node-palette__subtitle">{{ t('graphs.paletteSubtitle') }}</div>
    </div>

    <q-input
      v-model="search"
      dense
      outlined
      clearable
      class="graph-node-palette__search app-glass-control app-glass-control--sm"
      :placeholder="t('graphs.paletteSearchPlaceholder')"
    >
      <template #prepend><q-icon name="search" size="16px" /></template>
    </q-input>

    <div v-for="group in filteredGroups" :key="group.key" class="graph-node-palette__group">
      <div class="graph-node-palette__group-title">{{ t(group.labelKey) }}</div>
      <div class="graph-node-palette__list">
        <button
          v-for="item in group.items"
          :key="item.type"
          type="button"
          class="graph-node-palette__item"
          draggable="true"
          @dragstart="onDragStart($event, item.type)"
        >
          <div class="graph-node-palette__icon" :style="{ '--node-accent': item.style.borderColor }">
            <q-icon :name="item.style.icon" size="16px" />
          </div>
          <div class="graph-node-palette__info">
            <div class="graph-node-palette__name">{{ t(item.style.labelKey) }}</div>
            <div class="graph-node-palette__desc">{{ t(item.descKey) }}</div>
          </div>
        </button>
      </div>
    </div>

    <q-separator class="q-my-md" />

    <GraphTemplatePicker
      :templates="templates"
      :loading="templatesLoading"
      @request-templates="$emit('requestTemplates')"
      @create-from-template="$emit('createFromTemplate', $event)"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import GraphTemplatePicker from './GraphTemplatePicker.vue';
import { NODE_TYPE_STYLES, type NodeType, type GraphTemplateInfo } from '../../features/graph/types';

defineProps<{
  isDark: boolean;
  templates: GraphTemplateInfo[];
  templatesLoading: boolean;
}>();

defineEmits<{
  requestTemplates: [];
  createFromTemplate: [templateId: string];
}>();

const { t } = useI18n();
const search = ref('');

type PaletteItem = {
  type: NodeType;
  style: (typeof NODE_TYPE_STYLES)[NodeType];
  descKey: string;
};
type PaletteGroup = {
  key: string;
  labelKey: string;
  items: PaletteItem[];
};

const groups: PaletteGroup[] = [
  {
    key: 'agent',
    labelKey: 'graphs.paletteGroupAgent',
    items: [
      { type: 'agent', style: NODE_TYPE_STYLES.agent, descKey: 'graphs.paletteDescAgent' },
      { type: 'llm', style: NODE_TYPE_STYLES.llm, descKey: 'graphs.paletteDescLLM' },
      { type: 'tool', style: NODE_TYPE_STYLES.tool, descKey: 'graphs.paletteDescTool' },
    ],
  },
  {
    key: 'control',
    labelKey: 'graphs.paletteGroupControl',
    items: [
      { type: 'function', style: NODE_TYPE_STYLES.function, descKey: 'graphs.paletteDescFunction' },
      { type: 'router', style: NODE_TYPE_STYLES.router, descKey: 'graphs.paletteDescRouter' },
      { type: 'join', style: NODE_TYPE_STYLES.join, descKey: 'graphs.paletteDescJoin' },
      { type: 'hitl', style: NODE_TYPE_STYLES.hitl, descKey: 'graphs.paletteDescHITL' },
    ],
  },
];

const filteredGroups = computed(() => {
  const q = search.value.trim().toLowerCase();
  if (!q) return groups;
  return groups
    .map((group) => ({
      ...group,
      items: group.items.filter(
        (item) =>
          t(item.style.labelKey).toLowerCase().includes(q) ||
          t(item.descKey).toLowerCase().includes(q) ||
          item.type.includes(q),
      ),
    }))
    .filter((group) => group.items.length > 0);
});

function onDragStart(event: DragEvent, type: NodeType) {
  event.dataTransfer?.setData('application/graph-node-type', type);
  if (event.dataTransfer) {
    event.dataTransfer.effectAllowed = 'move';
  }
  const ghost = document.createElement('div');
  const style = NODE_TYPE_STYLES[type];
  ghost.textContent = t(style.labelKey);
  ghost.style.cssText = `padding:6px 14px;border-radius:8px;font-size:12px;font-weight:600;color:${style.borderColor};background:color-mix(in srgb, ${style.borderColor} 10%, var(--glass-surface));border:1px solid color-mix(in srgb, ${style.borderColor} 30%, transparent);-webkit-backdrop-filter:blur(8px);backdrop-filter:blur(8px);white-space:nowrap;position:absolute;top:-9999px;`;
  document.body.appendChild(ghost);
  event.dataTransfer?.setDragImage(ghost, ghost.offsetWidth / 2, ghost.offsetHeight / 2);
  requestAnimationFrame(() => document.body.removeChild(ghost));
}
</script>
