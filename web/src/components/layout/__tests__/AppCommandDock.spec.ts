import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils';
import { nextTick, ref } from 'vue';
import { createI18n } from 'vue-i18n';
import AppCommandDock from '../AppCommandDock.vue';

const quick = vi.hoisted(() => ({ quickAdd: vi.fn(), notify: vi.fn(), push: vi.fn() }));
vi.mock('../../../features/tasks/quickAdd', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../../features/tasks/quickAdd')>()),
  quickAdd: quick.quickAdd,
}));
vi.mock('../../../features/tasks/useTaskDispatch', () => ({
  useTaskDispatch: () => ({
    recipients: ref([{ kind: 'agent', id: 'a1', key: 'writer', name: '写手' }]),
    ensureRecipients: vi.fn(async () => undefined),
    dispatch: vi.fn(),
  }),
}));
vi.mock('quasar', async (importOriginal) => ({
  ...(await importOriginal<typeof import('quasar')>()),
  useQuasar: () => ({ notify: quick.notify }),
}));
vi.mock('vue-router', () => ({ useRouter: () => ({ push: quick.push }) }));

const wrappers: VueWrapper[] = [];
const i18n = createI18n({ legacy: false, locale: 'zh-CN', missingWarn: false, fallbackWarn: false });

function setup() {
  const wrapper = mount(AppCommandDock, {
    attachTo: document.body,
    global: {
      plugins: [i18n],
      stubs: {
        QIcon: true,
        QTooltip: true,
        QBtn: {
          props: ['type', 'label'],
          template: '<button :type="type || \'button\'" :data-label="label" @click="$emit(\'click\')" />',
        },
        // 不等过渡动画：收回后面板立刻从 DOM 里拿掉
        Transition: { template: '<slot />' },
      },
    },
  });
  wrappers.push(wrapper);
  return wrapper;
}

async function openDock(wrapper: VueWrapper) {
  await wrapper.find('.app-dock__tab').trigger('click');
  await nextTick();
}

beforeEach(() => {
  window.localStorage.clear();
  vi.clearAllMocks();
});

afterEach(() => {
  wrappers.splice(0).forEach((wrapper) => wrapper.unmount());
  document.body.innerHTML = '';
});

describe('AppCommandDock', () => {
  it('starts as the bottom tab and expands with the input focused', async () => {
    const wrapper = setup();
    expect(wrapper.find('.app-dock__panel').exists()).toBe(false);
    expect(document.documentElement.style.getPropertyValue('--app-dock-space')).toBe('40px');

    await openDock(wrapper);
    expect(wrapper.find('.app-dock__panel').exists()).toBe(true);
    expect(document.activeElement).toBe(wrapper.find('input[type="text"]').element);
  });

  it('send puts the text (and any files) into My tasks, then stays open and clears', async () => {
    quick.quickAdd.mockResolvedValue({
      ok: true,
      kind: 'task',
      task: null,
      recipient: null,
      link: '',
      failedFiles: [],
    });
    const wrapper = setup();
    await openDock(wrapper);
    // 一行里只有：回形针、输入、发送、展开交办面板、收起（没有标题、说明、添加按钮）
    expect(wrapper.find('.app-dock__head').exists()).toBe(false);
    expect(wrapper.find('.app-dock__hint').exists()).toBe(false);
    expect(wrapper.find('.app-dock__add').exists()).toBe(false);
    expect(wrapper.find('.app-dock__collapse').exists()).toBe(true);

    await wrapper.find('input[type="text"]').setValue('  写周报  ');
    await wrapper.find('form').trigger('submit');
    await flushPromises();
    expect(quick.quickAdd).toHaveBeenCalledWith('写周报', [], expect.anything());
    expect(quick.notify).toHaveBeenCalled();
    expect(wrapper.find('.app-dock__panel').exists()).toBe(true);
    expect((wrapper.find('input[type="text"]').element as HTMLInputElement).value).toBe('');

    const file = new File(['x'], '数据.csv', { type: 'text/csv' });
    const picker = wrapper.find('input[type="file"]');
    Object.defineProperty(picker.element, 'files', { value: [file], configurable: true });
    await picker.trigger('change');
    expect(wrapper.findAll('.app-dock__file')).toHaveLength(1);
    await wrapper.find('input[type="text"]').setValue('整理一下');
    await wrapper.find('form').trigger('submit');
    await flushPromises();
    expect(quick.quickAdd).toHaveBeenLastCalledWith('整理一下', [file], expect.anything());
    expect(wrapper.findAll('.app-dock__file')).toHaveLength(0);
  });

  it('expand hands the typed text and files to the assign panel and clears the bar', async () => {
    const seen: unknown[] = [];
    const onOpen = (e: Event) => seen.push((e as CustomEvent).detail);
    window.addEventListener('aranea:open-assign', onOpen);
    const wrapper = setup();
    await openDock(wrapper);
    const file = new File(['x'], '数据.csv', { type: 'text/csv' });
    const picker = wrapper.find('input[type="file"]');
    Object.defineProperty(picker.element, 'files', { value: [file], configurable: true });
    await picker.trigger('change');
    await wrapper.find('input[type="text"]').setValue(' 核对账单 ');
    await wrapper.find('.app-dock__assign').trigger('click');
    window.removeEventListener('aranea:open-assign', onOpen);

    // 测试里的按钮桩会把点击转两次，只看第一次带过去的内容
    expect(seen[0]).toEqual({ text: '核对账单', files: [file] });
    expect(quick.quickAdd).not.toHaveBeenCalled();
    expect((wrapper.find('input[type="text"]').element as HTMLInputElement).value).toBe('');
    expect(wrapper.findAll('.app-dock__file')).toHaveLength(0);
  });

  it('floats over the page: the reserved bottom space stays the tab height when open', async () => {
    const wrapper = setup();
    await openDock(wrapper);
    expect(document.documentElement.style.getPropertyValue('--app-dock-space')).toBe('40px');
  });

  it('only collapses when asked; clicking away keeps it open', async () => {
    const wrapper = setup();
    await openDock(wrapper);
    await wrapper.find('.app-dock__panel').trigger('focusout', { relatedTarget: null });
    expect(wrapper.find('.app-dock__panel').exists()).toBe(true);

    await wrapper.find('input[type="text"]').trigger('keydown', { key: 'Escape' });
    await nextTick();
    await nextTick();
    expect(wrapper.find('.app-dock__panel').exists()).toBe(false);
    expect(document.activeElement).toBe(wrapper.find('.app-dock__tab').element);
  });

  it('remembers being open across remounts (page switches)', async () => {
    const first = setup();
    await openDock(first);
    first.unmount();
    wrappers.length = 0;
    const second = setup();
    await nextTick();
    expect(second.find('.app-dock__panel').exists()).toBe(true);
  });

  it('clears the reserved bottom space on unmount', () => {
    setup().unmount();
    wrappers.length = 0;
    expect(document.documentElement.style.getPropertyValue('--app-dock-space')).toBe('');
  });
});
