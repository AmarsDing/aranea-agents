// Container: approved — feature-local panel/dialog; data from Page composable via props.
<template>
  <q-dialog :model-value="modelValue" persistent @update:model-value="$emit('update:modelValue', $event)">
    <q-card class="app-dialog-card app-dialog-card--sm">
      <q-card-section class="row items-center justify-between">
        <div>
          <div class="text-h6">用户凭据</div>
          <div class="text-caption text-grey-7">{{ serverLabel }} · 用户 {{ userId }}</div>
        </div>
        <q-btn flat dense round icon="close" aria-label="关闭" @click="$emit('update:modelValue', false)" />
      </q-card-section>
      <q-separator />
      <q-card-section class="app-dialog-body">
        <q-inner-loading :showing="loading" />
        <div v-if="!loading && items.length === 0" class="text-caption text-grey-7 q-mb-md">尚未配置凭据。保存后 Agent 会话方可调用此 MCP。</div>
        <q-list v-if="items.length" bordered separator class="rounded-borders q-mb-md">
          <q-item v-for="cred in items" :key="cred.credential_key">
            <q-item-section>
              <q-item-label>{{ cred.credential_key }}</q-item-label>
              <q-item-label caption>
                {{ cred.configured ? cred.masked_preview || "已配置" : "未配置" }}
                <span v-if="cred.status"> · {{ cred.status }}</span>
              </q-item-label>
            </q-item-section>
            <q-item-section side>
              <q-btn flat dense round icon="delete" color="negative" aria-label="删除" @click="remove(cred.credential_key)" />
            </q-item-section>
          </q-item>
        </q-list>
        <q-form class="app-form-field-grid app-form-field-grid--2col" @submit.prevent="save">
          <q-input v-model="form.credential_key" dense outlined label="凭据键" hint="通常为 Authorization 或 API 头名" />
          <q-input v-model="form.secret" dense outlined type="password" label="密钥 / Token" />
          <div class="app-actions-bar app-actions-bar--start">
            <q-btn color="primary" unelevated rounded no-caps label="保存凭据" type="submit" :loading="saving" :disable="!canSave" />
          </div>
        </q-form>
      </q-card-section>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import { useQuasar } from "quasar";
import { deleteMcpUserCredential, listMcpUserCredentials, upsertMcpUserCredential } from "./api";
import type { McpUserCredential } from "./types";

const props = defineProps<{
  modelValue: boolean;
  mcpServerId: string;
  serverLabel: string;
  userId: string;
}>();

const emit = defineEmits<{
  "update:modelValue": [value: boolean];
  saved: [];
}>();

const $q = useQuasar();
const loading = ref(false);
const saving = ref(false);
const items = ref<McpUserCredential[]>([]);
const form = reactive({ credential_key: "Authorization", secret: "" });

const canSave = computed(() => Boolean(props.mcpServerId && props.userId && form.credential_key.trim() && form.secret.trim()));

watch(
  () => [props.modelValue, props.mcpServerId, props.userId] as const,
  ([open]) => {
    if (open) void reload();
  }
);

async function reload() {
  if (!props.mcpServerId || !props.userId) return;
  loading.value = true;
  try {
    items.value = await listMcpUserCredentials(props.mcpServerId, props.userId);
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : "加载凭据失败" });
  } finally {
    loading.value = false;
  }
}

async function save() {
  if (!canSave.value) return;
  saving.value = true;
  try {
    await upsertMcpUserCredential(props.mcpServerId, props.userId, {
      credential_key: form.credential_key.trim(),
      secret: form.secret.trim()
    });
    form.secret = "";
    await reload();
    emit("saved");
    $q.notify({ type: "positive", message: "凭据已保存" });
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : "保存失败" });
  } finally {
    saving.value = false;
  }
}

async function remove(credentialKey: string) {
  try {
    await deleteMcpUserCredential(props.mcpServerId, props.userId, credentialKey);
    await reload();
    $q.notify({ type: "positive", message: "已删除" });
  } catch (err) {
    $q.notify({ type: "negative", message: err instanceof Error ? err.message : "删除失败" });
  }
}
</script>
