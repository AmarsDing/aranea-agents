<template>
  <div v-if="loading" class="row q-col-gutter-md q-mt-md">
    <div v-for="i in rowsPerPage" :key="i" class="col-12 col-sm-6 col-lg-4">
      <q-card flat bordered class="agent-card">
        <q-card-section>
          <q-skeleton type="QAvatar" size="52px" />
          <q-skeleton class="q-mt-md" type="text" />
          <q-skeleton type="text" width="70%" />
          <q-skeleton class="q-mt-md" height="72px" />
        </q-card-section>
      </q-card>
    </div>
  </div>

  <q-card v-else-if="agents.length === 0" flat bordered class="empty-agent-card q-mt-md">
    <q-card-section class="empty-agent-state column items-center text-center">
      <div class="empty-agent-visual">
        <q-avatar size="72px" color="primary" text-color="white" icon="smart_toy" />
      </div>
      <div class="text-h6 q-mt-md">{{ keyword ? "未找到匹配的 Agent" : "暂无 Agent" }}</div>
      <div class="text-body2 text-grey-7 q-mt-sm">调整筛选条件，或创建一个新的 Agent 开始配置。</div>
      <q-btn class="q-mt-md" color="primary" rounded unelevated icon="add" label="创建 Agent" @click="$emit('create')" />
    </q-card-section>
  </q-card>

  <section v-else class="q-mt-md">
    <div v-if="viewMode === 'grid'" class="row q-col-gutter-md">
      <div v-for="agent in agents" :key="agent.id" class="col-12 col-sm-6 col-lg-4">
        <agent-card
          :agent="agent"
          :favorite="isFavorite(agent.id)"
          :category-label="getCategoryLabel(agent.category_position_id)"
          :context-label="formatContext(agent.context_window)"
          :evolving="selfEvolveEnabled(agent)"
          @toggle-favorite="$emit('toggle-favorite', $event)"
          @copy-key="$emit('copy-key', $event)"
          @delete="$emit('delete', $event)"
        />
      </div>
    </div>

    <q-table
      v-else
      class="agents-table"
      flat
      bordered
      :rows="agents"
      :columns="tableColumns"
      row-key="id"
      hide-pagination
    >
      <template #body-cell-name="props">
        <q-td :props="props">
          <div class="row items-center no-wrap q-gutter-sm">
            <q-btn
              flat
              dense
              round
              size="sm"
              :color="isFavorite(props.row.id) ? 'amber-8' : 'grey-5'"
              :icon="isFavorite(props.row.id) ? 'star' : 'star_border'"
              @click="$emit('toggle-favorite', props.row.id)"
            />
            <agent-avatar-q :icon="props.row.icon" :alt="props.row.display_name" size="36px" />
            <div>
              <div class="text-weight-medium">{{ props.row.display_name }}</div>
              <button type="button" class="agent-handle" @click="$emit('copy-key', props.row.agent_key)">{{ props.row.agent_key }}</button>
            </div>
          </div>
        </q-td>
      </template>
      <template #body-cell-status="props">
        <q-td :props="props">
          <q-badge rounded :color="props.row.status === 'active' ? 'positive' : 'grey'">{{ props.row.status }}</q-badge>
        </q-td>
      </template>
      <template #body-cell-actions="props">
        <q-td :props="props" class="q-gutter-xs">
          <q-btn flat dense rounded color="primary" label="设置" :to="`/agents/${props.row.id}/settings`" />
          <q-btn flat dense round color="negative" icon="delete" @click="$emit('delete', props.row)" />
        </q-td>
      </template>
    </q-table>
  </section>
</template>

<script setup lang="ts">
import type { QTableColumn } from "quasar";
import type { Agent } from "../../features/agents/api";
import AgentCard from "./AgentCard.vue";
import AgentAvatarQ from "../avatar/AgentAvatarQ.vue";
import { formatContext, selfEvolveEnabled } from "./agentUi";

type ViewMode = "grid" | "list";

defineProps<{
  loading: boolean;
  agents: Agent[];
  keyword: string;
  viewMode: ViewMode;
  rowsPerPage: number;
  tableColumns: QTableColumn<Agent>[];
  isFavorite: (id: string) => boolean;
  getCategoryLabel: (categoryPositionId: string) => string;
}>();

defineEmits<{
  create: [];
  "toggle-favorite": [id: string];
  "copy-key": [key: string];
  delete: [agent: Agent];
}>();
</script>

<style scoped>
.agent-handle {
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--color-text-tertiary);
  cursor: pointer;
  font-size: 12px;
}

.empty-agent-card,
.agents-table {
  border-radius: 24px;
  border-color: rgb(15 23 42 / 8%);
  box-shadow: 0 18px 48px rgb(16 24 40 / 5%);
}

.empty-agent-card {
  overflow: hidden;
  background:
    radial-gradient(circle at center 26%, rgb(25 118 210 / 8%), transparent 22%),
    linear-gradient(180deg, var(--color-on-accent), var(--color-page-tint));
}

.empty-agent-state {
  min-height: 230px;
  padding: 36px 24px;
}

.empty-agent-visual {
  display: grid;
  place-items: center;
  width: 108px;
  height: 108px;
  border: 1px solid rgb(25 118 210 / 12%);
  border-radius: 32px;
  background:
    linear-gradient(180deg, rgb(255 255 255 / 90%), rgb(238 246 255 / 90%)),
    radial-gradient(circle at top, rgb(25 118 210 / 16%), transparent 55%);
  box-shadow: 0 18px 42px rgb(16 24 40 / 8%);
}

.agents-table :deep(th) {
  color: var(--color-text-tertiary);
  font-size: 12px;
  font-weight: 700;
  background: var(--color-surface-soft);
}

.agents-table :deep(td) {
  color: var(--color-text-gray-600-alt);
}
</style>
