<template>
  <section class="settings-section">
    <div class="section-heading">
      <div>
        <div class="text-subtitle1 text-weight-bold">A2A 远程代理</div>
        <div class="text-caption text-grey-7">此 Agent 通过 A2A 协议连接外部服务，无需本地 LLM。</div>
      </div>
    </div>
    <div class="app-form-field-grid">
      <q-input v-model.trim="proxyForm.remote_url" class="app-field-long" dense outlined label="远程 URL *" />
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
          <q-btn flat dense round :icon="showSecret ? 'visibility_off' : 'visibility'" @click="showSecret = !showSecret" />
        </template>
      </q-input>
      <template v-if="proxyForm.auth_type === 'mtls'">
        <q-input v-model="mtls.cert_file" dense outlined label="客户端证书路径 (cert_file)" />
        <q-input v-model="mtls.key_file" dense outlined label="私钥路径 (key_file)" />
        <q-input v-model="mtls.ca_file" dense outlined label="CA 路径 (ca_file，可选)" />
      </template>
    </div>
    <div class="app-actions-bar app-actions-bar--start q-mt-md">
      <q-btn color="primary" rounded unelevated no-caps label="保存连接" :loading="saving" @click="saveProxy" />
    </div>
  </section>
</template>

<script setup lang="ts">
import { reactive, ref, watch } from "vue";
import { storeToRefs } from "pinia";
import { useQuasar } from "quasar";
import type { A2AProxyConfig } from "../../features/agents/types";
import { useAgentDetailStore } from "../../stores/agents";

const props = defineProps<{
  agentId: string;
  a2aProxy?: A2AProxyConfig;
}>();

const emit = defineEmits<{
  saved: [];
}>();

const $q = useQuasar();
const detailStore = useAgentDetailStore();
const { saving } = storeToRefs(detailStore);
const showSecret = ref(false);
const authSecret = ref("");
const mtls = reactive({ cert_file: "", key_file: "", ca_file: "" });
const authTypeOptions = [
  { label: "无", value: "none" },
  { label: "API Key", value: "api_key" },
  { label: "Bearer Token", value: "bearer" },
  { label: "mTLS", value: "mtls" }
];
const proxyForm = reactive<A2AProxyConfig>({
  remote_url: "",
  enable_streaming: true,
  timeout_seconds: 30,
  auth_type: "none"
});

function parseAuthConfig(raw?: string, authType?: string) {
  authSecret.value = "";
  mtls.cert_file = "";
  mtls.key_file = "";
  mtls.ca_file = "";
  if (!raw) return;
  try {
    const parsed = JSON.parse(raw) as {
      api_key?: string;
      token?: string;
      cert_file?: string;
      key_file?: string;
      ca_file?: string;
    };
    if (authType === "mtls") {
      mtls.cert_file = parsed.cert_file ?? "";
      mtls.key_file = parsed.key_file ?? "";
      mtls.ca_file = parsed.ca_file ?? "";
      return;
    }
    authSecret.value = parsed.api_key ?? parsed.token ?? "";
  } catch {
    authSecret.value = "";
  }
}

function buildAuthConfigJson(): string | undefined {
  const authType = proxyForm.auth_type?.trim();
  if (!authType || authType === "none") return undefined;
  if (authType === "mtls") {
    if (!mtls.cert_file.trim() || !mtls.key_file.trim()) return undefined;
    return JSON.stringify({
      cert_file: mtls.cert_file.trim(),
      key_file: mtls.key_file.trim(),
      ca_file: mtls.ca_file.trim()
    });
  }
  const secret = authSecret.value.trim();
  if (!secret) return undefined;
  if (authType === "bearer") {
    return JSON.stringify({ token: secret });
  }
  return JSON.stringify({ api_key: secret });
}

watch(
  () => props.a2aProxy,
  (cfg) => {
    if (!cfg) return;
    proxyForm.remote_url = cfg.remote_url ?? "";
    proxyForm.enable_streaming = cfg.enable_streaming ?? true;
    proxyForm.timeout_seconds = cfg.timeout_seconds ?? 30;
    proxyForm.auth_type = cfg.auth_type ?? "none";
    parseAuthConfig(cfg.auth_config_json, proxyForm.auth_type);
  },
  { immediate: true }
);

async function saveProxy() {
  if (!proxyForm.remote_url.trim()) {
    $q.notify({ type: "negative", message: "远程 URL 不能为空" });
    return;
  }
  const authType = proxyForm.auth_type?.trim() || "none";
  if (authType === "api_key" || authType === "bearer") {
    if (!authSecret.value.trim()) {
      $q.notify({ type: "negative", message: "请填写鉴权密钥" });
      return;
    }
  }
  if (authType === "mtls" && (!mtls.cert_file.trim() || !mtls.key_file.trim())) {
    $q.notify({ type: "negative", message: "mTLS 需填写 cert_file 与 key_file" });
    return;
  }
  try {
    const payload: A2AProxyConfig = {
      ...proxyForm,
      auth_type: authType === "none" ? undefined : authType,
      auth_config_json: buildAuthConfigJson()
    };
    await detailStore.patch(props.agentId, { a2a_proxy_config: payload });
    $q.notify({ type: "positive", message: "A2A 代理配置已保存" });
    emit("saved");
  } catch (error) {
    $q.notify({ type: "negative", message: error instanceof Error ? error.message : "保存失败" });
  }
}
</script>
