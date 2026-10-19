import { describe, it, expect, vi, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { reactive, ref } from 'vue';
import { useAgentA2AProxyTab } from '../useAgentA2AProxyTab';
import { discoverRemoteAgent } from '../../a2a/api';

const notify = vi.fn();
const patch = vi.fn();
const fetchById = vi.fn();

vi.mock('quasar', () => ({
  useQuasar: () => ({ notify }),
}));

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, args?: Record<string, unknown>) => (args ? `${key} ${JSON.stringify(args)}` : key),
  }),
}));

vi.mock('../../../stores/agents', () => ({
  useAgentDetailStore: () => reactive({ saving: ref(false), patch, fetchById }),
}));

vi.mock('../../a2a/api', () => ({
  discoverRemoteAgent: vi.fn(),
}));

const discoverMock = vi.mocked(discoverRemoteAgent);

function makeTab(a2aProxy?: unknown) {
  return useAgentA2AProxyTab(
    () => 'agent-1',
    () => a2aProxy as never,
  );
}

describe('useAgentA2AProxyTab', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    notify.mockClear();
    patch.mockClear();
    fetchById.mockReset();
    discoverMock.mockReset();
  });

  describe('testConnection', () => {
    it('calls discoverRemoteAgent with built auth json and notifies success', async () => {
      const tab = makeTab();
      tab.proxyForm.remote_url = 'https://remote.example.com';
      tab.proxyForm.auth_type = 'bearer';
      tab.authSecret.value = 'secret-token';
      discoverMock.mockResolvedValue({
        agent_id: 'remote-1',
        display_name: 'Remote Agent',
        workspace: 'ws',
        enabled: true,
        capabilities: [{ name: 'chat' }],
        updated_at: '',
      } as never);

      await tab.testConnection();

      expect(discoverMock).toHaveBeenCalledWith({
        remote_url: 'https://remote.example.com',
        auth_type: 'bearer',
        auth_config_json: JSON.stringify({ token: 'secret-token' }),
      });
      const success = notify.mock.calls.find((c) => c[0]?.type === 'positive');
      expect(success, 'expected a positive notify').toBeTruthy();
      expect(success?.[0]?.message).toContain('Remote Agent');
    });

    it('rejects empty remote_url without calling the API', async () => {
      const tab = makeTab();
      tab.proxyForm.remote_url = '  ';

      await tab.testConnection();

      expect(discoverMock).not.toHaveBeenCalled();
      expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'negative' }));
    });

    it('rejects api_key auth without secret', async () => {
      const tab = makeTab();
      tab.proxyForm.remote_url = 'https://remote.example.com';
      tab.proxyForm.auth_type = 'api_key';
      tab.authSecret.value = '';

      await tab.testConnection();

      expect(discoverMock).not.toHaveBeenCalled();
      expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'negative' }));
    });

    it('rejects mtls without key_file', async () => {
      const tab = makeTab();
      tab.proxyForm.remote_url = 'https://remote.example.com';
      tab.proxyForm.auth_type = 'mtls';
      tab.mtls.cert_file = '/certs/client.crt';
      tab.mtls.key_file = '';

      await tab.testConnection();

      expect(discoverMock).not.toHaveBeenCalled();
      expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'negative' }));
    });

    it('rejects non-http(s) remote_url without calling the API', async () => {
      const tab = makeTab();
      tab.proxyForm.remote_url = 'ftp://remote.example.com';

      await tab.testConnection();

      expect(discoverMock).not.toHaveBeenCalled();
      expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'negative' }));
    });

    it('notifies negative when the API call fails', async () => {
      const tab = makeTab();
      tab.proxyForm.remote_url = 'https://remote.example.com';
      discoverMock.mockRejectedValue(new Error('connection refused'));

      await tab.testConnection();

      expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'negative', message: 'connection refused' }));
    });
  });

  describe('saveProxy validation', () => {
    it('rejects empty remote_url without patching', async () => {
      const tab = makeTab();
      tab.proxyForm.remote_url = '';

      await tab.saveProxy();

      expect(patch).not.toHaveBeenCalled();
      expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'negative' }));
    });

    it('echoes the stored catalog fields on save (full-replace contract)', async () => {
      const tab = makeTab();
      tab.proxyForm.remote_url = 'https://remote.example.com';
      fetchById.mockResolvedValue({
        id: 'agent-1',
        agent_key: 'proxy-agent',
        display_name: 'Proxy Agent',
        provider: 'openai',
        model: 'gpt-4o',
        status: 'active',
        is_default: true,
        is_favorite: false,
        icon: 'link',
        agent_description: 'desc',
        taxonomy_position_id: 'pos-1',
        system_prompt_mode: 'complete',
        context_window: 128000,
        budget_monthly_cents: 500,
        agent_kind: 'a2a_proxy',
        config_json: '{"evaluation":{}}',
        settings: { memory_enabled: true },
        files: [{ name: 'system.md' }],
      });
      patch.mockResolvedValue({});

      await tab.saveProxy();

      expect(fetchById).toHaveBeenCalledWith('agent-1');
      expect(patch).toHaveBeenCalledTimes(1);
      const [id, payload] = patch.mock.calls[0] as [string, Record<string, unknown>];
      expect(id).toBe('agent-1');
      // Catalog echo: is_default/icon/description must NOT be reset by the save.
      expect(payload.is_default).toBe(true);
      expect(payload.icon).toBe('link');
      expect(payload.provider).toBe('openai');
      // settings/files/config_json are managed by their own paths — excluded.
      expect(payload.settings).toBeUndefined();
      expect(payload.files).toBeUndefined();
      expect(payload.config_json).toBeUndefined();
      // Proxy payload carries the validated form.
      const proxy = payload.a2a_proxy_config as { remote_url?: string; auth_type?: string };
      expect(proxy.remote_url).toBe('https://remote.example.com');
      expect(proxy.auth_type).toBeUndefined();
      expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'positive' }));
    });
  });
});
