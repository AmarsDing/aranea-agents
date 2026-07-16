// Container: approved — feature-local panel/dialog; data from Page composable via props.
<template>
  <q-dialog :model-value="modelValue" persistent @update:model-value="$emit('update:modelValue', $event)">
    <q-card class="mcp-form-card app-dialog-card app-dialog-card--xl app-glass-dialog">
      <q-card-section class="row items-start justify-between q-gutter-md">
        <div>
          <div class="text-h6">{{ row ? '编辑 MCP 服务器' : '添加 MCP 服务器' }}</div>
          <div class="text-caption text-grey-7">配置连接方式、请求头、环境变量与工具前缀。</div>
        </div>
        <q-btn flat dense round icon="close" aria-label="关闭" @click="$emit('update:modelValue', false)" />
      </q-card-section>
      <q-separator />

      <q-card-section class="mcp-form-scroll">
        <q-form class="app-form-field-grid app-form-field-grid--2col" @submit.prevent="save">
          <q-input
            v-model="form.name"
            dense
            outlined
            label="标识符 (key) *"
            placeholder="my-mcp-server"
            :rules="[slugRule]"
          />
          <q-input v-model="form.display_name" dense outlined label="显示名称" placeholder="sqlserver" />
          <q-input
            v-model="form.description"
            class="app-grid-span-full"
            dense
            outlined
            autogrow
            type="textarea"
            label="描述"
          />

          <div class="app-grid-span-full">
            <div class="section-label q-mb-sm">传输方式 *</div>
            <q-btn-toggle
              v-model="form.transport"
              spread
              no-caps
              unelevated
              toggle-color="primary"
              color="grey-2"
              text-color="grey-9"
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
              label="Command *"
              placeholder="node"
              :rules="[commandRule]"
            />
            <q-input
              v-model="form.argsText"
              dense
              outlined
              autogrow
              type="textarea"
              label="Args"
              hint="每行一个参数，例如 server.js"
            />
          </template>

          <q-input v-model="form.tool_prefix" dense outlined label="工具前缀" hint="Tools: mcp_{prefix}__{tool}">
            <template #prepend>mcp_</template>
          </q-input>
          <q-input v-model.number="form.timeout_sec" dense outlined type="number" min="1" suffix="s" label="超时" />
          <q-input
            v-model.number="form.session_reconnect_max"
            dense
            outlined
            type="number"
            min="0"
            max="10"
            label="SSE 重连次数"
            hint="0=关闭"
          />
          <q-toggle v-model="form.enabled" color="primary" label="启用" />
          <q-select
            v-model="form.probe_mode"
            dense
            outlined
            emit-value
            map-options
            label="探活模式"
            :options="probeModeOptions"
            hint="connectivity=仅网络；auth_aware=带 OAuth/API Key"
          />
          <q-toggle
            v-model="form.require_user_credentials"
            class="app-grid-span-full"
            color="primary"
            label="每个用户须配置自己的凭据，否则无法使用"
          />
          <q-toggle
            v-model="form.allow_adhoc_http"
            class="app-grid-span-full"
            color="warning"
            label="允许 Broker AdHoc HTTP（仍需系统设置 mcp_allow_adhoc_http 开启）"
          />

          <div v-if="usesUrl" class="app-grid-span-full">
            <div class="section-label q-mb-sm">API 认证（可选）</div>
            <div class="app-form-field-grid q-mb-md">
              <q-select
                v-model="form.auth_type"
                dense
                outlined
                emit-value
                map-options
                label="认证方式"
                :options="authTypeOptions"
              />
              <q-input
                v-if="form.auth_type"
                v-model="form.auth_header_name"
                dense
                outlined
                label="Header 名称"
                placeholder="Authorization"
                hint="留空则使用 Authorization"
              />
              <q-input
                v-if="form.auth_type && !isOAuthAuth"
                v-model="form.auth_api_key"
                dense
                outlined
                type="password"
                label="API Key / Token"
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
                  label="Access Token（可选，静态）"
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
              <div class="section-label">请求头</div>
              <q-btn
                flat
                dense
                rounded
                no-caps
                color="primary"
                icon="add"
                label="添加请求头"
                @click="addPair('headers')"
              />
            </div>
            <div
              v-for="(item, index) in form.headers"
              :key="`header-${index}`"
              class="app-form-field-grid app-form-field-grid--wide items-end q-mb-sm"
            >
              <q-input v-model="item.key" dense outlined placeholder="Header 名称" />
              <q-input
                v-model="item.value"
                dense
                outlined
                :type="isSensitiveKey(item.key) ? 'password' : 'text'"
                placeholder="值"
              />
              <div class="app-actions-bar app-actions-bar--start">
                <q-btn
                  flat
                  dense
                  round
                  icon="delete"
                  color="negative"
                  aria-label="删除请求头"
                  @click="removePair('headers', index)"
                />
              </div>
            </div>
          </div>

          <div class="app-grid-span-full">
            <div class="row items-center justify-between q-mb-xs">
              <div class="section-label">环境变量</div>
              <q-btn flat dense rounded no-caps color="primary" icon="add" label="添加变量" @click="addPair('env')" />
            </div>
            <div
              v-for="(item, index) in form.env"
              :key="`env-${index}`"
              class="app-form-field-grid app-form-field-grid--wide items-end q-mb-sm"
            >
              <q-input v-model="item.key" dense outlined placeholder="变量名称" />
              <q-input
                v-model="item.value"
                dense
                outlined
                :type="isSensitiveKey(item.key) ? 'password' : 'text'"
                placeholder="值"
              />
              <div class="app-actions-bar app-actions-bar--start">
                <q-btn
                  flat
                  dense
                  round
                  icon="delete"
                  color="negative"
                  aria-label="删除变量"
                  @click="removePair('env', index)"
                />
              </div>
            </div>
          </div>

          <div v-if="serverError" class="app-grid-span-full text-negative">{{ serverError }}</div>
        </q-form>
      </q-card-section>

      <q-separator />
      <q-card-actions class="app-actions-bar">
        <q-btn
          outline
          rounded
          no-caps
          color="secondary"
          icon="rule"
          label="预检配置"
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
          label="测试连接"
          :loading="testing"
          :disable="!canSave || saving"
          @click="saveAndTest"
        />
        <q-space />
        <q-btn flat rounded no-caps label="取消" @click="$emit('update:modelValue', false)" />
        <q-btn
          color="primary"
          rounded
          unelevated
          no-caps
          :label="row ? '保存' : '创建'"
          :loading="saving"
          :disable="!canSave"
          @click="save"
        />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import type { McpServerRow } from './types';
import { useMcpServerForm } from './useMcpServerForm';

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
