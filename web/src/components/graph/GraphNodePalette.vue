<template>
  <div :class="['graph-node-palette', { 'is-dark': isDark }]">
    <div class="graph-node-palette__title">节点类型</div>
    <div class="graph-node-palette__list">
      <div
        v-for="item in nodeTypes"
        :key="item.type"
        class="graph-node-palette__item"
        draggable="true"
        @dragstart="onDragStart($event, item.type)"
      >
        <div class="graph-node-palette__icon" :style="{ background: item.style.fillColor, borderColor: item.style.borderColor }">
          <q-icon :name="item.style.icon" size="18px" :style="{ color: item.style.borderColor }" />
        </div>
        <div class="graph-node-palette__info">
          <div class="graph-node-palette__name">{{ item.style.label }}</div>
          <div class="graph-node-palette__desc">{{ item.desc }}</div>
        </div>
      </div>
    </div>
    <q-separator class="q-my-md" />
    <div class="graph-node-palette__title">State Schema</div>
    <div class="graph-node-palette__hint">在右侧属性面板中编辑工作流共享状态字段</div>
  </div>
</template>

<script setup lang="ts">
import { NODE_TYPE_STYLES, type NodeType } from "../../features/graph/types";

defineProps<{
  isDark: boolean;
}>();

const nodeTypes: Array<{ type: NodeType; style: typeof NODE_TYPE_STYLES[NodeType]; desc: string }> = [
  { type: "agent", style: NODE_TYPE_STYLES.agent, desc: "引用系统 Agent 目录中的 Agent" },
  { type: "llm", style: NODE_TYPE_STYLES.llm, desc: "轻量级 LLM 调用" },
  { type: "tool", style: NODE_TYPE_STYLES.tool, desc: "直接调用工具" },
  { type: "function", style: NODE_TYPE_STYLES.function, desc: "纯逻辑处理/数据转换" },
  { type: "router", style: NODE_TYPE_STYLES.router, desc: "条件路由，根据状态选择分支" },
  { type: "join", style: NODE_TYPE_STYLES.join, desc: "汇聚并行分支" },
  { type: "hitl", style: NODE_TYPE_STYLES.hitl, desc: "人工确认/输入，复用 Chat await_user 语义" },
];

function onDragStart(event: DragEvent, type: NodeType) {
  event.dataTransfer?.setData("application/graph-node-type", type);
  event.dataTransfer!.effectAllowed = "move";
}
</script>

<style scoped>
.graph-node-palette {
  width: 220px;
  padding: 16px 12px;
  border-right: 1px solid var(--glass-border, rgb(235 220 200 / 70%));
  background: var(--glass-surface, rgb(255 253 245 / 65%));
  backdrop-filter: blur(var(--glass-blur-default, 18px));
  -webkit-backdrop-filter: blur(var(--glass-blur-default, 18px));
  overflow-y: auto;
}

.graph-node-palette__title {
  font-size: 12px;
  font-weight: 800;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: var(--color-text-secondary, var(--color-text-secondary));
  margin-bottom: 10px;
}

.graph-node-palette__list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.graph-node-palette__item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 10px;
  border: 1px solid var(--glass-border, rgb(235 220 200 / 70%));
  border-radius: 12px;
  background: rgb(255 255 255 / 50%);
  cursor: grab;
  transition: background 0.15s, box-shadow 0.15s;
}

.graph-node-palette__item:hover {
  background: rgb(255 255 255 / 80%);
  box-shadow: 0 2px 8px rgb(0 0 0 / 6%);
}

.graph-node-palette__item:active {
  cursor: grabbing;
}

.graph-node-palette__icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border: 2px solid;
  border-radius: 10px;
  flex-shrink: 0;
}

.graph-node-palette__info {
  flex: 1;
  min-width: 0;
}

.graph-node-palette__name {
  font-size: 12px;
  font-weight: 700;
}

.graph-node-palette__desc {
  font-size: 10px;
  color: var(--color-text-secondary, var(--color-text-secondary));
  line-height: 1.3;
}

.graph-node-palette__hint {
  font-size: 11px;
  color: var(--color-text-secondary, var(--color-text-secondary));
  line-height: 1.4;
}

.graph-node-palette.is-dark {
  border-color: rgb(255 255 255 / 8%);
  background: rgb(18 24 34 / 65%);
}

.graph-node-palette.is-dark .graph-node-palette__item {
  border-color: rgb(255 255 255 / 8%);
  background: rgb(30 41 59 / 50%);
}

.graph-node-palette.is-dark .graph-node-palette__item:hover {
  background: rgb(30 41 59 / 80%);
}
</style>
