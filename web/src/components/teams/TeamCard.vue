<!--
  Team 域展示组件：仅 props / emits（aranea-frontend-guide SKILL §1 红线 #1）。
  路径约定：SKILL §3.3 → `web/src/components/teams/`。
-->
<template>
  <q-card v-liquid-glow flat bordered :class="['team-card full-height', { 'is-dark': isDark }]">
    <div class="team-card__inner">
      <header class="team-card__head">
        <div class="team-card__head-main min-width-0">
          <div class="team-card__title-row">
            <h3 class="team-card__name ellipsis">{{ team.display_name }}</h3>
            <KindBadge :kind="team.kind" />
            <q-chip
              v-if="definition.members.length"
              dense
              square
              size="sm"
              color="primary"
              text-color="white"
              class="team-card__mode-chip"
            >
              {{ teamModeLabel(definition.mode) }}
            </q-chip>
            <q-chip v-if="team.is_default" dense square size="sm" color="amber" text-color="black">默认</q-chip>
          </div>
        </div>
        <div class="team-card__badges">
          <q-badge v-if="team.has_active_run" rounded color="positive" class="team-card__live-badge">
            <q-icon name="fiber_manual_record" size="8px" class="q-mr-xs" />运行中
          </q-badge>
          <AppStatusChip :status="team.status" />
        </div>
      </header>

      <div class="team-card__meta">
        <p class="team-description">{{ definition.description || '暂无说明' }}</p>
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
              <span class="member-role">{{ teamRoleLabel(member.role) }}</span>
              <q-badge v-if="isTeamLeader(member)" dense rounded color="deep-orange" text-color="white" class="q-ml-xs"
                >队长</q-badge
              >
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
          <q-btn flat dense round size="sm" color="primary" icon="play_arrow" :to="`/chat?team=${team.id}`">
            <q-tooltip>进入 Chat</q-tooltip>
          </q-btn>
          <q-btn flat dense round size="sm" color="primary" icon="account_tree" :to="`/teams/${team.id}/orchestrate`">
            <q-tooltip>查看编排</q-tooltip>
          </q-btn>
          <q-btn flat dense round size="sm" color="primary" icon="edit" @click="$emit('edit', team)">
            <q-tooltip>编辑</q-tooltip>
          </q-btn>
          <q-btn flat dense round size="sm" icon="more_vert">
            <q-tooltip>更多操作</q-tooltip>
            <q-menu auto-close anchor="bottom right" self="top right">
              <q-list dense class="app-menu-min-160">
                <q-item clickable @click="$emit('openRuns', team)">
                  <q-item-section side><q-icon name="timeline" size="xs" /></q-item-section>
                  <q-item-section>运行轨迹</q-item-section>
                </q-item>
                <q-item v-if="team.has_active_run" clickable @click="$emit('openObservatory', team)">
                  <q-item-section side><q-icon name="insights" size="xs" /></q-item-section>
                  <q-item-section>运行观测台</q-item-section>
                </q-item>
                <q-item clickable @click="$emit('runTest', team)">
                  <q-item-section side><q-icon name="science" size="xs" /></q-item-section>
                  <q-item-section>运行测试（API）</q-item-section>
                </q-item>
                <q-item v-if="team.kind !== 'system_builtin'" clickable @click="$emit('duplicate', team)">
                  <q-item-section side><q-icon name="content_copy" size="xs" /></q-item-section>
                  <q-item-section>复制</q-item-section>
                </q-item>
                <q-item v-if="canRetry" clickable @click="$emit('retry', team)">
                  <q-item-section side><q-icon name="replay" size="xs" color="warning" /></q-item-section>
                  <q-item-section>重试（重置为待执行）</q-item-section>
                </q-item>
                <template v-if="team.kind !== 'system_builtin'">
                  <q-separator />
                  <q-item
                    clickable
                    class="text-negative"
                    :disable="team.is_default || !!team.readonly"
                    @click="$emit('remove', team)"
                  >
                    <q-item-section side><q-icon name="delete" size="xs" color="negative" /></q-item-section>
                    <q-item-section>{{ $t('common.delete') }}</q-item-section>
                  </q-item>
                </template>
              </q-list>
            </q-menu>
          </q-btn>
          <q-chip v-if="team.readonly" dense square size="sm" icon="verified_user">{{ t('teamsPage.builtin') }}</q-chip>
        </div>
      </footer>
    </div>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { Agent } from '../../features/agents/types';
import type { Team } from '../../features/teams/types';
import {
  agentName,
  formatDate,
  memberIcon,
  parseDefinition,
  teamModeLabel,
  teamRoleLabel,
} from './teamUtils';
import KindBadge from '../agents/KindBadge.vue';
import AppStatusChip from '../common/AppStatusChip.vue';

const props = defineProps<{
  team: Team;
  agents: Agent[];
  isDark: boolean;
}>();

const { t } = useI18n();

defineEmits<{
  openRuns: [team: Team];
  openObservatory: [team: Team];
  runTest: [team: Team];
  duplicate: [team: Team];
  edit: [team: Team];
  remove: [team: Team];
  retry: [team: Team];
}>();

const definition = computed(() => parseDefinition(props.team));

/** failed/cancelled teams can be reset to pending via RetryTeam RPC (backend state machine recover). */
const canRetry = computed(() => ['failed', 'cancelled'].includes(props.team.status) && !props.team.readonly);

type TeamMember = ReturnType<typeof parseDefinition>['members'][number];

/**
 * 队长标识（消除「看不出谁是 leader」的感知困惑）：
 * - 任意模式下 role=coordinator 的协调者；
 * - coordinator 模式首位启用成员（遗留 spirit 团队首位角色为 synthesizer，
 *   见 buildSpiritTeamDefinitionJSON）；
 * - parallel 模式的汇总者（synthesizer）。
 * sequential / critic_loop / swarm / adaptive 无单一 leader，不显示。
 */
function isTeamLeader(member: TeamMember): boolean {
  const role = String(member.role || '').toLowerCase();
  if (role === 'coordinator') return true;
  const mode = String(definition.value.mode || 'sequential').toLowerCase();
  if (mode === 'parallel') return role === 'synthesizer';
  if (mode === 'coordinator' && role === 'synthesizer') {
    const firstEnabled = definition.value.members.find((m) => m.enabled !== false);
    return firstEnabled === member;
  }
  return false;
}
</script>
