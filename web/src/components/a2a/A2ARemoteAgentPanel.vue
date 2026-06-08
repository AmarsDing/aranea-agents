<template>
  <div class="app-form-wide a2a-register-form">
    <!-- Header -->
    <div class="a2a-form-header">
      <q-icon name="cloud_upload" size="sm" color="primary" />
      <span class="a2a-form-header__title">注册远程 A2A Agent</span>
    </div>

    <div class="a2a-form-body">
      <!-- Group: 连接 -->
      <div class="a2a-form-group">
        <div class="a2a-form-group__title">连接</div>

        <div class="a2a-form-row">
          <div class="a2a-form-row__label">远程 URL *</div>
          <div class="a2a-form-row__control">
            <q-input v-model.trim="form.remote_url" dense outlined class="app-glass-control" />
          </div>
        </div>

        <div class="a2a-form-row">
          <div class="a2a-form-row__label">显示名称</div>
          <div class="a2a-form-row__control">
            <q-input v-model.trim="form.display_name" dense outlined class="app-glass-control" />
          </div>
        </div>

        <div class="a2a-form-row">
          <div class="a2a-form-row__label">工作区</div>
          <div class="a2a-form-row__control">
            <q-input v-model="form.workspace" dense outlined class="app-glass-control" />
          </div>
          <div class="a2a-form-row__hint">留空则使用远程 Card 或 default</div>
        </div>
      </div>

      <!-- Group: 鉴权 -->
      <div class="a2a-form-group">
        <div class="a2a-form-group__title">鉴权</div>

        <div class="a2a-form-row">
          <div class="a2a-form-row__label">鉴权类型</div>
          <div class="a2a-form-row__control">
            <q-select
              v-model="form.auth_type"
              dense
              outlined
              emit-value
              map-options
              :options="authTypeOptions"
              class="app-glass-control app-field-sm"
            />
          </div>
        </div>

        <!-- Auth: API Key / Bearer -->
        <div v-if="form.auth_type === 'api_key' || form.auth_type === 'bearer'" class="a2a-form-row">
          <div class="a2a-form-row__label">{{ form.auth_type === 'bearer' ? 'Bearer Token' : 'API Key' }}</div>
          <div class="a2a-form-row__control">
            <q-input
              v-model="authSecret"
              dense
              outlined
              :type="showSecret ? 'text' : 'password'"
              class="app-glass-control"
            >
              <template #append>
                <q-btn flat dense round :icon="showSecret ? 'visibility_off' : 'visibility'" @click="showSecret = !showSecret" />
              </template>
            </q-input>
          </div>
        </div>

        <!-- Auth: mTLS -->
        <template v-if="form.auth_type === 'mtls'">
          <div class="a2a-form-row">
            <div class="a2a-form-row__label">客户端证书</div>
            <div class="a2a-form-row__control">
              <q-input v-model="mtls.cert_file" dense outlined class="app-glass-control" />
            </div>
          </div>
          <div class="a2a-form-row">
            <div class="a2a-form-row__label">私钥路径</div>
            <div class="a2a-form-row__control">
              <q-input v-model="mtls.key_file" dense outlined class="app-glass-control" />
            </div>
          </div>
          <div class="a2a-form-row">
            <div class="a2a-form-row__label">CA 路径</div>
            <div class="a2a-form-row__control">
              <q-input v-model="mtls.ca_file" dense outlined class="app-glass-control" />
            </div>
            <div class="a2a-form-row__hint">可选，留空使用系统 CA</div>
          </div>
        </template>
      </div>

      <!-- Actions -->
      <div class="a2a-form-actions">
        <q-btn outline no-caps color="primary" icon="search" label="预览 Discover" :loading="discovering" @click="onDiscover" />
        <q-btn unelevated no-caps color="primary" icon="cloud_upload" label="注册" :loading="loading" @click="onRegister" />
      </div>

      <!-- Preview -->
      <q-card v-if="preview" flat bordered class="a2a-register-preview app-glass-side-panel">
        <div class="a2a-register-preview__meta">
          <span class="a2a-register-preview__name">{{ preview.display_name }}</span>
          <span class="a2a-register-preview__id">{{ preview.agent_id }}</span>
        </div>
        <div class="a2a-register-preview__caps">
          <q-chip v-for="c in preview.capabilities" :key="c.name" dense outline size="sm">{{ c.name }}</q-chip>
          <span v-if="!preview.capabilities.length" class="text-grey-6">无能力</span>
        </div>
      </q-card>
    </div>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue';
import type { A2AAgentCard, DiscoverRemoteInput, RegisterRemoteAgentInput } from '../../features/a2a/types';
import { buildA2AAuthJSON, A2A_AUTH_TYPE_OPTIONS } from '../../features/a2a/authUtils';

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
const authSecret = ref('');
const form = reactive<RegisterRemoteAgentInput>({
  workspace: '',
  remote_url: '',
  display_name: '',
  auth_type: 'none',
});
const mtls = reactive({ cert_file: '', key_file: '', ca_file: '' });

const authTypeOptions = A2A_AUTH_TYPE_OPTIONS;

function payload(): RegisterRemoteAgentInput {
  return {
    workspace: form.workspace?.trim(),
    remote_url: form.remote_url.trim(),
    display_name: form.display_name?.trim(),
    auth_type: form.auth_type,
    auth_config_json: buildA2AAuthJSON(form.auth_type, authSecret.value, mtls),
    enabled: true,
  };
}

function onDiscover() {
  emit('discover', {
    remote_url: form.remote_url.trim(),
    auth_type: form.auth_type,
    auth_config_json: buildA2AAuthJSON(form.auth_type, authSecret.value, mtls),
  });
}

function onRegister() {
  emit('register', payload());
}

function resetForm() {
  form.workspace = '';
  form.remote_url = '';
  form.display_name = '';
  form.auth_type = 'none';
  authSecret.value = '';
  mtls.cert_file = '';
  mtls.key_file = '';
  mtls.ca_file = '';
}

defineExpose({ resetForm });
</script>
