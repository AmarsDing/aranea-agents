<!--
  Channel 路由目标：Agent / Team 下拉，与 Cron 目标选择一致。
-->
<template>
  <div class="channel-routing-fields">
    <div class="app-grid-span-full">
      <div class="section-label q-mb-sm">{{ t('channelEditor.routingTargetLabel') }}</div>
      <q-btn-toggle
        v-model="targetType"
        spread
        no-caps
        unelevated
        toggle-color="primary"
        class="channel-routing-toggle"
        :options="channelRoutingTargetToggleOptions"
      />
    </div>

    <channel-config-row
      :label="t('channelEditor.routingAgentLabel')"
      field-key="default_agent_id"
      :status="targetType === 'agent' && !agentId ? t('channelEditor.status.required') : ''"
    >
      <q-select
        v-if="targetType === 'agent'"
        v-model="agentId"
        class="app-field-md"
        dense
        outlined
        emit-value
        map-options
        :options="agentOptions"
        :loading="loading"
        :disable="loading || !agentOptions.length"
        :placeholder="t('channelEditor.routingAgentPlaceholder')"
      />
      <span v-else class="text-grey-7">{{ t('channelEditor.routingAgentDisabledHint') }}</span>
      <q-banner
        v-if="targetType === 'agent' && agentId && routingAgentProvider && routingAgentModel"
        dense
        rounded
        class="q-mt-sm app-grid-span-full"
        :class="routingAgentModelBannerClass"
      >
        <template v-if="routingAgentModelChecking">
          {{ t('channelEditor.routingAgentModelChecking') }}
        </template>
        <template v-else-if="routingAgentModelOk === true">
          {{ t('channelEditor.routingAgentModelOk', { provider: routingAgentProvider, model: routingAgentModel }) }}
        </template>
        <template v-else-if="routingAgentModelOk === false">
          {{
            t('channelEditor.routingAgentModelFail', {
              provider: routingAgentProvider,
              model: routingAgentModel,
              detail: routingAgentModelMessage || t('channelEditor.routingAgentModelFailDefault'),
            })
          }}
        </template>
      </q-banner>
    </channel-config-row>

    <channel-config-row
      :label="t('channelEditor.dmScopeLabel')"
      field-key="dm_scope"
      :help="{ description: t('channelEditor.dmScopeHint') }"
    >
      <q-select
        v-model="dmScope"
        class="app-field-md"
        dense
        outlined
        emit-value
        map-options
        :options="dmScopeOptions"
      />
    </channel-config-row>

    <channel-config-row
      :label="t('channelEditor.routingTeamLabel')"
      field-key="default_team_id"
      :status="targetType === 'team' && !teamId ? t('channelEditor.status.required') : ''"
    >
      <q-select
        v-if="targetType === 'team'"
        v-model="teamId"
        class="app-field-md"
        dense
        outlined
        emit-value
        map-options
        :options="teamOptions"
        :loading="loading"
        :disable="loading || !teamOptions.length"
        :placeholder="t('channelEditor.routingTeamPlaceholder')"
      />
      <span v-else class="text-grey-7">{{ t('channelEditor.routingTeamDisabledHint') }}</span>
    </channel-config-row>

    <div class="app-grid-span-full q-mt-md">
      <channel-routing-rules-editor v-model="routingRules" :agents="agents" :teams="teams" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import ChannelConfigRow from './ChannelConfigRow.vue';
import ChannelRoutingRulesEditor, { type ChannelRoutingRulePayload } from './ChannelRoutingRulesEditor.vue';
import type { Agent } from '../../features/agents/types';
import {
  channelAgentSelectOptions,
  channelRoutingTargetToggleOptions,
  channelTeamSelectOptions,
  type ChannelRoutingTargetType,
} from '../../domain/channel';
import type { Team } from '../../features/teams/types';

const { t } = useI18n();

const targetType = defineModel<ChannelRoutingTargetType>('targetType', { required: true });
const agentId = defineModel<string>('agentId', { required: true });
const teamId = defineModel<string>('teamId', { required: true });
const dmScope = defineModel<string>('dmScope', { required: true });
const routingRules = defineModel<ChannelRoutingRulePayload[]>('routingRules', { required: true });

const dmScopeOptions = [
  { label: t('channelEditor.dmScope.perChannelPeer'), value: 'per-channel-peer' },
  { label: t('channelEditor.dmScope.main'), value: 'main' },
];

const props = withDefaults(
  defineProps<{
    agents: Agent[];
    teams: Team[];
    loading?: boolean;
    routingAgentProvider?: string;
    routingAgentModel?: string;
    routingAgentModelChecking?: boolean;
    routingAgentModelOk?: boolean | null;
    routingAgentModelMessage?: string;
  }>(),
  {
    loading: false,
    routingAgentProvider: '',
    routingAgentModel: '',
    routingAgentModelChecking: false,
    routingAgentModelOk: null,
    routingAgentModelMessage: '',
  },
);

const agentOptions = computed(() => channelAgentSelectOptions(props.agents));
const teamOptions = computed(() => channelTeamSelectOptions(props.teams));

const routingAgentModelBannerClass = computed(() => {
  if (props.routingAgentModelChecking) return 'bg-info text-white';
  if (props.routingAgentModelOk === true) return 'bg-positive text-white';
  if (props.routingAgentModelOk === false) return 'bg-negative text-white';
  return 'bg-grey-3';
});
</script>
