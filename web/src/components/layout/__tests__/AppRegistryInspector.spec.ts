import { afterEach, describe, expect, it, vi } from 'vitest';
import { mount, type VueWrapper } from '@vue/test-utils';
import { nextTick } from 'vue';
import { createI18n } from 'vue-i18n';
import AppRegistryInspector from '../AppRegistryInspector.vue';

const quasar = vi.hoisted(() => ({ screen: { width: 1440 } }));
vi.mock('quasar', () => ({ useQuasar: () => ({ screen: quasar.screen }) }));

const wrappers: VueWrapper[] = [];
const i18n = createI18n({
  legacy: false,
  locale: 'zh-CN',
  messages: { 'zh-CN': { shell: { common: { close: '关闭' } } } },
});

function setup(width = 1440) {
  quasar.screen.width = width;
  const trigger = document.createElement('button');
  document.body.append(trigger);
  trigger.focus();
  const wrapper = mount(AppRegistryInspector, {
    attachTo: document.body,
    props: { modelValue: true, title: 'Agent 检查器', eyebrow: 'AGENT / 01' },
    slots: { default: '<p>对象详情</p>' },
    global: {
      plugins: [i18n],
      stubs: { QBtn: { template: '<button type="button" @click="$emit(\'click\')"><slot /></button>' } },
    },
  });
  wrappers.push(wrapper);
  return { wrapper, trigger };
}

afterEach(() => {
  wrappers.splice(0).forEach((wrapper) => wrapper.unmount());
  document.body.innerHTML = '';
});

describe('AppRegistryInspector', () => {
  it('is a persistent complementary region on wide screens', async () => {
    const { wrapper } = setup();
    await nextTick();
    expect(wrapper.get('.app-registry-inspector').attributes('data-persistent')).toBe('true');
    expect(wrapper.get('[role="complementary"]').text()).toContain('对象详情');
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false);
  });

  it('becomes a modal drawer below the persistent breakpoint', () => {
    const { wrapper } = setup(1024);
    expect(wrapper.get('.app-registry-inspector').attributes('data-persistent')).toBe('false');
    expect(wrapper.get('[role="dialog"]').attributes('aria-modal')).toBe('true');
    expect(wrapper.find('.app-registry-inspector__scrim').exists()).toBe(true);
  });

  it('closes with Escape and restores focus to the opener', async () => {
    const { wrapper, trigger } = setup(1024);
    await nextTick();
    await wrapper.get('.app-registry-inspector__panel').trigger('keydown', { key: 'Escape' });
    expect(wrapper.emitted('update:modelValue')).toEqual([[false]]);
    await wrapper.setProps({ modelValue: false });
    await nextTick();
    expect(document.activeElement).toBe(trigger);
  });
});
