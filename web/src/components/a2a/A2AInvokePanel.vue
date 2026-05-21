<template>
  <q-card flat bordered class="q-pa-md q-gutter-md" style="max-width: 640px">
    <q-select
      v-if="agentOptions.length"
      v-model="calleeAgentId"
      dense
      outlined
      emit-value
      map-options
      clearable
      label="Callee Agent（Discover）"
      :options="agentOptions"
      hint="可从发现列表选择，或手动填写下方 ID"
    />
    <q-input v-model="calleeAgentId" dense outlined label="Callee Agent ID *" />
    <q-select
      v-if="capabilityOptions.length"
      v-model="capability"
      dense
      outlined
      emit-value
      map-options
      clearable
      label="Capability"
      :options="capabilityOptions"
      hint="选择已注册能力或手动输入"
    />
    <q-input v-model="capability" dense outlined label="Capability *" />
    <q-input
      v-model="workspace"
      dense
      outlined
      label="Workspace（跨工作区 Invoke）"
      hint="Admin 路径：须与 X-Workspace-ID 及被调 Agent Card 一致"
    />
    <q-input v-model="payloadJson" dense outlined type="textarea" rows="6" label="Payload JSON" hint='例如 {"message":"你好"}' />
    <q-input v-model.number="timeoutSeconds" dense outlined type="number" label="Timeout (秒)" />
    <q-btn color="primary" unelevated icon="send" label="Invoke" :loading="loading" @click="$emit('invoke')" />
    <q-card v-if="result" flat bordered class="q-pa-sm">
      <div class="text-caption">invoke_id: {{ result.invoke_id }}</div>
      <div class="text-caption">status: {{ result.status }} · {{ result.duration_ms }}ms</div>
      <pre class="a2a-result">{{ result.result_json || result.error_message }}</pre>
    </q-card>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { A2AAgentCard, A2AInvokeResult } from "../../features/a2a/types";

const props = defineProps<{
  loading: boolean;
  result: A2AInvokeResult | null;
  discoveredAgents?: A2AAgentCard[];
}>();

const calleeAgentId = defineModel<string>("calleeAgentId", { default: "" });
const capability = defineModel<string>("capability", { default: "" });
const payloadJson = defineModel<string>("payloadJson", { default: "{}" });
const timeoutSeconds = defineModel<number>("timeoutSeconds", { default: 30 });
const workspace = defineModel<string>("workspace", { default: "" });

defineEmits<{ invoke: [] }>();

const agentOptions = computed(() =>
  (props.discoveredAgents ?? [])
    .filter((a) => a.enabled)
    .map((a) => ({
      label: `${a.display_name || a.agent_id} (${a.agent_id})`,
      value: a.agent_id
    }))
);

const capabilityOptions = computed(() => {
  const agent = (props.discoveredAgents ?? []).find((a) => a.agent_id === calleeAgentId.value);
  if (!agent) return [];
  return agent.capabilities.map((c) => ({ label: c.name, value: c.name }));
});
</script>

<style scoped>
.a2a-result {
  margin: 0.5rem 0 0;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  font-size: 0.85rem;
}
</style>
