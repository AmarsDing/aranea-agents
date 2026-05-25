<template>
  <div :class="['graph-node-palette', { 'is-dark': isDark }]">
    <div class="graph-node-palette__head">
      <div class="graph-node-palette__title">组件库</div>
      <div class="graph-node-palette__subtitle">拖拽到画布添加节点</div>
    </div>

    <q-input
      v-model="search"
      dense
      outlined
      clearable
      class="graph-node-palette__search app-glass-control app-glass-control--sm"
      placeholder="搜索节点类型…"
    >
      <template #prepend><q-icon name="search" size="16px" /></template>
    </q-input>

    <div v-for="group in filteredGroups" :key="group.key" class="graph-node-palette__group">
      <div class="graph-node-palette__group-title">{{ group.label }}</div>
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
            <div class="graph-node-palette__name">{{ item.style.label }}</div>
            <div class="graph-node-palette__desc">{{ item.desc }}</div>
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
import { computed, ref } from "vue";
import GraphTemplatePicker from "./GraphTemplatePicker.vue";
import { NODE_TYPE_STYLES, type NodeType, type GraphTemplateInfo } from "../../features/graph/types";

defineProps<{
  isDark: boolean;
  templates: GraphTemplateInfo[];
  templatesLoading: boolean;
}>();

defineEmits<{
  requestTemplates: [];
  createFromTemplate: [templateId: string];
}>();

const search = ref("");

const groups: Array<{
  key: string;
  label: string;
  items: Array<{ type: NodeType; style: typeof NODE_TYPE_STYLES[NodeType]; desc: string }>;
}> = [
  {
    key: "agent",
    label: "智能体",
    items: [
      { type: "agent", style: NODE_TYPE_STYLES.agent, desc: "引用 Agent 目录中的 Agent" },
      { type: "llm", style: NODE_TYPE_STYLES.llm, desc: "轻量级 LLM 调用" },
      { type: "tool", style: NODE_TYPE_STYLES.tool, desc: "直接调用工具" },
    ],
  },
  {
    key: "control",
    label: "控制流",
    items: [
      { type: "function", style: NODE_TYPE_STYLES.function, desc: "纯逻辑 / 数据转换" },
      { type: "router", style: NODE_TYPE_STYLES.router, desc: "条件路由分支" },
      { type: "join", style: NODE_TYPE_STYLES.join, desc: "汇聚并行分支" },
      { type: "hitl", style: NODE_TYPE_STYLES.hitl, desc: "人工确认 / 审批" },
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
          item.style.label.toLowerCase().includes(q) ||
          item.desc.toLowerCase().includes(q) ||
          item.type.includes(q),
      ),
    }))
    .filter((group) => group.items.length > 0);
});

function onDragStart(event: DragEvent, type: NodeType) {
  event.dataTransfer?.setData("application/graph-node-type", type);
  if (event.dataTransfer) {
    event.dataTransfer.effectAllowed = "move";
  }
}
</script>
