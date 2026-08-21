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
      <div class="text-h6 q-mt-md">{{ keyword ? '未找到匹配的 Agent' : '暂无 Agent' }}</div>
      <div class="text-body2 text-grey-7 q-mt-sm">调整筛选条件，或创建一个新的 Agent 开始配置。</div>
      <q-btn
        class="q-mt-md"
        color="primary"
        rounded
        unelevated
        icon="add"
        label="创建 Agent"
        @click="$emit('create')"
      />
    </q-card-section>
  </q-card>

  <section v-else class="q-mt-md">
    <template v-if="viewMode === 'grid'">
      <div v-if="builtinAgents.length" class="q-mb-lg">
        <div class="text-subtitle2 text-weight-bold q-mb-sm">
          <q-icon name="verified_user" size="18px" class="q-mr-xs" />系统内置
        </div>
        <div class="app-entity-grid">
          <agent-card
            v-for="agent in builtinAgents"
            :key="agent.id"
            :agent="agent"
            :favorite="isFavorite(agent.id)"
            :taxonomy-label="getCategoryLabel(agent.taxonomy_position_id)"
            :context-label="formatLastRunContext(agent)"
            :evolving="isAgentEvolving(agent)"
            @toggle-favorite="$emit('toggle-favorite', $event)"
            @copy-key="$emit('copy-key', $event)"
            @delete="$emit('delete', $event)"
            @duplicate="$emit('duplicate', $event)"
          />
        </div>
      </div>
      <div v-if="presetAgents.length" class="q-mb-lg">
        <div class="text-subtitle2 text-weight-bold q-mb-sm">
          <q-icon name="auto_awesome" size="18px" class="q-mr-xs" />预设模板
        </div>
        <div class="app-entity-grid">
          <agent-card
            v-for="agent in presetAgents"
            :key="agent.id"
            :agent="agent"
            :favorite="isFavorite(agent.id)"
            :taxonomy-label="getCategoryLabel(agent.taxonomy_position_id)"
            :context-label="formatLastRunContext(agent)"
            :evolving="isAgentEvolving(agent)"
            @toggle-favorite="$emit('toggle-favorite', $event)"
            @copy-key="$emit('copy-key', $event)"
            @delete="$emit('delete', $event)"
            @duplicate="$emit('duplicate', $event)"
          />
        </div>
      </div>
      <div v-if="userAgents.length">
        <div v-if="builtinAgents.length || presetAgents.length" class="text-subtitle2 text-weight-bold q-mb-sm">
          <q-icon name="person" size="18px" class="q-mr-xs" />我的 Agent
        </div>
        <draggable
          v-model="draggableUserAgents"
          item-key="id"
          class="agents-draggable-grid"
          ghost-class="agent-card--ghost"
          chosen-class="agent-card--chosen"
          drag-class="agent-card--dragging"
          :animation="200"
          :delay="100"
          :disabled="true"
        >
          <template #item="{ element: agent }">
            <agent-card
              :agent="agent"
              :favorite="isFavorite(agent.id)"
              :taxonomy-label="getCategoryLabel(agent.taxonomy_position_id)"
              :context-label="formatLastRunContext(agent)"
              :evolving="isAgentEvolving(agent)"
              selectable
              :selected="selectedIds.includes(agent.id)"
              @toggle-select="$emit('toggle-select', $event)"
              @toggle-favorite="$emit('toggle-favorite', $event)"
              @copy-key="$emit('copy-key', $event)"
              @delete="$emit('delete', $event)"
              @duplicate="$emit('duplicate', $event)"
            />
          </template>
        </draggable>
      </div>
    </template>

    <AppRegistryTable
      v-else
      class="agents-table"
      :rows="agents"
      :columns="tableColumns"
      row-key="id"
      hide-pagination
      :pagination="{ rowsPerPage: 0 }"
    >
      <template #body-cell-name="slotProps">
        <q-td :props="slotProps">
          <div class="row items-center no-wrap q-gutter-sm">
            <q-checkbox
              v-if="!slotProps.row.readonly"
              dense
              :model-value="selectedIds.includes(slotProps.row.id)"
              :aria-label="t('agentsPage.batch.selectAriaLabel')"
              @update:model-value="$emit('toggle-select', slotProps.row.id)"
            />
            <q-btn
              v-if="!slotProps.row.readonly"
              flat
              dense
              round
              size="sm"
              :color="isFavorite(slotProps.row.id) ? 'amber-8' : 'grey-5'"
              :icon="isFavorite(slotProps.row.id) ? 'star' : 'star_border'"
              :aria-label="
                isFavorite(slotProps.row.id) ? t('agentsPage.actions.unfavorite') : t('agentsPage.actions.favorite')
              "
              @click="$emit('toggle-favorite', slotProps.row.id)"
            >
              <q-tooltip>{{
                isFavorite(slotProps.row.id) ? t('agentsPage.actions.unfavorite') : t('agentsPage.actions.favorite')
              }}</q-tooltip>
            </q-btn>
            <agent-avatar-q :icon="slotProps.row.icon" :alt="slotProps.row.display_name" size="36px" />
            <div class="min-width-0">
              <div class="app-registry-cell-primary ellipsis">
                {{ slotProps.row.display_name || getCategoryLabel(slotProps.row.taxonomy_position_id) }}
              </div>
              <button type="button" class="agent-handle ellipsis" @click="$emit('copy-key', slotProps.row.agent_key)">
                {{ slotProps.row.agent_key }}
              </button>
            </div>
          </div>
        </q-td>
      </template>
      <template #body-cell-model="slotProps">
        <q-td :props="slotProps">
          <span class="app-registry-cell-sub ellipsis">{{ slotProps.row.provider }} / {{ slotProps.row.model }}</span>
        </q-td>
      </template>
      <template #body-cell-status="slotProps">
        <q-td :props="slotProps">
          <div class="row items-center no-wrap q-gutter-xs">
            <q-badge rounded :color="slotProps.row.status === 'active' ? 'positive' : 'grey'">{{
              statusLabel(slotProps.row.status)
            }}</q-badge>
            <q-chip v-if="slotProps.row.readonly" dense square class="agent-card__readonly-chip" icon="verified_user">
              {{ t('agentsPage.actions.builtin') }}
              <q-tooltip>{{ t('agentsPage.actions.builtinTip') }}</q-tooltip>
            </q-chip>
          </div>
        </q-td>
      </template>
      <template #body-cell-memory_mode="slotProps">
        <q-td :props="slotProps">
          <q-badge
            rounded
            :color="
              slotProps.value === 'working_memory'
                ? 'info'
                : slotProps.value === 'framework_memory'
                  ? 'warning'
                  : 'positive'
            "
            >{{ MEMORY_TOOL_MODE_LABELS[slotProps.value as keyof typeof MEMORY_TOOL_MODE_LABELS] }}</q-badge
          >
        </q-td>
      </template>
      <template #body-cell-actions="slotProps">
        <q-td :props="slotProps">
          <div class="app-registry-cell-actions">
            <q-btn
              flat
              dense
              round
              color="primary"
              icon="edit"
              :aria-label="t('agentsPage.actions.edit')"
              :to="`/agents/${slotProps.row.id}/settings`"
            >
              <q-tooltip>{{ t('agentsPage.actions.edit') }}</q-tooltip>
            </q-btn>
            <q-btn
              v-if="!slotProps.row.readonly"
              flat
              dense
              round
              color="secondary"
              icon="content_copy"
              :aria-label="t('agentsPage.actions.duplicate')"
              @click="$emit('duplicate', slotProps.row)"
            >
              <q-tooltip>{{ t('agentsPage.actions.duplicate') }}</q-tooltip>
            </q-btn>
            <q-btn
              v-if="!slotProps.row.readonly"
              flat
              dense
              round
              color="negative"
              icon="delete"
              :aria-label="t('agentsPage.actions.delete')"
              @click="$emit('delete', slotProps.row)"
            >
              <q-tooltip>{{ t('agentsPage.actions.delete') }}</q-tooltip>
            </q-btn>
          </div>
        </q-td>
      </template>
    </AppRegistryTable>
  </section>

  <q-card v-if="selectedIds.length > 0" flat bordered class="agents-batch-bar">
    <span class="text-body2">{{ t('agentsPage.batch.selected', { n: selectedIds.length }) }}</span>
    <div class="row items-center q-gutter-xs">
      <q-btn
        flat
        dense
        rounded
        no-caps
        color="positive"
        icon="play_circle"
        :label="t('agentsPage.batch.enable')"
        @click="$emit('batch-enable')"
      />
      <q-btn
        flat
        dense
        rounded
        no-caps
        color="warning"
        icon="pause_circle"
        :label="t('agentsPage.batch.disable')"
        @click="$emit('batch-disable')"
      />
      <q-btn
        flat
        dense
        rounded
        no-caps
        color="negative"
        icon="delete"
        :label="t('agentsPage.batch.delete')"
        @click="$emit('batch-delete')"
      />
      <q-btn
        flat
        dense
        rounded
        no-caps
        color="grey-7"
        icon="close"
        :label="t('agentsPage.batch.cancel')"
        @click="$emit('clear-selection')"
      />
    </div>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { QTableColumn } from 'quasar';
import draggable from 'vuedraggable';
import type { Agent } from '../../features/agents/types';
import AgentCard from './AgentCard.vue';
import AgentAvatarQ from '../avatar/AgentAvatarQ.vue';
import AppRegistryTable from '../layout/AppRegistryTable.vue';
import { formatLastRunContext, isAgentEvolving, statusLabel, MEMORY_TOOL_MODE_LABELS } from './agentUi';

const { t } = useI18n();

type ViewMode = 'grid' | 'list';

const props = defineProps<{
  loading: boolean;
  agents: Agent[];
  keyword: string;
  viewMode: ViewMode;
  rowsPerPage: number;
  tableColumns: QTableColumn<Agent>[];
  isFavorite: (id: string) => boolean;
  getCategoryLabel: (taxonomyPositionId: string) => string;
  selectedIds: string[];
}>();

const emit = defineEmits<{
  create: [];
  'toggle-favorite': [id: string];
  'toggle-select': [id: string];
  'copy-key': [key: string];
  delete: [agent: Agent];
  duplicate: [agent: Agent];
  reorder: [ids: string[]];
  'batch-enable': [];
  'batch-disable': [];
  'batch-delete': [];
  'clear-selection': [];
}>();

// 内置管家 = system_builtin 且非 dept_lead（与后端 CleanupNonSystemData 的保留规则一致）：
// 仅精灵助手/系统管家/记忆管家/技能管家 4 个核心管家；26 个部门主管归入预设模板区。
const isDeptLead = (a: Agent) => a.agent_variant === 'dept_lead';
const builtinAgents = computed(() => props.agents.filter((a) => a.readonly && a.kind === 'system_builtin' && !isDeptLead(a)));
const presetAgents = computed(() =>
  props.agents.filter((a) => isDeptLead(a) || (!a.readonly && a.kind === 'ecosystem_preset')),
);
const userAgents = computed(() =>
  props.agents.filter((a) => !a.readonly && a.kind !== 'ecosystem_preset' && !isDeptLead(a)),
);

const draggableUserAgents = computed({
  get: () => userAgents.value,
  set: (value: Agent[]) => {
    const ids = value.map((a) => a.id);
    emit('reorder', ids);
  },
});
</script>

<style scoped>
.agents-batch-bar {
  position: fixed;
  bottom: 24px;
  left: 50%;
  transform: translateX(-50%);
  z-index: 2000;
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 8px 16px;
  border-radius: 12px;
}
</style>
