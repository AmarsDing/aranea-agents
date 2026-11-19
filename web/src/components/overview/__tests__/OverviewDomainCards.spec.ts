import { flushPromises, mount } from '@vue/test-utils';
import { defineComponent } from 'vue';
import { createI18n } from 'vue-i18n';
import { describe, expect, it, vi } from 'vitest';
import zh from '../../../i18n/locales/zh-CN';
import OverviewDomainCards from '../OverviewDomainCards.vue';
import { useHarnessCounts } from '../../../features/usage/useHarnessCounts';

vi.mock('../../../features/tools/api', () => ({ listTools: vi.fn(async () => ({ total: 111 })) }));
vi.mock('../../../features/skills/api', () => ({ listSkills: vi.fn(async () => ({ total: 7 })) }));
vi.mock('../../../features/mcp/api', () => ({
  listMcpServersPaged: vi.fn(async () => {
    throw new Error('boom');
  }),
}));
vi.mock('../../../features/channels/api', () => ({ listChannelsPaged: vi.fn(async () => ({ total: 3 })) }));
vi.mock('../../../features/credentials/api', () => ({ listCredentials: vi.fn(async () => ({ total: 9 })) }));
vi.mock('../../../features/graph/api', () => ({ listGraphs: vi.fn(async () => ({ items: [1, 2] })) }));
vi.mock('../../../features/cron/api', () => ({ listCronTasks: vi.fn(async () => [1, 2, 3, 4]) }));

const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zh } });

describe('基座总览 · 领域卡', () => {
  it('每张卡带名称、问题和几个数；没读到的数写「—」，不显示成 0', () => {
    const wrapper = mount(OverviewDomainCards, {
      props: {
        cards: [
          {
            key: 'capability',
            icon: 'extension',
            title: '能力',
            question: '它们能用什么、知道什么',
            to: '/tools',
            facts: [{ label: '工具', value: 111 }, { label: 'MCP' }, { label: '技能', value: 0 }],
          },
        ],
      },
      global: {
        plugins: [i18n],
        stubs: { 'q-icon': true, 'router-link': { props: ['to'], template: '<a :href="to"><slot /></a>' } },
      },
    });
    const card = wrapper.get('.ov-domain');
    expect(card.attributes('href')).toBe('/tools');
    expect(card.findAll('.ov-domain__facts > span').map((el) => el.text().replace(/\s+/g, ''))).toEqual([
      '工具111',
      'MCP—',
      '技能0',
    ]);
  });
});

describe('基座总览 · 各领域的数', () => {
  const run = async (canSee: (path: string) => boolean) => {
    let counts: ReturnType<typeof useHarnessCounts>['counts'] | undefined;
    mount(
      defineComponent({
        setup() {
          counts = useHarnessCounts(canSee).counts;
          return () => null;
        },
      }),
    );
    await flushPromises();
    return counts?.value;
  };

  it('各数独立读取：一个接口失败只少这一个数，其余照常', async () => {
    vi.spyOn(console, 'warn').mockImplementation(() => undefined);
    expect(await run(() => true)).toEqual({ tools: 111, skills: 7, channels: 3, credentials: 9, graphs: 2, cron: 4 });
  });

  it('没有权限看的页面不发请求，也没有这个数', async () => {
    const counts = await run((path) => path === '/tools' || path === '/cron');
    expect(counts).toEqual({ tools: 111, cron: 4 });
  });
});
