import { describe, expect, it } from 'vitest';
import { defineComponent, h } from 'vue';
import { mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import zh from '../../../i18n/locales/zh-CN';
import WorkflowPanel from '../WorkflowPanel.vue';
import { advance, createWorkflow, decide } from '../../../features/ecommerce-loop/workflow';

const slotStub = defineComponent({
  setup:
    (_, { slots }) =>
    () =>
      h('div', slots.default?.()),
});
const buttonStub = defineComponent({
  props: { label: { type: String, default: '' }, disable: Boolean },
  emits: ['click'],
  setup:
    (props, { emit }) =>
    () =>
      h('button', { disabled: props.disable, onClick: () => emit('click') }, props.label),
});
const inputStub = defineComponent({
  props: { modelValue: { type: String, default: '' } },
  emits: ['update:modelValue'],
  setup:
    (props, { emit }) =>
    () =>
      h('textarea', {
        value: props.modelValue,
        onInput: (event: Event) => emit('update:modelValue', (event.target as HTMLTextAreaElement).value),
      }),
});
function panel(editing = false) {
  const workflow = createWorkflow('launch');
  if (!editing) {
    decide(workflow, 'approved', 1);
    advance(workflow);
    advance(workflow);
  }
  return mount(WorkflowPanel, {
    props: { workflow, editing, shopName: '店铺 A', embedded: true },
    global: {
      plugins: [createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zh } })],
      stubs: {
        QCard: slotStub,
        QCardSection: slotStub,
        QCardActions: slotStub,
        QSeparator: true,
        QIcon: true,
        QBtn: buttonStub,
        QInput: inputStub,
        QCheckbox: defineComponent({
          props: { modelValue: Boolean },
          emits: ['update:modelValue'],
          setup:
            (props, { emit }) =>
            () =>
              h('input', {
                type: 'checkbox',
                checked: props.modelValue,
                onChange: (e: Event) => emit('update:modelValue', (e.target as HTMLInputElement).checked),
              }),
        }),
      },
    },
  });
}

describe('workflow review panel', () => {
  it('requires a new publish confirmation after the content version changes', async () => {
    const wrapper = panel();
    const publishButton = () => wrapper.findAll('button').find((button) => button.text().includes('确认演示发布'))!;
    expect(publishButton().attributes('disabled')).toBeDefined();
    await wrapper.get('input[type="checkbox"]').setValue(true);
    expect(publishButton().attributes('disabled')).toBeUndefined();
    await wrapper.setProps({ workflow: { ...wrapper.props('workflow'), version: 2 } });
    expect(publishButton().attributes('disabled')).toBeDefined();
    wrapper.unmount();
  });

  it('does not reuse a publish confirmation for another store', async () => {
    const wrapper = panel();
    await wrapper.get('input[type="checkbox"]').setValue(true);
    await wrapper.setProps({ shopName: '店铺 C' });
    expect((wrapper.get('input[type="checkbox"]').element as HTMLInputElement).checked).toBe(false);
    wrapper.unmount();
  });

  it('reports dirty input and emits the reviewed version with revision content', async () => {
    const wrapper = panel(true);
    await wrapper.get('textarea').setValue('保留商品香型，调整标题');
    expect(wrapper.emitted('dirty')?.at(-1)).toEqual([true]);
    await wrapper
      .findAll('button')
      .find((button) => button.text().includes('保存新版'))!
      .trigger('click');
    expect(wrapper.emitted('action')?.at(-1)).toEqual([
      { action: 'revise', version: 1, note: '保留商品香型，调整标题' },
    ]);
    wrapper.unmount();
  });
});
