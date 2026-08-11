import { reactive, ref, watch } from 'vue';
import { storeToRefs } from 'pinia';
import { useQuasar } from 'quasar';
import type { A2AProxyConfig } from './types';
import { useAgentDetailStore } from '../../stores/agents';
import { buildA2AAuthJSON, A2A_AUTH_TYPE_OPTIONS } from '../a2a/authUtils';
import { discoverRemoteAgent } from '../a2a/api';

export function useAgentA2AProxyTab(
  agentId: () => string,
  a2aProxy: () => A2AProxyConfig | undefined,
  onSaved?: () => void,
) {
  const $q = useQuasar();
  const detailStore = useAgentDetailStore();
  const { saving } = storeToRefs(detailStore);
  const showSecret = ref(false);
  const authSecret = ref('');
  const mtls = reactive({ cert_file: '', key_file: '', ca_file: '' });
  const authTypeOptions = A2A_AUTH_TYPE_OPTIONS;
  const proxyForm = reactive<A2AProxyConfig>({
    remote_url: '',
    enable_streaming: true,
    timeout_seconds: 30,
    auth_type: 'none',
  });

  function parseAuthConfig(raw?: string, authType?: string) {
    authSecret.value = '';
    mtls.cert_file = '';
    mtls.key_file = '';
    mtls.ca_file = '';
    if (!raw) return;
    try {
      const parsed = JSON.parse(raw) as {
        api_key?: string;
        token?: string;
        cert_file?: string;
        key_file?: string;
        ca_file?: string;
      };
      if (authType === 'mtls') {
        mtls.cert_file = parsed.cert_file ?? '';
        mtls.key_file = parsed.key_file ?? '';
        mtls.ca_file = parsed.ca_file ?? '';
        return;
      }
      authSecret.value = parsed.api_key ?? parsed.token ?? '';
    } catch {
      authSecret.value = '';
    }
  }

  function buildAuthConfigJson(): string | undefined {
    return buildA2AAuthJSON(proxyForm.auth_type ?? 'none', authSecret.value, mtls);
  }

  watch(
    a2aProxy,
    (cfg) => {
      if (!cfg) return;
      proxyForm.remote_url = cfg.remote_url ?? '';
      proxyForm.enable_streaming = cfg.enable_streaming ?? true;
      proxyForm.timeout_seconds = cfg.timeout_seconds ?? 30;
      proxyForm.auth_type = cfg.auth_type ?? 'none';
      parseAuthConfig(cfg.auth_config_json, proxyForm.auth_type);
    },
    { immediate: true },
  );

  async function saveProxy() {
    const errMsg = validateProxyForm();
    if (errMsg) {
      $q.notify({ type: 'negative', message: errMsg });
      return;
    }
    try {
      const authType = proxyForm.auth_type?.trim() || 'none';
      const payload: A2AProxyConfig = {
        ...proxyForm,
        auth_type: authType === 'none' ? undefined : authType,
        auth_config_json: buildAuthConfigJson(),
      };
      await detailStore.patch(agentId(), { a2a_proxy_config: payload });
      $q.notify({ type: 'positive', message: 'A2A 代理配置已保存' });
      onSaved?.();
    } catch (error) {
      $q.notify({ type: 'negative', message: error instanceof Error ? error.message : '保存失败' });
    }
  }

  /** 返回校验错误消息；null 表示通过。saveProxy 与 testConnection 共用。 */
  function validateProxyForm(): string | null {
    const url = proxyForm.remote_url.trim();
    if (!url) return '远程 URL 不能为空';
    if (!/^https?:\/\//i.test(url)) return '远程 URL 必须以 http:// 或 https:// 开头';
    const authType = proxyForm.auth_type?.trim() || 'none';
    if ((authType === 'api_key' || authType === 'bearer') && !authSecret.value.trim()) {
      return '请填写鉴权密钥';
    }
    if (authType === 'mtls' && (!mtls.cert_file.trim() || !mtls.key_file.trim())) {
      return 'mTLS 需填写 cert_file 与 key_file';
    }
    return null;
  }

  const testing = ref(false);

  /** 复用 DiscoverRemoteAgent 做连接测试，不落库。 */
  async function testConnection() {
    const errMsg = validateProxyForm();
    if (errMsg) {
      $q.notify({ type: 'negative', message: errMsg });
      return;
    }
    const authType = proxyForm.auth_type?.trim() || 'none';
    testing.value = true;
    try {
      const card = await discoverRemoteAgent({
        remote_url: proxyForm.remote_url.trim(),
        auth_type: authType === 'none' ? undefined : authType,
        auth_config_json: buildAuthConfigJson(),
      });
      const name = card.display_name || card.agent_id || '远程 Agent';
      $q.notify({ type: 'positive', message: `连接成功：${name}（${card.capabilities?.length ?? 0} 个能力）` });
    } catch (error) {
      $q.notify({ type: 'negative', message: error instanceof Error ? error.message : '连接失败' });
    } finally {
      testing.value = false;
    }
  }

  return {
    saving,
    testing,
    showSecret,
    authSecret,
    mtls,
    authTypeOptions,
    proxyForm,
    saveProxy,
    testConnection,
  };
}
