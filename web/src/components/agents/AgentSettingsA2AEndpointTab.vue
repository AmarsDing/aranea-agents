<template>
  <section class="settings-section">
    <div class="section-heading">
      <div>
        <div class="text-subtitle1 text-weight-bold">A2A Endpoint</div>
        <div class="text-caption text-grey-7">将本 Agent 暴露为 A2A 服务，供同工作区 call_agent 或外部客户端调用。</div>
      </div>
    </div>

    <q-inner-loading :showing="loading" />

    <div v-if="card" class="q-gutter-md">
      <q-toggle v-model="card.enabled" color="primary" label="启用 A2A" />
      <div>
        <div class="text-caption text-grey-7 q-mb-sm">Capabilities（JSON 名称列表，每行一个能力名）</div>
        <q-input v-model="capabilityLines" class="app-field-long" outlined type="textarea" rows="4" hint="例如 chat、summarize" />
      </div>
      <div class="app-actions-bar app-actions-bar--start">
        <q-btn color="primary" rounded unelevated no-caps label="保存 AgentCard" :loading="saving" @click="saveEndpoint" />
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { useAgentA2AEndpointTab } from "../../features/agents/useAgentA2AEndpointTab";

const props = defineProps<{ agentId: string }>();

const { loading, saving, card, capabilityLines, saveEndpoint } = useAgentA2AEndpointTab(() => props.agentId);
</script>
