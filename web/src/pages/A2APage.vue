<template>
  <q-page class="app-page-cream a2a-page q-pa-sm q-pa-md-md">
    <section class="a2a-hero">
      <div>
        <div class="a2a-kicker">Agent-to-Agent</div>
        <h1 class="a2a-title">A2A 管理</h1>
        <p class="a2a-subtitle">发现 AgentCard、查看调用审计、测试 Invoke（经 ChatService 派发目标 Agent）。</p>
      </div>
      <q-btn outline rounded color="primary" icon="refresh" label="刷新" :loading="loading" @click="reload" />
    </section>

    <q-tabs v-model="tab" dense align="left" class="text-primary q-mb-md">
      <q-tab name="discover" label="发现" />
      <q-tab name="audit" label="审计" />
      <q-tab name="invoke" label="Invoke" />
    </q-tabs>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="reload" />
      </template>
    </q-banner>

    <q-tab-panels v-model="tab" animated>
      <q-tab-panel name="discover" class="q-pa-none">
        <A2ADiscoverPanel
          v-model:workspace="discoverWorkspace"
          v-model:capability="discoverCapability"
          :agents="agents"
          :loading="loading"
          :columns="cardColumns"
          @discover="loadDiscover"
        />
      </q-tab-panel>
      <q-tab-panel name="audit" class="q-pa-none">
        <A2AAuditPanel :rows="auditRows" :loading="auditLoading" :columns="auditColumns" :status-color="auditStatusColor" />
      </q-tab-panel>
      <q-tab-panel name="invoke" class="q-pa-none">
        <A2AInvokePanel
          v-model:callee-agent-id="invokeForm.callee_agent_id"
          v-model:capability="invokeForm.capability"
          v-model:payload-json="invokeForm.payload_json"
          v-model:timeout-seconds="invokeForm.timeout_seconds"
          :loading="invokeLoading"
          :result="invokeResult"
          @invoke="submitInvoke"
        />
      </q-tab-panel>
    </q-tab-panels>
  </q-page>
</template>

<script setup lang="ts">
import A2ADiscoverPanel from "../components/a2a/A2ADiscoverPanel.vue";
import A2AAuditPanel from "../components/a2a/A2AAuditPanel.vue";
import A2AInvokePanel from "../components/a2a/A2AInvokePanel.vue";
import { useA2APage } from "../features/a2a/useA2APage";

const {
  agents,
  auditRows,
  loading,
  tab,
  auditLoading,
  invokeLoading,
  error,
  invokeResult,
  discoverWorkspace,
  discoverCapability,
  invokeForm,
  cardColumns,
  auditColumns,
  auditStatusColor,
  loadDiscover,
  submitInvoke,
  reload
} = useA2APage();
</script>

<style scoped>
.a2a-hero {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: 1rem;
  margin-bottom: 1rem;
}
.a2a-kicker {
  font-size: 0.75rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--q-primary);
  font-weight: 600;
}
.a2a-title {
  margin: 0.25rem 0;
  font-size: 1.75rem;
  font-weight: 700;
}
.a2a-subtitle {
  margin: 0;
  color: #666;
  max-width: 36rem;
}
</style>
