<template>
  <div class="channel-routing-rules column q-gutter-sm">
    <div class="row items-center justify-between">
      <div class="text-subtitle2">{{ t("channelEditor.routingRulesLabel") }}</div>
      <q-btn flat dense no-caps color="primary" icon="add" :label="t('channelEditor.routingRulesAdd')" @click="addRule" />
    </div>
    <q-card v-for="rule in rules" :key="rule.id" flat bordered class="q-pa-sm">
      <div class="row q-col-gutter-sm items-start">
        <div class="col-12 col-md-5">
          <q-input
            v-model="rule.peer_pattern"
            :label="t('channelEditor.routingRulesPeerPattern')"
            dense
            outlined
            :hint="t('channelEditor.routingRulesPeerPatternHint')"
            @update:model-value="emitRules"
          />
        </div>
        <div class="col-12 col-md-3">
          <q-select
            v-model="rule.target_type"
            :label="t('channelEditor.routingRulesTargetType')"
            dense
            outlined
            emit-value
            map-options
            :options="targetTypeOptions"
            @update:model-value="onTargetTypeChange(rule)"
          />
        </div>
        <div class="col-12 col-md-3">
          <q-select
            v-if="rule.target_type === 'agent'"
            v-model="rule.agent_id"
            :label="t('channelEditor.routingAgentLabel')"
            dense
            outlined
            emit-value
            map-options
            :options="agentOptions"
            @update:model-value="emitRules"
          />
          <q-select
            v-else
            v-model="rule.team_id"
            :label="t('channelEditor.routingTeamLabel')"
            dense
            outlined
            emit-value
            map-options
            :options="teamOptions"
            @update:model-value="emitRules"
          />
        </div>
        <div class="col-12 col-md-1 row justify-end">
          <q-btn flat dense round icon="delete" color="negative" @click="removeRule(rule.id)" />
        </div>
      </div>
    </q-card>
    <div v-if="rules.length === 0" class="text-caption text-grey-7">
      {{ t("channelEditor.rulesEmptyHint") }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import type { Agent } from "../../features/agents/types";
import { channelAgentSelectOptions, channelTeamSelectOptions } from "../../features/channels/channelRoutingUtils";
import type { Team } from "../../features/teams/types";

export type ChannelRoutingRuleRow = {
  id: string;
  peer_pattern: string;
  target_type: "agent" | "team";
  agent_id: string;
  team_id: string;
};

export type ChannelRoutingRulePayload = {
  peer_pattern: string;
  agent_id?: string;
  team_id?: string;
};

const props = defineProps<{
  modelValue: ChannelRoutingRulePayload[];
  agents: Agent[];
  teams: Team[];
}>();

const emit = defineEmits<{
  "update:modelValue": [value: ChannelRoutingRulePayload[]];
}>();

const { t } = useI18n();
const rules = ref<ChannelRoutingRuleRow[]>([]);

const agentOptions = computed(() => channelAgentSelectOptions(props.agents));
const teamOptions = computed(() => channelTeamSelectOptions(props.teams));
const targetTypeOptions = [
  { label: "Agent", value: "agent" },
  { label: "Team", value: "team" }
];

function syncFromProps() {
  rules.value = (props.modelValue ?? []).map((raw, idx) => ({
    id: `rule-${idx}-${raw.peer_pattern}`,
    peer_pattern: String(raw.peer_pattern ?? ""),
    target_type: raw.team_id ? "team" : "agent",
    agent_id: String(raw.agent_id ?? ""),
    team_id: String(raw.team_id ?? "")
  }));
}

watch(() => props.modelValue, syncFromProps, { immediate: true, deep: true });

function emitRules() {
  emit(
    "update:modelValue",
    rules.value
      .filter((r) => r.peer_pattern.trim())
      .map((r) => ({
        peer_pattern: r.peer_pattern.trim(),
        ...(r.target_type === "team" ? { team_id: r.team_id.trim() } : { agent_id: r.agent_id.trim() })
      }))
  );
}

function addRule() {
  rules.value.push({
    id: `rule-${Date.now()}`,
    peer_pattern: "",
    target_type: "agent",
    agent_id: "",
    team_id: ""
  });
  emitRules();
}

function removeRule(id: string) {
  rules.value = rules.value.filter((r) => r.id !== id);
  emitRules();
}

function onTargetTypeChange(rule: ChannelRoutingRuleRow) {
  if (rule.target_type === "agent") {
    rule.team_id = "";
  } else {
    rule.agent_id = "";
  }
  emitRules();
}
</script>
