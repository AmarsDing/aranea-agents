import { afterEach, describe, expect, it } from 'vitest';
import { defineComponent, nextTick } from 'vue';
import { mount, type VueWrapper } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import AppDialog from './AppDialog.vue';

const DialogStub = defineComponent({
  name: 'QDialog',
  props: { modelValue: Boolean, persistent: Boolean, noRefocus: Boolean, position: String, maximized: Boolean },
  emits: ['update:modelValue', 'escape-key', 'hide'],
  template: '<div v-if="modelValue" class="q-dialog-stub"><slot /></div>',
});

const stubs = {
  'q-dialog': DialogStub,
  'q-card': { template: '<section class="q-card-stub"><slot /></section>' },
  'q-card-section': { template: '<div class="q-card-section-stub"><slot /></div>' },
  'q-card-actions': { template: '<div class="q-card-actions-stub"><slot /></div>' },
  'q-btn': {
    props: ['label', 'disable', 'icon'],
    emits: ['click'],
    template:
      '<button type="button" :disabled="disable" v-bind="$attrs" @click="$emit(\'click\')">{{ label || icon }}</button>',
  },
  'q-icon': { template: '<i />' },
};

const i18n = createI18n({
  legacy: false,
  locale: 'zh-CN',
  messages: { 'zh-CN': { shell: { common: { close: '关闭' } } } },
});

const wrappers: VueWrapper[] = [];
function dialog(props: Record<string, unknown> = {}) {
  const wrapper = mount(AppDialog, {
    attachTo: document.body,
    props: { modelValue: true, title: '对象检查器', ...props },
    slots: { default: '<p>正文</p>', actions: '<button type="button">执行</button>' },
    global: { plugins: [i18n], stubs },
  });
  wrappers.push(wrapper);
  return wrapper;
}

afterEach(() => {
  for (const wrapper of wrappers.splice(0)) wrapper.unmount();
  document.body.innerHTML = '';
});

describe('AppDialog', () => {
  it.each(['sm', 'md', 'lg', 'xl'])('提供 %s 尺寸与固定头正文尾结构', (size) => {
    const wrapper = dialog({ size });

    expect(wrapper.find(`.app-dialog--${size}`).exists()).toBe(true);
    expect(wrapper.find('.app-dialog__title').text()).toBe('对象检查器');
    expect(wrapper.find('.app-dialog__body').text()).toContain('正文');
    expect(wrapper.find('.app-dialog__actions').text()).toContain('执行');
  });

  it('默认 md 尺寸从右侧滑出成检视面板', () => {
    const wrapper = dialog();

    expect(wrapper.getComponent(DialogStub).props('position')).toBe('right');
    expect(wrapper.find('.app-dialog--drawer-right').exists()).toBe(true);
  });

  it.each(['sm', 'xl'])('默认 %s 尺寸保持居中', (size) => {
    const wrapper = dialog({ size });

    expect(wrapper.getComponent(DialogStub).props('position')).toBeUndefined();
    expect(wrapper.find('.app-dialog--drawer').exists()).toBe(false);
  });

  it('显式 standard 不向 QDialog 传 position', () => {
    const wrapper = dialog({ position: 'standard' });

    expect(wrapper.getComponent(DialogStub).props('position')).toBeUndefined();
    expect(wrapper.find('.app-dialog--drawer').exists()).toBe(false);
  });

  it.each(['right', 'left'])('position=%s 时透传定位并切到抽屉卡片形态', (position) => {
    const wrapper = dialog({ position });

    expect(wrapper.getComponent(DialogStub).props('position')).toBe(position);
    expect(wrapper.find('.app-dialog--drawer').exists()).toBe(true);
    expect(wrapper.find(`.app-dialog--drawer-${position}`).exists()).toBe(true);
  });

  it('maximized 透传全屏并追加全屏卡片类，Esc 语义保持不变', async () => {
    const wrapper = dialog({ maximized: true });

    expect(wrapper.getComponent(DialogStub).props('maximized')).toBe(true);
    expect(wrapper.find('.app-dialog--maximized').exists()).toBe(true);

    wrapper.getComponent(DialogStub).vm.$emit('escape-key');
    await nextTick();
    expect(wrapper.emitted('update:modelValue')).toEqual([[false]]);
  });

  it('cardClass 追加到卡片并保留基础类', () => {
    const wrapper = dialog({ cardClass: 'channel-editor-dialog' });

    const card = wrapper.get('.app-dialog');
    expect(card.classes()).toContain('channel-editor-dialog');
    expect(card.classes()).toContain('app-dialog--md');
  });

  it('Esc 请求关闭并同步 v-model', async () => {
    const wrapper = dialog();

    wrapper.getComponent(DialogStub).vm.$emit('escape-key');
    await nextTick();

    expect(wrapper.emitted('update:modelValue')).toEqual([[false]]);
  });

  it('loading 时禁止 Esc 和关闭按钮', async () => {
    const wrapper = dialog({ loading: true });

    wrapper.getComponent(DialogStub).vm.$emit('escape-key');
    await wrapper.get('[data-test="dialog-close"]').trigger('click');

    expect(wrapper.emitted('update:modelValue')).toBeUndefined();
    expect(wrapper.get('[data-test="dialog-close"]').attributes('disabled')).toBeDefined();
    expect(wrapper.getComponent(DialogStub).props('persistent')).toBe(true);
  });

  it('persistent 时 Esc 不关闭，但标题栏关闭按钮仍可关闭', async () => {
    const wrapper = dialog({ persistent: true });

    wrapper.getComponent(DialogStub).vm.$emit('escape-key');
    await nextTick();
    expect(wrapper.emitted('update:modelValue')).toBeUndefined();

    await wrapper.get('[data-test="dialog-close"]').trigger('click');
    expect(wrapper.emitted('update:modelValue')).toEqual([[false]]);
  });

  it('header-prefix 槽渲染在标题之前', () => {
    const wrapper = mount(AppDialog, {
      props: { modelValue: true, title: '新建对象' },
      slots: { 'header-prefix': '<span data-test="prefix">头像</span>' },
      global: { plugins: [i18n], stubs },
    });
    wrappers.push(wrapper);

    const header = wrapper.get('.app-dialog__header-main');
    expect(header.get('[data-test="prefix"]').exists()).toBe(true);
    expect(header.html().indexOf('data-test="prefix"')).toBeLessThan(header.html().indexOf('app-dialog__title'));
  });

  it('关闭后把焦点还给打开前控件', async () => {
    const trigger = document.createElement('button');
    trigger.textContent = '打开';
    document.body.append(trigger);
    trigger.focus();

    const wrapper = dialog();
    await wrapper.setProps({ modelValue: false });
    await nextTick();

    expect(wrapper.getComponent(DialogStub).props('noRefocus')).toBe(true);
    expect(document.activeElement).toBe(trigger);
  });

  it('QBtn 的 focus-helper 获得焦点时仍返回触发按钮本身', async () => {
    const trigger = document.createElement('button');
    trigger.className = 'q-btn';
    const focusHelper = document.createElement('span');
    focusHelper.className = 'q-focus-helper';
    focusHelper.tabIndex = -1;
    trigger.append(focusHelper);
    document.body.append(trigger);
    focusHelper.focus();

    const wrapper = dialog();
    await wrapper.setProps({ modelValue: false });
    await nextTick();
    wrapper.getComponent(DialogStub).vm.$emit('hide');
    await nextTick();

    expect(document.activeElement).toBe(trigger);
  });

  it('Quasar 完成隐藏后纠正 focus-helper 并再次回焦', async () => {
    const trigger = document.createElement('button');
    document.body.append(trigger);
    trigger.focus();

    const wrapper = dialog();
    await wrapper.setProps({ modelValue: false });
    await nextTick();

    const focusHelper = document.createElement('span');
    focusHelper.className = 'q-focus-helper';
    focusHelper.tabIndex = -1;
    document.body.append(focusHelper);
    focusHelper.focus();
    wrapper.getComponent(DialogStub).vm.$emit('hide');
    await nextTick();

    expect(document.activeElement).toBe(trigger);
  });
});
