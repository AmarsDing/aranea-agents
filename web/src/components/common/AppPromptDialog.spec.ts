// AppPromptDialog — 表单 prompt / 单选选择器的渲染与校验契约。
import { describe, it, expect } from 'vitest';
import { mount } from '@vue/test-utils';
import { defineComponent } from 'vue';
import { createI18n } from 'vue-i18n';
import AppPromptDialog from './AppPromptDialog.vue';

const DialogStub = defineComponent({
  name: 'AppDialog',
  props: { modelValue: Boolean, title: String, size: String, persistent: Boolean, loading: Boolean },
  emits: ['update:modelValue'],
  template:
    '<div class="app-dialog-stub" :data-title="title"><slot /><div class="actions"><slot name="actions" /></div></div>',
});

const InputStub = defineComponent({
  name: 'QInput',
  props: { modelValue: String, type: String, error: Boolean, errorMessage: String, label: String },
  emits: ['update:modelValue'],
  template: '<input class="q-input-stub" :value="modelValue" :data-error="String(!!error)" />',
});

const BtnStub = defineComponent({
  name: 'QBtn',
  props: { label: String, disable: Boolean },
  emits: ['click'],
  template: '<button type="button" :disabled="disable" @click="$emit(\'click\')">{{ label }}</button>',
});

const i18n = createI18n({
  legacy: false,
  locale: 'zh-CN',
  messages: { 'zh-CN': { common: { confirm: '确认', cancel: '取消', required: '必填' } } },
});

const stubs = { AppDialog: DialogStub, 'q-input': InputStub, 'q-btn': BtnStub, 'q-icon': { template: '<i />' } };

function mountDialog(props: Record<string, unknown> = {}) {
  return mount(AppPromptDialog, {
    props: { modelValue: true, title: '新建文档', ...props },
    global: { plugins: [i18n], stubs },
  });
}

describe('AppPromptDialog', () => {
  it('渲染输入框并提交去空白值', async () => {
    const wrapper = mountDialog({ defaultValue: '  周报  ' });
    const input = wrapper.findComponent(InputStub);
    expect(input.props('modelValue')).toBe('  周报  ');

    await wrapper.findAll('button').at(-1)!.trigger('click');
    expect(wrapper.emitted('submit')?.[0]).toEqual(['周报']);
  });

  it('必填为空时点确认不提交，并就地标出错误', async () => {
    const wrapper = mountDialog({ defaultValue: '   ', required: true });
    const submit = wrapper.findAll('button').at(-1)!;

    await submit.trigger('click');
    expect(wrapper.emitted('submit')).toBeUndefined();
    expect(wrapper.findComponent(InputStub).props('error')).toBe(true);
  });

  it('radio 模式渲染可选项、禁用项不可选、必选项缺失时禁止提交', async () => {
    const wrapper = mountDialog({
      type: 'radio',
      items: [
        { label: '存为草稿', value: 'draft', disabled: true },
        { label: '保存并离开', value: 'save', hint: '会先保存所有脏页签' },
      ],
      defaultValue: '',
    });

    const radios = wrapper.findAll('input[type="radio"]');
    expect(radios).toHaveLength(2);
    expect(radios[0].attributes('disabled')).toBeDefined();
    expect(wrapper.text()).toContain('会先保存所有脏页签');
    expect(wrapper.findAll('button').at(-1)!.attributes('disabled')).toBeDefined();

    await radios[1].setValue(true);
    await wrapper.findAll('button').at(-1)!.trigger('click');
    expect(wrapper.emitted('submit')?.[0]).toEqual(['save']);
  });

  it('loading 时关闭请求被拦截，取消键发出关闭事件', async () => {
    const wrapper = mountDialog({ loading: true });
    const dialog = wrapper.findComponent(DialogStub);
    dialog.vm.$emit('update:modelValue', false);
    expect(wrapper.emitted('update:modelValue')).toBeUndefined();

    await wrapper.setProps({ loading: false });
    await wrapper.findAll('button').at(0)!.trigger('click');
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([false]);
  });
});
