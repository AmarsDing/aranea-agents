// W2a D3 笔记视图（spec §6.3）：渲染 + 查询口径 + 域面收敛。
// - 默认查 kind=note + scope_type=user；切公司空间查 workspace。
// - 视图域面只有 个人/公司空间 两个入口（无 agent/team/global 选项）——
//   无权限域不可见由服务端 authorizeMemoryScope 强制，客户端不暴露其它域。
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { mount, flushPromises } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import MemoryNotesTab from '../MemoryNotesTab.vue';
import { listMemoryFacts } from '../../api';
import type { MemoryFact } from '../../types';
import zhCN from '../../../../i18n/locales/zh-CN';

vi.mock('../../api', () => ({
  listMemoryFacts: vi.fn(),
}));

const mockListFacts = vi.mocked(listMemoryFacts);

const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } });

const quasarStubs = {
  'q-banner': { template: '<div><slot /></div>' },
  'q-card': { template: '<div><slot /></div>' },
  'q-card-section': { template: '<div><slot /></div>' },
  'q-chip': {
    template: '<button type="button" v-bind="$attrs"><slot /></button>',
  },
  'q-input': { template: '<div><slot name="prepend" /><slot /></div>' },
  'q-icon': { template: '<i />' },
  'q-btn': { template: '<button type="button" v-bind="$attrs"><slot /></button>' },
  'q-list': { template: '<div><slot /></div>' },
  'q-item': { template: '<div><slot /></div>' },
  'q-item-section': { template: '<div><slot /></div>' },
  'q-item-label': { template: '<div><slot /></div>' },
  'q-pagination': { template: '<nav />' },
};

function mkFact(over: Partial<MemoryFact> = {}): MemoryFact {
  return {
    id: 'wbmem-m_a1',
    scope_type: 'user',
    scope_id: '1001',
    workspace_id: '',
    user_id: '1001',
    team_id: '',
    agent_id: '',
    statement: '迁移样例记忆',
    details_markdown: '',
    fact_kind: 'note',
    tags_json: '[]',
    confidence: 1,
    importance: 1,
    use_count: 0,
    hit_count: 0,
    recalled_count: 0,
    injected_count: 0,
    cited_count: 0,
    positive_feedback_count: 0,
    negative_feedback_count: 0,
    conflict_count: 0,
    source_kind: 'manual',
    source_episode_id: '',
    source_session_id: '',
    source_message_id: '',
    version: 1,
    status: 'active',
    pii_flag: false,
    pii_types: [],
    quality_score: 0.5,
    created_at: '2026-11-10T06:00:00Z',
    updated_at: '2026-11-10T06:00:00Z',
    ...over,
  };
}

function mountTab() {
  return mount(MemoryNotesTab, {
    global: { plugins: [i18n], stubs: quasarStubs },
  });
}

describe('MemoryNotesTab（W2a D3 笔记视图）', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockListFacts.mockResolvedValue({ items: [mkFact()], total: 1, limit: 10, offset: 0 });
  });

  it('默认查 kind=note + scope_type=user 并渲染笔记条目', async () => {
    const wrapper = mountTab();
    await flushPromises();

    expect(mockListFacts).toHaveBeenCalledWith(
      expect.objectContaining({ kind: 'note', scope_type: 'user', status: 'active' }),
    );
    expect(wrapper.text()).toContain('迁移样例记忆');
    expect(wrapper.findAll('.memory-note-statement')).toHaveLength(1);
  });

  it('域面只暴露 个人/公司空间 两个入口（无 agent/team/global 选项）', async () => {
    const wrapper = mountTab();
    await flushPromises();

    const text = wrapper.text();
    expect(text).toContain('个人');
    expect(text).toContain('公司空间');
    expect(text).not.toContain('智能体');
    expect(text).not.toContain('团队');
    expect(text).not.toContain('全局');
  });

  it('点击公司空间后改查 scope_type=workspace', async () => {
    const wrapper = mountTab();
    await flushPromises();

    const workspaceChip = wrapper.findAll('button').find((b) => b.text() === '公司空间');
    expect(workspaceChip).toBeDefined();
    await workspaceChip!.trigger('click');
    await flushPromises();

    expect(mockListFacts).toHaveBeenLastCalledWith(expect.objectContaining({ kind: 'note', scope_type: 'workspace' }));
  });

  it('查询失败渲染不可用横幅且列表为空', async () => {
    mockListFacts.mockRejectedValueOnce(new Error('gateway down'));
    const wrapper = mountTab();
    await flushPromises();

    expect(wrapper.text()).toContain('笔记列表暂时不可用');
    expect(wrapper.findAll('.memory-note-statement')).toHaveLength(0);
  });

  it('空结果渲染空态文案', async () => {
    mockListFacts.mockResolvedValueOnce({ items: [], total: 0, limit: 10, offset: 0 });
    const wrapper = mountTab();
    await flushPromises();

    expect(wrapper.text()).toContain('暂无笔记');
  });
});
