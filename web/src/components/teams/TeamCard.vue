<!--
  Team 域展示组件：仅 props / emits（aranea-frontend-guide SKILL §1 红线 #1）。
  路径约定：SKILL §3.3 → `web/src/components/teams/`。
-->
<template>
  <q-card flat bordered :class="['team-card full-height', { 'is-dark': isDark }]">
    <div class="team-card__inner">
      <header class="team-card__head">
        <div class="team-card__head-main min-width-0">
          <div class="team-card__title-row">
            <h3 class="team-card__name ellipsis">{{ team.display_name }}</h3>
            <q-chip dense square size="sm" color="primary" text-color="white" class="team-card__mode-chip">
              {{ definition.mode }}
            </q-chip>
            <q-chip v-if="team.is_default" dense square size="sm" color="amber" text-color="black">默认</q-chip>
          </div>
          <button type="button" class="team-key" @click="$emit('copyKey', team.team_key)">{{ team.team_key }}</button>
        </div>
        <q-badge rounded class="team-card__status" :color="team.status === 'active' ? 'positive' : 'grey'">
          {{ team.status }}
        </q-badge>
      </header>

      <div class="team-card__meta">
        <p class="team-description">{{ definition.description || '暂无说明' }}</p>
        <div v-if="topologyNodes.length" class="topology-strip">
          <div v-for="node in topologyNodes" :key="node.label" class="topology-node">
            <q-icon :name="node.icon" size="14px" />
            <span>{{ node.label }}</span>
          </div>
        </div>
      </div>

      <div v-if="definition.members.length" class="team-card__members">
        <div class="member-list">
          <div
            v-for="member in definition.members"
            :key="`${team.id}-${member.agent_id}-${member.role}`"
            class="member-row"
          >
            <q-avatar size="26px" color="primary" text-color="white" :icon="memberIcon(member.role)" />
            <div class="member-primary ellipsis">
              <span class="member-role">{{ member.role }}</span>
              <span class="member-sep">·</span>
              <span class="member-label">{{ member.name || agentName(agents, member.agent_id) }}</span>
            </div>
            <q-badge dense rounded class="member-row__badge" :color="member.enabled ? 'positive' : 'grey'">
              {{ member.enabled ? '启用' : '停用' }}
            </q-badge>
          </div>
        </div>
      </div>
      <div v-else class="team-empty team-card__members">尚未配置成员 Agent。</div>

      <footer class="team-card__foot">
        <span class="team-card__foot-meta"
          >成员 {{ definition.members.length }} · {{ formatDate(team.updated_at) }}</span
        >
        <div class="team-card__action-group">
          <q-btn flat dense round size="sm" color="primary" icon="account_tree" :to="`/teams/${team.id}/orchestrate`">
            <q-tooltip>编排 Graph</q-tooltip>
          </q-btn>
          <q-btn
            v-if="team.has_active_run"
            flat
            dense
            round
            size="sm"
            color="primary"
            icon="insights"
            @click="$emit('openObservatory', team)"
          >
            <q-tooltip>运行观测台</q-tooltip>
          </q-btn>
          <q-btn flat dense round size="sm" color="primary" icon="science" @click="$emit('runTest', team)">
            <q-tooltip>运行测试（API）</q-tooltip>
          </q-btn>
          <q-btn flat dense round size="sm" color="primary" icon="play_arrow" :to="`/chat?team=${team.id}`">
            <q-tooltip>进入 Chat 测试</q-tooltip>
          </q-btn>
          <q-btn flat dense round size="sm" color="primary" icon="timeline" @click="$emit('openRuns', team)">
            <q-tooltip>查看运行轨迹</q-tooltip>
          </q-btn>
          <q-btn flat dense round size="sm" color="primary" icon="content_copy" @click="$emit('duplicate', team)" />
          <q-btn flat dense round size="sm" color="primary" icon="edit" @click="$emit('edit', team)" />
          <q-btn
            flat
            dense
            round
            size="sm"
            color="negative"
            icon="delete"
            :disable="team.is_default || !!team.readonly"
            @click="$emit('remove', team)"
          />
          <q-chip v-if="team.readonly" dense square size="sm" icon="verified_user">内置</q-chip>
        </div>
      </footer>
    </div>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { Agent } from '../../features/agents/types';
import type { Team } from '../../features/teams/types';
import { agentName, formatDate, memberIcon, parseDefinition, topologyNodesFromDefinition } from './teamUtils';

const props = defineProps<{
  team: Team;
  agents: Agent[];
  isDark: boolean;
}>();

defineEmits<{
  copyKey: [value: string];
  openRuns: [team: Team];
  openObservatory: [team: Team];
  runTest: [team: Team];
  duplicate: [team: Team];
  edit: [team: Team];
  remove: [team: Team];
}>();

const definition = computed(() => parseDefinition(props.team));
const topologyNodes = computed(() => topologyNodesFromDefinition(definition.value));
</script>
