import { computed, reactive, ref, watch } from 'vue';
import { useQuasar } from 'quasar';
import type { PlatformResourceInput } from '../platform/types';
import type { McpKeyValue, McpServerConfig, McpServerFormValue, McpServerMetadata, McpServerRow } from './types';
import { parseJSON } from './utils';
import { useMcpStore } from '../../stores/mcp';

const slugPattern = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;

const transportOptions = [
  { label: 'stdio', value: 'stdio' },
  { label: 'SSE', value: 'sse' },
  { label: 'Streamable HTTP', value: 'streamable_http' },
];

const authTypeOptions = [
  { label: '无', value: '' },
  { label: 'API Key / Bearer', value: 'api_key' },
  { label: 'OAuth2 静态 Token', value: 'oauth2_static' },
  { label: 'OAuth2 Client Credentials', value: 'oauth2_client_credentials' },
  { label: 'OAuth2 Refresh Token', value: 'oauth2_refresh' },
];

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

function recordToPairs(record: Record<string, string>): McpKeyValue[] {
  return Object.entries(record).map(([key, value]) => ({ key, value: String(value ?? '') }));
}

function pairsToRecord(pairs: McpKeyValue[]) {
  return Object.fromEntries(pairs.map((item) => [item.key.trim(), item.value]).filter(([key]) => key));
}

function slugRule(value: string) {
  return slugPattern.test(value) || '仅支持小写字母、数字、连字符，且不能以连字符开头或结尾';
}

function urlRule(value: string) {
  return isHttpUrl(value) || '请输入有效的 HTTP(S) URL';
}

function commandRule(value: string) {
  return Boolean(value.trim()) || 'stdio 传输需要填写 command';
}

export function useMcpServerForm(
  props: { modelValue: boolean; row: McpServerRow | null },
  emit: {
    (e: 'update:modelValue', value: boolean): void;
    (e: 'saved', row: McpServerRow): void;
    (e: 'tested'): void;
  },
) {
  const $q = useQuasar();
  const mcpStore = useMcpStore();

  const saving = ref(false);
  const testing = ref(false);
  const validating = ref(false);
  const serverError = ref('');
  const form = reactive<McpServerFormValue>(emptyForm());

  const usesUrl = computed(() => form.transport === 'sse' || form.transport === 'streamable_http');
  const canSave = computed(
    () => slugPattern.test(form.name) && (usesUrl.value ? isHttpUrl(form.url) : Boolean(form.command.trim())),
  );
  const isOAuthAuth = computed(() => form.auth_type.startsWith('oauth2'));

  watch(
    () => props.modelValue,
    (open) => {
      if (open) resetForm();
    },
  );

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

  return {
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
    save,
    runValidate,
    saveAndTest,
    addPair,
    removePair,
    slugRule,
    urlRule,
    commandRule,
    isSensitiveKey,
  };
}
