<template>
  <div v-if="loading" class="app-entity-grid q-mt-md">
    <q-card v-for="i in rowsPerPage" :key="i" flat bordered class="agent-card">
      <q-card-section>
        <q-skeleton type="QAvatar" size="52px" />
        <q-skeleton class="q-mt-md" type="text" />
        <q-skeleton type="text" width="70%" />
        <q-skeleton class="q-mt-md" height="72px" />
      </q-card-section>
    </q-card>
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
    <div v-if="viewMode === 'grid'" class="app-entity-grid">
      <agent-card
        v-for="agent in agents"
        :key="agent.id"
        :agent="agent"
        :favorite="isFavorite(agent.id)"
        :category-label="getCategoryLabel(agent.category_position_id)"
        :context-label="formatLastRunContext(agent)"
        :evolving="isAgentEvolving(agent)"
        @toggle-favorite="$emit('toggle-favorite', $event)"
        @copy-key="$emit('copy-key', $event)"
        @delete="$emit('delete', $event)"
        @duplicate="$emit('duplicate', $event)"
      />
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
          <q-btn flat dense rounded color="secondary" label="复制" @click="$emit('duplicate', props.row)" />
          <q-btn flat dense round color="negative" icon="delete" @click="$emit('delete', props.row)" />
        </q-td>
      </template>
    </q-table>
  </section>
</template>

<script setup lang="ts">
import type { QTableColumn } from "quasar";
import type { Agent } from "../../features/agents/types";
import AgentCard from "./AgentCard.vue";
import AgentAvatarQ from "../avatar/AgentAvatarQ.vue";
import { formatLastRunContext, isAgentEvolving } from "./agentUi";

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
  duplicate: [agent: Agent];
}>();
</script>
