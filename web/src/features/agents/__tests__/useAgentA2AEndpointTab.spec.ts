import { describe, it, expect, vi, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useAgentA2AEndpointTab } from '../useAgentA2AEndpointTab';

const notify = vi.fn();
const refreshCard = vi.fn();
const updateCard = vi.fn();

vi.mock('quasar', () => ({
  useQuasar: () => ({ notify }),
}));

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}));

vi.mock('../../../stores/a2a', () => ({
  useA2AStore: () => ({ refreshCard, updateCard }),
}));

function makeCard(capabilities: Array<{ name: string; description?: string }>) {
  return {
    agent_id: 'agent-1',
    display_name: 'Agent',
    workspace: 'ws',
    enabled: true,
    capabilities: capabilities.map((c) => ({
      name: c.name,
      description: c.description ?? c.name,
      input_schema_json: '{}',
      output_schema_json: '{}',
    })),
    updated_at: '',
  };
}

async function loadTab(capabilities: Array<{ name: string; description?: string }>) {
  refreshCard.mockResolvedValue(makeCard(capabilities));
  const tab = useAgentA2AEndpointTab(() => 'agent-1');
  // onMounted(loadCard) does not fire outside a component; invoke explicitly.
  await tab.loadCard();
  return tab;
}

describe('useAgentA2AEndpointTab capabilityLines', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    notify.mockClear();
    refreshCard.mockReset();
    updateCard.mockReset();
  });

  it('renders "name: description" lines when description differs from name', async () => {
    const tab = await loadTab([{ name: 'chat', description: '对话能力' }, { name: 'search' }]);
    expect(tab.capabilityLines.value).toBe('chat: 对话能力\nsearch');
  });

  it('parses "name: description" lines into capabilities', async () => {
    const tab = await loadTab([]);
    tab.capabilityLines.value = 'chat: 对话能力\nsearch\nsummarize: 总结长文';
    const caps = tab.card.value?.capabilities ?? [];
    expect(caps.map((c) => c.name)).toEqual(['chat', 'search', 'summarize']);
    expect(caps[0]?.description).toBe('对话能力');
    expect(caps[1]?.description).toBe('search');
    expect(caps[2]?.description).toBe('总结长文');
  });

  it('keeps description text after the first colon only', async () => {
    const tab = await loadTab([]);
    tab.capabilityLines.value = 'chat: 支持 a:b 格式';
    expect(tab.card.value?.capabilities[0]?.description).toBe('支持 a:b 格式');
  });
});
