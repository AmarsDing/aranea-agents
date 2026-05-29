<template>
  <q-card flat bordered class="q-pa-md q-gutter-md" style="max-width: 720px">
    <div class="text-subtitle2">注册远程 A2A Agent</div>
    <q-input v-model="form.workspace" dense outlined label="工作区" hint="留空则使用远程 Card 或 default" />
    <q-input v-model.trim="form.remote_url" dense outlined label="远程 URL *" />
    <q-input v-model.trim="form.display_name" dense outlined label="显示名称" />
    <q-select
      v-model="form.auth_type"
      dense
      outlined
      emit-value
      map-options
      label="鉴权类型"
      :options="authTypeOptions"
    />
    <q-input
      v-if="form.auth_type === 'api_key' || form.auth_type === 'bearer'"
      v-model="authSecret"
      dense
      outlined
      :type="showSecret ? 'text' : 'password'"
      :label="form.auth_type === 'bearer' ? 'Bearer Token' : 'API Key'"
    />
    <template v-if="form.auth_type === 'mtls'">
      <q-input v-model="mtls.cert_file" dense outlined label="客户端证书路径 (cert_file)" />
      <q-input v-model="mtls.key_file" dense outlined label="私钥路径 (key_file)" />
      <q-input v-model="mtls.ca_file" dense outlined label="CA 路径 (ca_file，可选)" />
    </template>
    <div class="row q-gutter-sm">
      <q-btn outline color="primary" label="预览 Discover" :loading="discovering" @click="onDiscover" />
      <q-btn color="primary" unelevated label="注册" :loading="loading" @click="onRegister" />
    </div>
    <q-card v-if="preview" flat bordered class="q-pa-sm">
      <div class="text-caption">预览：{{ preview.display_name }} ({{ preview.agent_id }})</div>
      <div class="text-caption">能力：{{ preview.capabilities.map((c) => c.name).join(", ") || "—" }}</div>
    </q-card>
  </q-card>
</template>

<script setup lang="ts">
import { reactive, ref } from "vue";
import type { A2AAgentCard, DiscoverRemoteInput, RegisterRemoteAgentInput } from "../../features/a2a/types";

defineProps<{
  loading: boolean;
  discovering: boolean;
  preview: A2AAgentCard | null;
}>();

const emit = defineEmits<{
  register: [payload: RegisterRemoteAgentInput];
  discover: [payload: DiscoverRemoteInput];
}>();

const showSecret = ref(false);
const authSecret = ref("");
const form = reactive<RegisterRemoteAgentInput>({
  workspace: "",
  remote_url: "",
  display_name: "",
  auth_type: "none"
});
const mtls = reactive({ cert_file: "", key_file: "", ca_file: "" });

const authTypeOptions = [
  { label: "无", value: "none" },
  { label: "API Key", value: "api_key" },
  { label: "Bearer", value: "bearer" },
  { label: "mTLS", value: "mtls" }
];

function buildAuthJSON() {
  if (form.auth_type === "mtls") {
    return JSON.stringify({
      cert_file: mtls.cert_file.trim(),
      key_file: mtls.key_file.trim(),
      ca_file: mtls.ca_file.trim()
    });
  }
  if (form.auth_type === "api_key") {
    return JSON.stringify({ api_key: authSecret.value.trim() });
  }
  if (form.auth_type === "bearer") {
    return JSON.stringify({ token: authSecret.value.trim() });
  }
  return "";
}

function payload(): RegisterRemoteAgentInput {
  return {
    workspace: form.workspace?.trim(),
    remote_url: form.remote_url.trim(),
    display_name: form.display_name?.trim(),
    auth_type: form.auth_type,
    auth_config_json: buildAuthJSON(),
    enabled: true
  };
}

function onDiscover() {
  emit("discover", {
    remote_url: form.remote_url.trim(),
    auth_type: form.auth_type,
    auth_config_json: buildAuthJSON()
  });
}

function onRegister() {
  emit("register", payload());
}

function resetForm() {
  form.workspace = "";
  form.remote_url = "";
  form.display_name = "";
  form.auth_type = "none";
  authSecret.value = "";
  mtls.cert_file = "";
  mtls.key_file = "";
  mtls.ca_file = "";
}

defineExpose({ resetForm });
</script>
