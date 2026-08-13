// useMcpServerForm — buildPayload 行为契约：
// 传输切换时互斥字段清理（stdio ↔ http）、args 逐行解析、auth 仅在有凭据字段时注入、
// 数值字段回退默认、编辑路径保留既有 metadata/status。
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { reactive, nextTick } from 'vue';
import { useMcpServerForm } from '../useMcpServerForm';
import { createMcpServer, updateMcpServer } from '../api';
import type { PlatformResourceInput } from '../../platform/types';
import type { McpServerRow } from '../types';

vi.mock('quasar', () => ({
  useQuasar: () => ({ notify: vi.fn(), dialog: vi.fn(() => ({ onOk: vi.fn() })) }),
}));

vi.mock('../api', () => ({
  listMcpServers: vi.fn(async () => []),
  listMcpServersPaged: vi.fn(async () => ({ items: [], total: 0, page: 1, page_size: 20 })),
  createMcpServer: vi.fn(async (payload: PlatformResourceInput) => ({ id: 'mcp-new', ...payload })),
  updateMcpServer: vi.fn(async (id: string, payload: Partial<PlatformResourceInput>) => ({ id, ...payload })),
  deleteMcpServer: vi.fn(async () => {}),
  testMcpServer: vi.fn(),
  validateMcpServer: vi.fn(async () => ({ ok: true, status: 'ok', message: '' })),
  listMcpUserCredentials: vi.fn(async () => []),
  upsertMcpUserCredential: vi.fn(),
  deleteMcpUserCredential: vi.fn(async () => {}),
}));

function makeRow(overrides: Partial<McpServerRow> = {}): McpServerRow {
  return {
    id: 'mcp-1',
    resource: 'mcp-servers',
    key: 'existing-srv',
    name: 'Existing',
    description: '',
    status: 'deprecated',
    enabled: true,
    sort_order: 3,
    parent_id: '',
    level: '',
    agent_id: '',
    provider: '',
    model: '',
    is_system: false,
    config_json: '{}',
    metadata_json: '{}',
    dept_lead_agent_id: '',
    dept_lead_config_json: '{}',
    created_at: '2025-01-01T00:00:00Z',
    updated_at: '2025-01-01T00:00:00Z',
    deleted_at: '',
    ...overrides,
  };
}

function setup(row: McpServerRow | null = null) {
  const props = reactive({ modelValue: false, row });
  const emit = vi.fn();
  const form = useMcpServerForm(props, emit);
  return { props, emit, ...form };
}

async function open(props: { modelValue: boolean }) {
  props.modelValue = true;
  await nextTick();
}

const createMock = vi.mocked(createMcpServer);
const updateMock = vi.mocked(updateMcpServer);

function payloadOf(call: [PlatformResourceInput] | [string, Partial<PlatformResourceInput>]) {
  const payload = (call.length === 1 ? call[0] : call[1]) as PlatformResourceInput;
  return { payload, config: JSON.parse(payload.config_json ?? '{}') as Record<string, unknown> };
}

describe('useMcpServerForm buildPayload', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.clearAllMocks();
  });

  it('stdio 传输：清空 url/headers，args 按行解析并过滤空行', async () => {
    const c = setup();
    await open(c.props);
    c.form.transport = 'stdio';
    c.form.name = 'local-fs';
    c.form.command = ' npx ';
    c.form.argsText = ' -y \n\n@mcp/server-filesystem \n /tmp ';
    c.form.url = 'https://ignored.example/mcp';
    c.form.headers = [{ key: 'X-H', value: 'v' }];
    c.form.env = [
      { key: 'E', value: '1' },
      { key: ' ', value: 'skip' },
    ];

    await c.save();

    expect(createMock).toHaveBeenCalledTimes(1);
    const { payload, config } = payloadOf(createMock.mock.calls[0]);
    expect(config.transport).toBe('stdio');
    expect(config.command).toBe('npx');
    expect(config.args).toEqual(['-y', '@mcp/server-filesystem', '/tmp']);
    expect(config.url).toBe('');
    expect(config.headers).toEqual({});
    expect(config.env).toEqual({ E: '1' });
    expect(payload.key).toBe('local-fs');
    // display_name 缺省回退为 key
    expect(payload.name).toBe('local-fs');
  });

  it('streamable_http 传输：清空 command/args，保留 url/headers', async () => {
    const c = setup();
    await open(c.props);
    c.form.transport = 'streamable_http';
    c.form.name = 'remote-srv';
    c.form.url = ' https://mcp.example.com/mcp ';
    c.form.headers = [{ key: 'X-A', value: '1' }];
    c.form.command = 'node';
    c.form.argsText = 'server.js';

    await c.save();

    const { config } = payloadOf(createMock.mock.calls[0]);
    expect(config.url).toBe('https://mcp.example.com/mcp');
    expect(config.headers).toEqual({ 'X-A': '1' });
    expect(config.command).toBe('');
    expect(config.args).toEqual([]);
  });

  it('auth：api_key 缺密钥不注入 auth；有密钥则注入', async () => {
    const c = setup();
    await open(c.props);
    c.form.name = 'no-auth';
    c.form.auth_type = 'api_key';
    c.form.auth_api_key = '   ';

    await c.save();
    expect(payloadOf(createMock.mock.calls[0]).config.auth).toBeUndefined();

    c.form.auth_api_key = ' secret-key ';
    c.form.auth_header_name = ' X-Api-Key ';
    await c.save();
    const auth = payloadOf(createMock.mock.calls[1]).config.auth as Record<string, unknown>;
    expect(auth.type).toBe('api_key');
    expect(auth.api_key).toBe('secret-key');
    expect(auth.header_name).toBe('X-Api-Key');
  });

  it('auth：oauth2 系列凭 access_token 或 client_id 判定注入', async () => {
    const c = setup();
    await open(c.props);
    c.form.name = 'oauth-srv';
    c.form.auth_type = 'oauth2_static';
    // access_token / client_id 均空 → 不注入
    await c.save();
    expect(payloadOf(createMock.mock.calls[0]).config.auth).toBeUndefined();

    c.form.auth_access_token = ' at-1 ';
    await c.save();
    const auth = payloadOf(createMock.mock.calls[1]).config.auth as Record<string, unknown>;
    expect(auth.type).toBe('oauth2_static');
    expect(auth.access_token).toBe('at-1');
  });

  it('数值字段：timeout_sec/session_reconnect_max 为 0 或非法值时回退默认', async () => {
    const c = setup();
    await open(c.props);
    c.form.name = 'defaults';
    c.form.timeout_sec = 0;
    c.form.session_reconnect_max = Number.NaN;

    await c.save();

    const { config } = payloadOf(createMock.mock.calls[0]);
    expect(config.timeout_sec).toBe(60);
    expect(config.session_reconnect_max).toBe(0);
  });

  it('编辑路径：走 updateMcpServer，保留既有 status 与 metadata_json', async () => {
    const row = makeRow({
      config_json: JSON.stringify({ transport: 'sse', url: 'https://old.example/sse' }),
      metadata_json: JSON.stringify({ health_status: 'ok', last_health_at: '2026-08-01T00:00:00Z' }),
    });
    const c = setup(row);
    await open(c.props);

    // resetForm 从 row 回填
    expect(c.form.name).toBe('existing-srv');
    expect(c.form.transport).toBe('sse');
    expect(c.form.url).toBe('https://old.example/sse');

    c.form.description = ' updated ';
    await c.save();

    expect(updateMock).toHaveBeenCalledTimes(1);
    expect(updateMock.mock.calls[0][0]).toBe('mcp-1');
    const { payload } = payloadOf(updateMock.mock.calls[0]);
    expect(payload.status).toBe('deprecated');
    expect(payload.sort_order).toBe(3);
    expect(JSON.parse(payload.metadata_json ?? '{}')).toEqual({
      health_status: 'ok',
      last_health_at: '2026-08-01T00:00:00Z',
    });
    expect(payload.description).toBe('updated');
  });
});
