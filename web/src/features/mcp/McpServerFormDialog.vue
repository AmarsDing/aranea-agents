// Container: approved — feature-local panel/dialog; data from Page composable via props.
<template>
  <AppDialog
    :model-value="modelValue"
    :title="row ? t('domains.mcpForm.editTitle') : t('domains.mcpForm.createTitle')"
    :subtitle="t('domains.mcpForm.subtitle')"
    size="xl"
    icon="hub"
    persistent
    :loading="saving"
    @update:model-value="$emit('update:modelValue', $event)"
  >
    <q-form class="app-form-field-grid app-form-field-grid--2col" @submit.prevent="save">
          <q-input
            v-model="form.name"
            dense
            outlined
            :label="t('domains.mcpForm.key')"
            placeholder="my-mcp-server"
            :rules="[slugRule]"
          />
          <q-input
            v-model="form.display_name"
            dense
            outlined
            :label="t('domains.mcpForm.displayName')"
            placeholder="sqlserver"
          />
          <q-input
            v-model="form.description"
            class="app-grid-span-full"
            dense
            outlined
            autogrow
            type="textarea"
            :label="t('domains.mcpForm.description')"
          />

          <div class="app-grid-span-full">
            <div class="section-label q-mb-sm">{{ t('domains.mcpForm.transport') }}</div>
            <q-btn-toggle
              v-model="form.transport"
              spread
              no-caps
              unelevated
              toggle-color="primary"
              class="app-btn-toggle"
              :options="transportOptions"
            />
          </div>

          <q-input
            v-if="usesUrl"
            v-model="form.url"
            class="app-grid-span-full app-field-long"
            dense
            outlined
            label="URL *"
            placeholder="https://example.com/mcp"
            :rules="[urlRule]"
          />

          <template v-else>
            <q-input
              v-model="form.command"
              dense
              outlined
              :label="t('domains.mcpForm.command')"
              placeholder="node"
              :rules="[commandRule]"
            />
            <q-input
              v-model="form.argsText"
              dense
              outlined
              autogrow
              type="textarea"
              :label="t('domains.mcpForm.args')"
              :hint="t('domains.mcpForm.argsHint')"
            />
          </template>

          <q-input
            v-model="form.tool_prefix"
            dense
            outlined
            :label="t('domains.mcpForm.toolPrefix')"
            hint="Tools: mcp_{prefix}__{tool}"
          >
            <template #prepend>mcp_</template>
          </q-input>
          <q-input
            v-model.number="form.timeout_sec"
            dense
            outlined
            type="number"
            min="1"
            suffix="s"
            :label="t('domains.mcpForm.timeout')"
          />
          <q-input
            v-model.number="form.session_reconnect_max"
            dense
            outlined
            type="number"
            min="0"
            max="10"
            :label="t('domains.mcpForm.reconnect')"
            :hint="t('domains.mcpForm.reconnectHint')"
          />
          <q-toggle v-model="form.enabled" color="primary" :label="t('domains.mcpForm.enabled')" />
          <q-select
            v-model="form.probe_mode"
            dense
            outlined
            emit-value
            map-options
            :label="t('domains.mcpForm.probeMode')"
            :options="probeModeOptions"
            :hint="t('domains.mcpForm.probeHint')"
          />
          <q-toggle
            v-model="form.require_user_credentials"
            class="app-grid-span-full"
            color="primary"
            :label="t('domains.mcpForm.requireUserCredentials')"
          />
          <q-toggle
            v-model="form.allow_adhoc_http"
            class="app-grid-span-full"
            color="warning"
            :label="t('domains.mcpForm.allowAdhoc')"
          />

          <div v-if="usesUrl" class="app-grid-span-full">
            <div class="section-label q-mb-sm">{{ t('domains.mcpForm.apiAuth') }}</div>
            <div class="app-form-field-grid q-mb-md">
              <q-select
                v-model="form.auth_type"
                dense
                outlined
                emit-value
                map-options
                :label="t('domains.mcpForm.authType')"
                :options="authTypeOptions"
              />
              <q-input
                v-if="form.auth_type"
                v-model="form.auth_header_name"
                dense
                outlined
                :label="t('domains.mcpForm.headerName')"
                placeholder="Authorization"
                :hint="t('domains.mcpForm.headerHint')"
              />
              <q-input
                v-if="form.auth_type && !isOAuthAuth"
                v-model="form.auth_api_key"
                dense
                outlined
                type="password"
                :label="t('domains.mcpForm.apiKey')"
                placeholder="sk-..."
              />
              <template v-if="isOAuthAuth">
                <q-input
                  v-model="form.auth_token_url"
                  class="app-grid-span-full app-field-long"
                  dense
                  outlined
                  label="Token URL"
                  placeholder="https://provider/oauth/token"
                />
                <q-input v-model="form.auth_client_id" dense outlined label="Client ID" />
                <q-input v-model="form.auth_client_secret" dense outlined type="password" label="Client Secret" />
                <q-input v-model="form.auth_scope" dense outlined label="Scope" placeholder="openid profile" />
                <q-input
                  v-model="form.auth_access_token"
                  dense
                  outlined
                  type="password"
                  :label="t('domains.mcpForm.accessToken')"
                />
                <q-input
                  v-if="form.auth_type === 'oauth2_refresh'"
                  v-model="form.auth_refresh_token"
                  class="app-grid-span-full"
                  dense
                  outlined
                  type="password"
                  label="Refresh Token"
                />
              </template>
            </div>
            <div class="row items-center justify-between q-mb-xs">
              <div class="section-label">{{ t('domains.mcpForm.headers') }}</div>
              <q-btn
                flat
                dense
                rounded
                no-caps
                color="primary"
                icon="add"
                :label="t('domains.mcpForm.addHeader')"
                @click="addPair('headers')"
              />
            </div>
            <div
              v-for="(item, index) in form.headers"
              :key="`header-${index}`"
              class="app-form-field-grid app-form-field-grid--wide items-end q-mb-sm"
            >
              <q-input v-model="item.key" dense outlined :placeholder="t('domains.mcpForm.headerName')" />
              <q-input
                v-model="item.value"
                dense
                outlined
                :type="isSensitiveKey(item.key) ? 'password' : 'text'"
                :placeholder="t('domains.mcpForm.value')"
              />
              <div class="app-actions-bar app-actions-bar--start">
                <q-btn
                  flat
                  dense
                  round
                  icon="delete"
                  color="negative"
                  :aria-label="t('domains.mcpForm.removeHeader')"
                  @click="removePair('headers', index)"
                />
              </div>
            </div>
          </div>

          <div class="app-grid-span-full">
            <div class="row items-center justify-between q-mb-xs">
              <div class="section-label">{{ t('domains.mcpForm.env') }}</div>
              <q-btn
                flat
                dense
                rounded
                no-caps
                color="primary"
                icon="add"
                :label="t('domains.mcpForm.addEnv')"
                @click="addPair('env')"
              />
            </div>
            <div
              v-for="(item, index) in form.env"
              :key="`env-${index}`"
              class="app-form-field-grid app-form-field-grid--wide items-end q-mb-sm"
            >
              <q-input v-model="item.key" dense outlined :placeholder="t('domains.mcpForm.envKey')" />
              <q-input
                v-model="item.value"
                dense
                outlined
                :type="isSensitiveKey(item.key) ? 'password' : 'text'"
                :placeholder="t('domains.mcpForm.value')"
              />
              <div class="app-actions-bar app-actions-bar--start">
                <q-btn
                  flat
                  dense
                  round
                  icon="delete"
                  color="negative"
                  :aria-label="t('domains.mcpForm.removeEnv')"
                  @click="removePair('env', index)"
                />
              </div>
            </div>
          </div>

          <div v-if="serverError" class="app-grid-span-full text-negative">{{ serverError }}</div>
    </q-form>

    <template #actions>
      <q-btn
        outline
        rounded
        no-caps
        color="secondary"
        icon="rule"
        :label="t('domains.mcpForm.validate')"
        :loading="validating"
        :disable="!canSave || saving"
        @click="runValidate"
      />
      <q-btn
        outline
        rounded
        no-caps
        color="primary"
        icon="science"
        :label="t('domains.mcpForm.test')"
        :loading="testing"
        :disable="!canSave || saving"
        @click="saveAndTest"
      />
      <q-space />
      <q-btn flat rounded no-caps :label="t('domains.mcpForm.cancel')" @click="$emit('update:modelValue', false)" />
      <q-btn
        color="primary"
        rounded
        unelevated
        no-caps
        data-test="mcp-server-save"
        :label="row ? t('domains.mcpForm.save') : t('domains.mcpForm.create')"
        :loading="saving"
        :disable="!canSave"
        @click="save"
      />
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import AppDialog from '../../components/common/AppDialog.vue';
import type { McpServerRow } from './types';
import { useMcpServerForm } from './useMcpServerForm';

const { t } = useI18n();

const props = defineProps<{ modelValue: boolean; row: McpServerRow | null }>();
const emit = defineEmits<{ 'update:modelValue': [value: boolean]; saved: [row: McpServerRow]; tested: [] }>();

const {
  form,
  saving,
  testing,
  validating,
  serverError,
  usesUrl,
  canSave,
  isOAuthAuth,
  transportOptions,
  authTypeOptions,
  probeModeOptions,
  save,
  runValidate,
  saveAndTest,
  addPair,
  removePair,
  slugRule,
  urlRule,
  commandRule,
  isSensitiveKey,
} = useMcpServerForm(props, emit);
</script>
