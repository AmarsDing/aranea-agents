<template>
  <section class="settings-section">
    <div class="section-heading">
      <div>
        <div class="text-subtitle1 text-weight-bold">A2A 端点</div>
        <div class="text-caption text-grey-7">将本 Agent 暴露为 A2A 服务，供同工作区 call_agent 或外部客户端调用。</div>
      </div>
    </div>

    <q-inner-loading :showing="loading" />

    <div v-if="card" class="q-gutter-md">
      <q-toggle :model-value="card.enabled" color="primary" label="启用 A2A" @update:model-value="setCardEnabled" />
      <div>
        <div class="text-caption text-grey-7 q-mb-sm">Capabilities（JSON 名称列表，每行一个能力名）</div>
        <q-input
          :model-value="capabilityLines"
          class="app-field-long"
          outlined
          type="textarea"
          rows="4"
          hint="例如 chat、summarize"
          @update:model-value="capabilityLines = String($event ?? '')"
        />
      </div>
      <div class="app-actions-bar app-actions-bar--start">
        <q-btn
          color="primary"
          rounded
          unelevated
          no-caps
          label="保存 AgentCard"
          :loading="saving"
          :disable="!card"
          @click="saveEndpoint"
        />
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
// Container: approved — A2A endpoint Tab；内部调用 useAgentA2AEndpointTab。
import { reactive } from 'vue';
import { useAgentA2AEndpointTab } from '../../features/agents/useAgentA2AEndpointTab';

const props = defineProps<{
  agentId: string;
}>();

const { loading, saving, card, capabilityLines, setCardEnabled, saveEndpoint } = reactive(
  useAgentA2AEndpointTab(() => props.agentId),
);
</script>
