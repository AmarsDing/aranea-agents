<template>
  <figure class="tm-flow" :aria-label="t('teamsHub.structure')">
    <figcaption class="tm-flow__mode">
      <q-icon :name="modeIcon" size="16px" />
      <strong>{{ teamModeLabel(mode) }}</strong>
      <span class="tm-flow__hint">{{ modeHint }}</span>
    </figcaption>

    <p v-if="!members.length" class="org-note">{{ t('teamsPage.orchestrateNoMembers') }}</p>

    <!-- 顺序：一条链 -->
    <div v-else-if="mode === 'sequential'" class="tm-flow__lane">
      <template v-for="(m, i) in members" :key="m.key">
        <q-icon v-if="i > 0" name="south" size="14px" class="tm-flow__arrow" />
        <div class="tm-flow__node" :class="{ 'tm-flow__node--off': !m.enabled }">
          <span class="tm-avatar">{{ m.initial }}</span>
          <span class="tm-flow__name">{{ m.name }}</span>
          <span class="tm-flow__role org-mono">{{ i + 1 }}</span>
        </div>
      </template>
    </div>

    <!-- 生成评审：生成 ⇄ 评审 -->
    <div v-else-if="mode === 'critic_loop'" class="tm-flow__lane">
      <div
        v-for="m in byRole(['generator', 'critic'])"
        :key="m.key"
        class="tm-flow__node"
        :class="{ 'tm-flow__node--off': !m.enabled }"
      >
        <span class="tm-avatar">{{ m.initial }}</span>
        <span class="tm-flow__name">{{ m.name }}</span>
        <span class="tm-flow__role">{{ teamRoleLabel(m.role) }}</span>
      </div>
      <span class="tm-flow__mode"><q-icon name="sync" size="14px" />{{ t('teamsHub.loopHint', { n: loopMax }) }}</span>
      <template v-for="m in byRole(['synthesizer'])" :key="m.key">
        <q-icon name="south" size="14px" class="tm-flow__arrow" />
        <div class="tm-flow__node" :class="{ 'tm-flow__node--off': !m.enabled }">
          <span class="tm-avatar">{{ m.initial }}</span>
          <span class="tm-flow__name">{{ m.name }}</span>
          <span class="tm-flow__role">{{ teamRoleLabel(m.role) }}</span>
        </div>
      </template>
    </div>

    <!-- 并行 / 主控 / 群智：入口（主控）→ 一组并列成员 → 汇总 -->
    <div v-else class="tm-flow__lane">
      <template v-for="m in heads" :key="m.key">
        <div class="tm-flow__node" :class="{ 'tm-flow__node--off': !m.enabled }">
          <span class="tm-avatar">{{ m.initial }}</span>
          <span class="tm-flow__name">{{ m.name }}</span>
          <span class="tm-flow__role">{{ teamRoleLabel(m.role) }}</span>
        </div>
        <q-icon name="south" size="14px" class="tm-flow__arrow" />
      </template>
      <div class="tm-flow__fan">
        <div v-for="m in workers" :key="m.key" class="tm-flow__node" :class="{ 'tm-flow__node--off': !m.enabled }">
          <span class="tm-avatar">{{ m.initial }}</span>
          <span class="tm-flow__name">{{ m.name }}</span>
        </div>
      </div>
      <template v-for="m in tails" :key="m.key">
        <q-icon name="south" size="14px" class="tm-flow__arrow" />
        <div class="tm-flow__node" :class="{ 'tm-flow__node--off': !m.enabled }">
          <span class="tm-avatar">{{ m.initial }}</span>
          <span class="tm-flow__name">{{ m.name }}</span>
          <span class="tm-flow__role">{{ teamRoleLabel(m.role) }}</span>
        </div>
      </template>
    </div>
  </figure>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { agentName, teamModeLabel, teamRoleLabel } from './teamUtils';
import type { Agent } from '../../features/agents/types';
import type { TeamDefinition } from '../../features/teams/types';

const props = defineProps<{ definition: TeamDefinition; agents: Agent[] }>();
const { t } = useI18n();

const mode = computed(() => String(props.definition.mode || 'sequential').toLowerCase());

const members = computed(() =>
  [...props.definition.members]
    .sort((a, b) => (a.sort_order ?? 0) - (b.sort_order ?? 0))
    .map((m, i) => {
      const name = m.name || agentName(props.agents, m.agent_id);
      return {
        key: `${m.agent_id}:${i}`,
        agentId: m.agent_id,
        name,
        initial: name.trim().slice(0, 1).toUpperCase() || '?',
        role: String(m.role || 'worker'),
        enabled: m.enabled !== false,
      };
    }),
);

function byRole(roles: string[]) {
  return members.value.filter((m) => roles.includes(m.role));
}

const synthId = computed(() => String(props.definition.synthesizer_agent_id || '').trim());
const heads = computed(() => (mode.value === 'coordinator' ? byRole(['coordinator']) : []));
const tails = computed(() =>
  members.value.filter((m) => m.role === 'synthesizer' || (synthId.value && m.agentId === synthId.value)),
);
const workers = computed(() => members.value.filter((m) => !heads.value.includes(m) && !tails.value.includes(m)));

const loopMax = computed(() => props.definition.critic_loop?.max_iterations ?? 3);

const modeIcon = computed(
  () =>
    ({ sequential: 'trending_flat', parallel: 'call_split', coordinator: 'account_tree', critic_loop: 'sync' })[
      mode.value
    ] ?? 'hub',
);

const modeHint = computed(() => {
  const key = {
    sequential: 'modeHintSequential',
    parallel: 'modeHintParallel',
    coordinator: 'modeHintCoordinator',
    critic_loop: 'modeHintCriticLoop',
  }[mode.value];
  return t(`teamsPage.${key ?? 'modeHintAdaptive'}`);
});
</script>
