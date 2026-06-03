// Container: approved — feature-local panel/dialog; data from Page composable via props.
<template>
  <q-dialog :model-value="modelValue" persistent @update:model-value="$emit('update:modelValue', $event)">
    <q-card class="mcp-form-card app-dialog-card app-dialog-card--xl">
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
          <q-toggle
            v-model="form.require_user_credentials"
            class="app-grid-span-full"
            color="primary"
            label="每个用户须配置自己的凭据，否则无法使用"
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
import { computed, reactive, ref, watch } from 'vue';
import { useQuasar } from 'quasar';
import type { PlatformResourceInput } from '../platform/types';
import type { McpKeyValue, McpServerConfig, McpServerFormValue, McpServerMetadata, McpServerRow } from './types';
import { parseJSON } from './utils';
import { useMcpStore } from '../../stores/mcp';

const props = defineProps<{
  modelValue: boolean;
  row: McpServerRow | null;
}>();

const emit = defineEmits<{
  'update:modelValue': [value: boolean];
  saved: [row: McpServerRow];
  tested: [];
}>();

const $q = useQuasar();
const mcpStore = useMcpStore();
const saving = ref(false);
const testing = ref(false);
const validating = ref(false);
const serverError = ref('');
let form = reactive<McpServerFormValue>(emptyForm());

const transportOptions = [
  { label: 'stdio', value: 'stdio' },
  { label: 'SSE', value: 'sse' },
  { label: 'Streamable HTTP', value: 'streamable_http' },
];

const usesUrl = computed(() => form.transport === 'sse' || form.transport === 'streamable_http');
const canSave = computed(
  () => slugPattern.test(form.name) && (usesUrl.value ? isHttpUrl(form.url) : Boolean(form.command.trim())),
);

watch(
  () => props.modelValue,
  (open) => {
    if (open) resetForm();
  },
);

function emptyForm(): McpServerFormValue {
  return {
    name: '',
    display_name: '',
    description: '',
    transport: 'streamable_http',
    url: '',
    command: '',
    argsText: '',
    headers: [],
    env: [],
    tool_prefix: '',
    timeout_sec: 60,
    session_reconnect_max: 0,
    auth_type: '',
    auth_api_key: '',
    auth_header_name: '',
    auth_token_url: '',
    auth_client_id: '',
    auth_client_secret: '',
    auth_scope: '',
    auth_access_token: '',
    auth_refresh_token: '',
    enabled: true,
    require_user_credentials: false,
  };
}

const authTypeOptions = [
  { label: '无', value: '' },
  { label: 'API Key / Bearer', value: 'api_key' },
  { label: 'OAuth2 静态 Token', value: 'oauth2_static' },
  { label: 'OAuth2 Client Credentials', value: 'oauth2_client_credentials' },
  { label: 'OAuth2 Refresh Token', value: 'oauth2_refresh' },
];

const isOAuthAuth = computed(() => form.auth_type.startsWith('oauth2'));

function resetForm() {
  serverError.value = '';
  const row = props.row;
  const config = parseJSON<McpServerConfig>(row?.config_json, {});
  Object.assign(form, {
    name: row?.key || '',
    display_name: row?.name || '',
    description: row?.description || '',
    transport: config.transport || 'streamable_http',
    url: config.url || '',
    command: config.command || '',
    argsText: (config.args || []).join('\n'),
    headers: recordToPairs(config.headers || {}),
    env: recordToPairs(config.env || {}),
    tool_prefix: config.tool_prefix || '',
    timeout_sec: config.timeout_sec || 60,
    session_reconnect_max: config.session_reconnect_max || 0,
    auth_type: config.auth?.type || '',
    auth_api_key: config.auth?.api_key || '',
    auth_header_name: config.auth?.header_name || '',
    auth_token_url: config.auth?.token_url || '',
    auth_client_id: config.auth?.client_id || '',
    auth_client_secret: config.auth?.client_secret || '',
    auth_scope: config.auth?.scope || '',
    auth_access_token: config.auth?.access_token || '',
    auth_refresh_token: config.auth?.refresh_token || '',
    enabled: row?.enabled ?? true,
    require_user_credentials: Boolean(config.require_user_credentials),
  });
}

async function save() {
  await persist({ close: true, notify: true });
}

async function runValidate() {
  validating.value = true;
  serverError.value = '';
  try {
    const payload = buildPayload();
    const result = await mcpStore.validate(payload.enabled ?? true, payload.config_json ?? '{}');
    $q.notify({
      type: result.ok ? 'positive' : 'warning',
      message: result.message || (result.ok ? '配置有效' : result.status),
    });
  } catch (err) {
    serverError.value = err instanceof Error ? err.message : '预检失败';
    $q.notify({ type: 'negative', message: serverError.value });
  } finally {
    validating.value = false;
  }
}

async function saveAndTest() {
  testing.value = true;
  try {
    const saved = await persist({ close: false, notify: false });
    const result = await mcpStore.test(saved.id);
    emit('tested');
    emit('update:modelValue', false);
    $q.notify({ type: result.ok ? 'positive' : 'warning', message: result.message || result.status });
  } catch (err) {
    serverError.value = err instanceof Error ? err.message : '测试连接失败';
    $q.notify({ type: 'negative', message: serverError.value });
  } finally {
    testing.value = false;
  }
}

async function persist(options: { close: boolean; notify: boolean }) {
  serverError.value = '';
  saving.value = true;
  try {
    const payload = buildPayload();
    const saved = props.row ? await mcpStore.editServer(props.row.id, payload) : await mcpStore.addServer(payload);
    emit('saved', saved);
    if (options.close) emit('update:modelValue', false);
    if (options.notify) $q.notify({ type: 'positive', message: 'MCP 服务器已保存' });
    return saved;
  } catch (err) {
    serverError.value = err instanceof Error ? err.message : '保存失败';
    $q.notify({ type: 'negative', message: serverError.value });
    throw err;
  } finally {
    saving.value = false;
  }
}

function buildPayload(): PlatformResourceInput {
  const existingMetadata = parseJSON<McpServerMetadata>(props.row?.metadata_json, {});
  const config: McpServerConfig = {
    transport: form.transport,
    url: usesUrl.value ? form.url.trim() : '',
    command: usesUrl.value ? '' : form.command.trim(),
    args: usesUrl.value
      ? []
      : form.argsText
          .split(/\r?\n/)
          .map((item) => item.trim())
          .filter(Boolean),
    headers: usesUrl.value ? pairsToRecord(form.headers) : {},
    env: pairsToRecord(form.env),
    tool_prefix: form.tool_prefix.trim(),
    timeout_sec: Number(form.timeout_sec) || 60,
    session_reconnect_max: Number(form.session_reconnect_max) || 0,
    require_user_credentials: form.require_user_credentials,
  };
  if (form.auth_type) {
    const auth: McpServerConfig['auth'] = {
      type: form.auth_type,
      api_key: form.auth_api_key.trim(),
      header_name: form.auth_header_name.trim(),
      token_url: form.auth_token_url.trim(),
      client_id: form.auth_client_id.trim(),
      client_secret: form.auth_client_secret.trim(),
      scope: form.auth_scope.trim(),
      access_token: form.auth_access_token.trim(),
      refresh_token: form.auth_refresh_token.trim(),
    };
    if (isOAuthAuth.value ? auth.access_token || auth.client_id : auth.api_key) {
      config.auth = auth;
    }
  }
  return {
    key: form.name.trim(),
    name: form.display_name.trim() || form.name.trim(),
    description: form.description.trim(),
    enabled: form.enabled,
    status: props.row?.status || 'active',
    sort_order: props.row?.sort_order || 0,
    config_json: JSON.stringify(config),
    metadata_json: JSON.stringify(existingMetadata),
  };
}

function addPair(field: 'headers' | 'env') {
  form[field].push({ key: '', value: '' });
}

function removePair(field: 'headers' | 'env', index: number) {
  form[field].splice(index, 1);
}

function recordToPairs(record: Record<string, string>): McpKeyValue[] {
  return Object.entries(record).map(([key, value]) => ({ key, value: String(value ?? '') }));
}

function pairsToRecord(pairs: McpKeyValue[]) {
  return Object.fromEntries(pairs.map((item) => [item.key.trim(), item.value]).filter(([key]) => key));
}

const slugPattern = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;

function slugRule(value: string) {
  return slugPattern.test(value) || '仅支持小写字母、数字、连字符，且不能以连字符开头或结尾';
}

function urlRule(value: string) {
  return isHttpUrl(value) || '请输入有效的 HTTP(S) URL';
}

function commandRule(value: string) {
  return Boolean(value.trim()) || 'stdio 传输需要填写 command';
}

function isHttpUrl(value: string) {
  try {
    const parsed = new URL(value);
    return parsed.protocol === 'http:' || parsed.protocol === 'https:';
  } catch {
    return false;
  }
}

function isSensitiveKey(key: string) {
  return /authorization|token|secret|password|key/i.test(key);
}
</script>
