<template>
  <div class="ar-insp__body" :aria-label="t('agentRoster.insp.aria')">
    <header class="ar-insp__head">
      <agent-avatar-q :icon="agent.icon" :alt="agent.display_name" size="44px" />
      <div class="ar-insp__title">
        <div class="ar-insp__name">{{ agent.display_name || agent.agent_key }}</div>
        <button
          type="button"
          class="ar-insp__key ar-mono"
          :title="t('agentRoster.actions.copyKey')"
          @click="$emit('copy-key', agent.agent_key)"
        >
          {{ agent.agent_key }} <q-icon name="content_copy" size="12px" aria-hidden="true" />
        </button>
      </div>
      <q-btn
        flat
        dense
        round
        size="sm"
        icon="close"
        :aria-label="t('agentRoster.insp.close')"
        @click="$emit('close')"
      />
    </header>

    <div class="ar-insp__tags">
      <span class="ar-role" :class="`ar-role--${role}`" :title="t(`agentRoster.roleHint.${role}`)">{{
        t(`agentRoster.role.${role}`)
      }}</span>
      <span class="ar-st" :class="`ar-st--${agent.status}`">
        <span class="ar-dot" aria-hidden="true" />{{
          t(`agentRoster.status.${agent.status === 'inactive' ? 'inactive' : 'active'}`)
        }}
      </span>
      <span v-if="agent.agent_kind === 'a2a_proxy'" class="ar-chip">{{ t('agentRoster.insp.a2aProxy') }}</span>
    </div>

    <p class="ar-insp__desc">{{ agent.agent_description || t('agentRoster.insp.noDescription') }}</p>

    <div class="ar-insp__actions">
      <q-btn
        unelevated
        no-caps
        color="primary"
        icon="tune"
        :label="t('agentRoster.insp.openSettings')"
        :to="settingsLink()"
      />
      <q-btn
        flat
        dense
        round
        :icon="agent.is_favorite ? 'star' : 'star_border'"
        :class="{ 'ar-star': agent.is_favorite }"
        :aria-label="t(agent.is_favorite ? 'agentRoster.actions.unfavorite' : 'agentRoster.actions.favorite')"
        @click="$emit('toggle-favorite', agent)"
      >
        <q-tooltip>{{
          t(agent.is_favorite ? 'agentRoster.actions.unfavorite' : 'agentRoster.actions.favorite')
        }}</q-tooltip>
      </q-btn>
      <q-btn
        flat
        dense
        round
        icon="content_copy"
        :aria-label="t('agentRoster.actions.duplicate')"
        @click="$emit('duplicate', agent)"
      >
        <q-tooltip>{{ t('agentRoster.actions.duplicate') }}</q-tooltip>
      </q-btn>
      <q-btn
        v-if="!agent.readonly"
        flat
        dense
        round
        :icon="agent.status === 'inactive' ? 'play_circle' : 'pause_circle'"
        :aria-label="t(agent.status === 'inactive' ? 'agentRoster.actions.enable' : 'agentRoster.actions.disable')"
        @click="$emit('set-status', agent, agent.status === 'inactive' ? 'active' : 'inactive')"
      >
        <q-tooltip>{{
          t(agent.status === 'inactive' ? 'agentRoster.actions.enable' : 'agentRoster.actions.disable')
        }}</q-tooltip>
      </q-btn>
      <q-btn
        v-if="deletable"
        flat
        dense
        round
        icon="delete"
        class="ar-danger"
        :aria-label="t('agentRoster.actions.delete')"
        @click="$emit('delete', agent)"
      >
        <q-tooltip>{{ t('agentRoster.actions.delete') }}</q-tooltip>
      </q-btn>
    </div>
    <p v-if="note" class="ar-hint">{{ note }}</p>

    <section class="ar-sec">
      <h3 class="ar-sec__t">{{ t('agentRoster.insp.reporting') }}</h3>
      <dl class="ar-facts">
        <div>
          <dt>{{ t('agentRoster.insp.belongs') }}</dt>
          <dd>
            <template v-if="path.length">
              <template v-for="(node, i) in path" :key="node.id">
                <span v-if="i" class="ar-sep" aria-hidden="true">›</span>
                <button v-if="node.level !== 'position'" type="button" class="ar-link" @click="$emit('scope', node.id)">
                  {{ node.name }}
                </button>
                <span v-else>{{ node.name }}</span>
              </template>
            </template>
            <span v-else>{{ role === 'butler' ? t('agentRoster.butlers') : t('agentRoster.unassigned') }}</span>
          </dd>
        </div>
        <div v-if="boss">
          <dt>{{ t('agentRoster.insp.reportsTo') }}</dt>
          <dd>
            <button type="button" class="ar-link" @click="$emit('open', boss)">{{ boss.display_name }}</button>
          </dd>
        </div>
        <div v-if="reports !== null">
          <dt>{{ t('agentRoster.insp.reports') }}</dt>
          <dd>
            <button type="button" class="ar-link ar-num" @click="$emit('scope', reportsScope)">
              {{ t('agentRoster.insp.reportsCount', { n: reports }) }}
            </button>
          </dd>
        </div>
      </dl>
    </section>

    <section class="ar-sec">
      <h3 class="ar-sec__t">{{ t('agentRoster.insp.runtime') }}</h3>
      <dl class="ar-facts">
        <div>
          <dt>{{ t('agentRoster.insp.model') }}</dt>
          <dd class="ar-mono">{{ agent.provider || '—' }} / {{ agent.model || '—' }}</dd>
        </div>
        <div>
          <dt>{{ t('agentRoster.insp.promptMode') }}</dt>
          <dd>{{ promptModeLabel(agent.system_prompt_mode) }}</dd>
        </div>
        <div>
          <dt>{{ t('agentRoster.insp.context') }}</dt>
          <dd class="ar-num">{{ formatContext(agent.context_window) }}</dd>
        </div>
        <div>
          <dt>{{ t('agentRoster.insp.lastRun') }}</dt>
          <dd :class="{ 'ar-bad': agent.last_run_status === 'failed' }">{{ runLabel }}</dd>
        </div>
        <div v-if="agent.pending_evolution_count">
          <dt>{{ t('agentRoster.insp.evolution') }}</dt>
          <dd>
            <router-link class="ar-link" :to="settingsLink('evolution')">{{
              t('agentRoster.insp.evolutionPending', { n: agent.pending_evolution_count })
            }}</router-link>
          </dd>
        </div>
        <div>
          <dt>{{ t('agentRoster.insp.a2a') }}</dt>
          <dd>
            {{
              agent.agent_kind === 'a2a_proxy'
                ? t('agentRoster.insp.a2aProxy')
                : agent.a2a_endpoint_enabled
                  ? t('agentRoster.insp.a2aEndpoint')
                  : t('agentRoster.insp.a2aOff')
            }}
          </dd>
        </div>
      </dl>
    </section>

    <section class="ar-sec">
      <h3 class="ar-sec__t">{{ t('agentRoster.insp.assembly') }}</h3>
      <div class="ar-jump">
        <router-link v-for="tab in ASSEMBLY_TABS" :key="tab.key" class="ar-jump__a" :to="settingsLink(tab.key)">
          <q-icon :name="tab.icon" size="16px" aria-hidden="true" />{{ t(`agentRoster.tabs.${tab.key}`) }}
        </router-link>
      </div>
    </section>

    <section class="ar-sec">
      <h3 class="ar-sec__t">{{ t('agentRoster.insp.profile') }}</h3>
      <dl class="ar-facts">
        <div>
          <dt>{{ t('agentRoster.insp.source') }}</dt>
          <dd>{{ sourceLabel }}</dd>
        </div>
        <div v-if="agent.created_by">
          <dt>{{ t('agentRoster.insp.creator') }}</dt>
          <dd class="ar-mono">{{ agent.created_by }}</dd>
        </div>
        <div>
          <dt>{{ t('agentRoster.insp.created') }}</dt>
          <dd class="ar-num">{{ formatDateTime(agent.created_at) || '—' }}</dd>
        </div>
        <div>
          <dt>{{ t('agentRoster.insp.updated') }}</dt>
          <dd class="ar-num">{{ formatDateTime(agent.updated_at) || '—' }}</dd>
        </div>
      </dl>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { Agent } from '../../features/agents/types';
import type { RosterRole, RosterScope } from '../../features/agents/roster';
import type { PlatformResourceTreeNode } from '../../features/platform/types';
import { relativeTime } from '../../features/monitor/eventView';
import { formatContext, promptModeLabel } from './agentUi';
import AgentAvatarQ from '../avatar/AgentAvatarQ.vue';

const props = defineProps<{
  agent: Agent;
  role: RosterRole;
  path: PlatformResourceTreeNode[];
  boss?: Agent;
  /** 领导岗的下属人数；非领导岗为 null */
  reports: number | null;
  reportsScope: RosterScope;
  deletable: boolean;
}>();

defineEmits<{
  close: [];
  open: [agent: Agent];
  scope: [scope: RosterScope];
  'copy-key': [key: string];
  'toggle-favorite': [agent: Agent];
  duplicate: [agent: Agent];
  'set-status': [agent: Agent, status: 'active' | 'inactive'];
  delete: [agent: Agent];
}>();

const { t, te } = useI18n();

/** 设置页的页签直达（键与 AgentSettingsPage 的 tab 名一致） */
const ASSEMBLY_TABS = [
  { key: 'files', icon: 'description' },
  { key: 'skills', icon: 'build' },
  { key: 'memory', icon: 'psychology' },
  { key: 'evolution', icon: 'auto_awesome' },
  { key: 'hooks', icon: 'webhook' },
  { key: 'permissions', icon: 'savings' },
] as const;

/** 本地时间 YYYY-MM-DD HH:mm（toLocaleString 在不同系统上会出 09/11/2026 这种有歧义的写法） */
function formatDateTime(iso?: string) {
  const d = iso ? new Date(iso) : null;
  if (!d || Number.isNaN(d.getTime())) return '';
  const p = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
}

function settingsLink(tab?: string) {
  return tab ? `/agents/${props.agent.id}/settings?tab=${tab}` : `/agents/${props.agent.id}/settings`;
}

const note = computed(() => {
  if (props.deletable) return '';
  if (props.agent.kind === 'ecosystem_preset') return t('agentRoster.insp.presetNote');
  return t('agentRoster.insp.builtinNote');
});

const sourceLabel = computed(() => {
  const s = props.agent.source ?? '';
  return te(`agentRoster.source.${s}`) ? t(`agentRoster.source.${s}`) : s || '—';
});

const runLabel = computed(() => {
  const s = props.agent.last_run_status ?? '';
  const when = relativeTime(t, props.agent.last_run_at);
  if (!when && (!s || s === 'idle')) return t('agentRoster.never');
  const label = te(`agentRoster.run.${s}`) ? t(`agentRoster.run.${s}`) : s;
  return when ? `${label} · ${when}` : label;
});
</script>
