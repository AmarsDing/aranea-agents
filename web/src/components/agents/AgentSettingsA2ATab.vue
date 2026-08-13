<template>
  <section class="settings-section">
    <div class="section-heading">
      <div>
        <div class="text-subtitle1 text-weight-bold">A2A 远程代理</div>
        <div class="text-caption text-grey-7">此 Agent 通过 A2A 协议连接外部服务，无需本地 LLM。</div>
      </div>
    </div>
    <div class="app-form-field-grid">
      <q-input
        v-model.trim="proxyForm.remote_url"
        class="app-field-long"
        dense
        outlined
        label="远程 URL *"
        hint="远程 A2A 服务地址，须以 http:// 或 https:// 开头"
      />
      <q-toggle v-model="proxyForm.enable_streaming" color="primary" label="流式响应" />
      <q-input v-model.number="proxyForm.timeout_seconds" dense outlined type="number" min="5" label="超时（秒）" />
      <q-select
        v-model="proxyForm.auth_type"
        dense
        outlined
        emit-value
        map-options
        label="鉴权类型"
        :options="authTypeOptions"
      />
      <q-input
        v-if="proxyForm.auth_type === 'api_key' || proxyForm.auth_type === 'bearer'"
        v-model="authSecret"
        class="app-field-long"
        dense
        outlined
        :type="showSecret ? 'text' : 'password'"
        :label="proxyForm.auth_type === 'bearer' ? 'Bearer Token' : 'API Key'"
      >
        <template #append>
          <q-btn
            flat
            dense
            round
            :icon="showSecret ? 'visibility_off' : 'visibility'"
            @click="showSecret = !showSecret"
          />
        </template>
      </q-input>
      <template v-if="proxyForm.auth_type === 'mtls'">
        <q-input v-model="mtls.cert_file" class="app-field-long" dense outlined label="客户端证书路径 (cert_file)" />
        <q-input v-model="mtls.key_file" class="app-field-long" dense outlined label="私钥路径 (key_file)" />
        <q-input v-model="mtls.ca_file" class="app-field-long" dense outlined label="CA 路径 (ca_file，可选)" />
      </template>
    </div>
    <div class="app-actions-bar app-actions-bar--start q-mt-md">
      <q-btn
        flat
        rounded
        no-caps
        color="primary"
        icon="cable"
        :label="$t('agentSettings.a2aTestConnection')"
        :loading="testing"
        @click="testConnection"
      />
      <q-btn
        color="primary"
        rounded
        unelevated
        no-caps
        :label="$t('agentSettings.a2aSaveConnection')"
        :loading="saving"
        @click="saveProxy"
      />
    </div>
  </section>
</template>

<script setup lang="ts">
import type { A2AProxyConfig } from '../../features/agents/types';
import { useAgentA2AProxyTab } from '../../features/agents/useAgentA2AProxyTab';

const props = defineProps<{
  agentId: string;
  a2aProxy?: A2AProxyConfig;
}>();

const emit = defineEmits<{
  saved: [];
}>();

const { saving, testing, showSecret, authSecret, mtls, authTypeOptions, proxyForm, saveProxy, testConnection } =
  useAgentA2AProxyTab(
    () => props.agentId,
    () => props.a2aProxy,
    () => emit('saved'),
  );
</script>
