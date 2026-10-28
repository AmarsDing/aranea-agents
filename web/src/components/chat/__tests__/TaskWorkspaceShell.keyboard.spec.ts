import { afterEach, describe, expect, it, vi } from 'vitest';
import { mount, type VueWrapper } from '@vue/test-utils';
import { defineComponent, nextTick } from 'vue';
import { createI18n } from 'vue-i18n';
import TaskWorkspaceShell from '../TaskWorkspaceShell.vue';

vi.mock('quasar', () => ({ useQuasar: () => ({ dark: { isActive: false } }) }));
const shell = defineComponent({ template: '<section><slot name="toolbar"/><slot/></section>' });
const wrappers: VueWrapper[] = [];
afterEach(() => wrappers.splice(0).forEach((wrapper) => wrapper.unmount()));
function setup() {
  const wrapper = mount(TaskWorkspaceShell, {
    attachTo: document.body,
    props: { pane: 'chat', contextOpen: true },
    slots: {
      default:
        '<div class="task-conversation"><div role="region" tabindex="0"/><textarea/></div><aside class="task-context" tabindex="-1" />',
    },
    global: {
      plugins: [
        createI18n({ legacy: false, locale: 'en', missingWarn: false, fallbackWarn: false, messages: { en: {} } }),
      ],
      stubs: {
        ChatWorkspaceShell: shell,
        QResizeObserver: true,
        QIcon: true,
        QSpace: true,
        QBtn: true,
        QBtnDropdown: true,
        QList: true,
        QItem: true,
        QItemSection: true,
      },
    },
  });
  wrappers.push(wrapper);
  return wrapper;
}
describe('workspace keyboard navigation', () => {
  it('focuses the input when returning to chat and the context when opening artifacts', async () => {
    const wrapper = setup();
    await wrapper.trigger('keydown', { key: '1', altKey: true });
    await nextTick();
    expect(wrapper.emitted('show-chat')).toHaveLength(1);
    expect(document.activeElement).toBe(wrapper.find('textarea').element);
    await wrapper.trigger('keydown', { key: '3', altKey: true });
    await nextTick();
    expect(wrapper.emitted('show-artifacts')).toHaveLength(1);
    expect(document.activeElement).toBe(wrapper.find('aside').element);
  });
  it('keeps IME composition and textarea Escape under the input owner', async () => {
    const wrapper = setup();
    await wrapper.trigger('keydown', { key: '4', altKey: true, isComposing: true });
    await wrapper.find('textarea').trigger('keydown', { key: 'Escape' });
    expect(wrapper.emitted('show-activity')).toBeUndefined();
    expect(wrapper.emitted('close-context')).toBeUndefined();
    await wrapper.find('aside').trigger('keydown', { key: 'Escape' });
    expect(wrapper.emitted('close-context')).toHaveLength(1);
  });
});
