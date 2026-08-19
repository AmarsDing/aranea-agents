// useMcpServersPage — healthTone / healthTooltip 分支契约：
// 健康点色调优先级 health_status > last_error_message > unknown；
// tooltip 优先级 last_error_message > 最近成功时间 > 未启用/未测试 i18n 文案。
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { mount } from '@vue/test-utils';
import { defineComponent } from 'vue';
import { useMcpServersPage } from '../useMcpServersPage';
import type { McpServerRow } from '../types';

vi.mock('quasar', () => ({
  useQuasar: () => ({ notify: vi.fn(), dialog: vi.fn(() => ({ onOk: vi.fn() })) }),
}));

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}));

vi.mock('../../../stores/auth', async () => {
  const { ref } = await import('vue');
  // storeToRefs 需要真 ref（plain null 会在 pinia storeToRefs 内炸 effect 读取）
  return { useAuthStore: () => ({ user: ref(null) }) };
});

vi.mock('../api', () => ({
  listMcpServers: vi.fn(async () => []),
  listMcpServersPaged: vi.fn(async () => ({ items: [], total: 0, page: 1, page_size: 20 })),
  createMcpServer: vi.fn(),
  updateMcpServer: vi.fn(),
  deleteMcpServer: vi.fn(),
  testMcpServer: vi.fn(),
  validateMcpServer: vi.fn(),
  listMcpUserCredentials: vi.fn(async () => []),
  upsertMcpUserCredential: vi.fn(),
  deleteMcpUserCredential: vi.fn(async () => {}),
}));

function makeRow(metadata: Record<string, unknown>, overrides: Partial<McpServerRow> = {}): McpServerRow {
  return {
    id: 'mcp-1',
    resource: 'mcp-servers',
    key: 'srv',
    name: 'Srv',
    description: '',
    status: 'active',
    enabled: true,
    sort_order: 0,
    parent_id: '',
    level: '',
    agent_id: '',
    provider: '',
    model: '',
    is_system: false,
    config_json: '{}',
    metadata_json: JSON.stringify(metadata),
    dept_lead_agent_id: '',
    dept_lead_config_json: '{}',
    created_at: '',
    updated_at: '',
    deleted_at: '',
    ...overrides,
  };
}

function setupPage() {
  let page!: ReturnType<typeof useMcpServersPage>;
  mount(
    defineComponent({
      setup() {
        page = useMcpServersPage();
        return () => null;
      },
    }),
  );
  return page;
}

describe('useMcpServersPage healthTone', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('health_status 直出：ok / error / degraded', () => {
    const page = setupPage();
    expect(page.healthTone(makeRow({ health_status: 'ok' }))).toBe('ok');
    expect(page.healthTone(makeRow({ health_status: 'error' }))).toBe('error');
    expect(page.healthTone(makeRow({ health_status: 'degraded' }))).toBe('degraded');
  });

  it('无 health_status 但有 last_error_message → error', () => {
    const page = setupPage();
    expect(page.healthTone(makeRow({ last_error_message: 'connect refused' }))).toBe('error');
  });

  it('既无状态也无错误 → unknown；metadata_json 非法 JSON → unknown', () => {
    const page = setupPage();
    expect(page.healthTone(makeRow({}))).toBe('unknown');
    const broken = makeRow({});
    broken.metadata_json = '{not-json';
    expect(page.healthTone(broken)).toBe('unknown');
  });
});

describe('useMcpServersPage healthTooltip', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('last_error_message 优先展示', () => {
    const page = setupPage();
    const row = makeRow({ health_status: 'ok', last_error_message: 'boom' });
    expect(page.healthTooltip(row)).toBe('boom');
  });

  it('ok + last_health_at → 最近成功时间', () => {
    const page = setupPage();
    const row = makeRow({ health_status: 'ok', last_health_at: '2026-08-01T12:00:00Z' });
    expect(page.healthTooltip(row)).toMatch(/^最近成功：/);
  });

  it('未启用 → notEnabledNotTested；已启用未探测 → notTested', () => {
    const page = setupPage();
    expect(page.healthTooltip(makeRow({}, { enabled: false }))).toBe('mcpPage.notEnabledNotTested');
    expect(page.healthTooltip(makeRow({}))).toBe('mcpPage.notTested');
  });

  it('P2：tool_count 存在时追加「已发现 N 个工具」', () => {
    const page = setupPage();
    const row = makeRow({ health_status: 'ok', last_health_at: '2026-08-01T12:00:00Z', tool_count: 5 });
    expect(page.healthTooltip(row)).toMatch(/^最近成功：.*；已发现 5 个工具$/);
  });

  it('P2：仅 tools_error_message（无 last-good 计数）→ 追加「工具发现失败」', () => {
    const page = setupPage();
    const row = makeRow({ health_status: 'ok', last_health_at: '2026-08-01T12:00:00Z', tools_error_message: 'boom' });
    expect(page.healthTooltip(row)).toMatch(/；工具发现失败$/);
  });

  it('P2：last-good 计数优先于错误记录（ApplyToolDiscoveryError 语义）', () => {
    const page = setupPage();
    const row = makeRow({ tool_count: 3, tools_error_message: 'recent fail' });
    expect(page.healthTooltip(row)).toBe('mcpPage.notTested；已发现 3 个工具');
  });
});
