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
        <q-card flat bordered class="q-mb-md">
          <q-card-section class="row q-col-gutter-md">
            <q-input v-model="discoverWorkspace" class="col-12 col-md-5" dense outlined clearable label="Workspace" />
            <q-input v-model="discoverCapability" class="col-12 col-md-5" dense outlined clearable label="Capability" />
            <div class="col-12 col-md-2 flex items-center">
              <q-btn color="primary" unelevated icon="search" label="发现" :loading="loading" @click="loadDiscover" />
            </div>
          </q-card-section>
        </q-card>
        <q-table flat :rows="agents" :columns="cardColumns" row-key="agent_id" :loading="loading" :pagination="{ rowsPerPage: 10 }">
          <template #body-cell-enabled="props">
            <q-td :props="props">
              <q-chip dense :color="props.row.enabled ? 'positive' : 'grey'" text-color="white" size="sm">
                {{ props.row.enabled ? "启用" : "禁用" }}
              </q-chip>
            </q-td>
          </template>
          <template #body-cell-capabilities="props">
            <q-td :props="props">
              <q-chip v-for="c in props.row.capabilities" :key="c.name" dense outline size="sm">{{ c.name }}</q-chip>
            </q-td>
          </template>
        </q-table>
      </q-tab-panel>

      <q-tab-panel name="audit" class="q-pa-none">
        <q-table flat :rows="auditRows" :columns="auditColumns" row-key="id" :loading="auditLoading" :pagination="{ rowsPerPage: 15 }">
          <template #body-cell-status="props">
            <q-td :props="props">
              <q-chip dense :color="auditStatusColor(props.row.status)" text-color="white" size="sm">{{ props.row.status }}</q-chip>
            </q-td>
          </template>
        </q-table>
      </q-tab-panel>

      <q-tab-panel name="invoke" class="q-pa-none">
        <q-card flat bordered class="q-pa-md q-gutter-md" style="max-width: 640px">
          <q-input v-model="invokeForm.callee_agent_id" dense outlined label="Callee Agent ID" />
          <q-input v-model="invokeForm.capability" dense outlined label="Capability" />
          <q-input v-model="invokeForm.payload_json" dense outlined type="textarea" rows="6" label="Payload JSON" />
          <q-input v-model.number="invokeForm.timeout_seconds" dense outlined type="number" label="Timeout (秒)" />
          <q-btn color="primary" unelevated icon="send" label="Invoke" :loading="invokeLoading" @click="submitInvoke" />
          <q-card v-if="invokeResult" flat bordered class="q-pa-sm">
            <div class="text-caption">invoke_id: {{ invokeResult.invoke_id }}</div>
            <div class="text-caption">status: {{ invokeResult.status }} · {{ invokeResult.duration_ms }}ms</div>
            <pre class="a2a-result">{{ invokeResult.result_json || invokeResult.error_message }}</pre>
          </q-card>
        </q-card>
      </q-tab-panel>
    </q-tab-panels>
  </q-page>
</template>

<script setup lang="ts">
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
.a2a-result {
  margin: 0.5rem 0 0;
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 0.85rem;
}
</style>
