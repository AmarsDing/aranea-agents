import { afterEach, describe, expect, it } from 'vitest';
import { mount, type VueWrapper } from '@vue/test-utils';
import { defineComponent } from 'vue';
import { createI18n } from 'vue-i18n';
import ChatComposer from '../ChatComposer.vue';

const mounted: VueWrapper[] = [];
const button = defineComponent({
  props: { disable: Boolean },
  template: '<button :disabled="disable"><slot /></button>',
});
const input = defineComponent({
  props: { modelValue: { type: String, default: '' } },
  template: '<textarea :value="modelValue" />',
});
const box = { template: '<div><slot /><slot name="action" /></div>' };
afterEach(() => {
  for (const wrapper of mounted.splice(0)) wrapper.unmount();
});
function setup(overrides: Record<string, unknown> = {}) {
  const wrapper = mount(ChatComposer, {
    props: {
      modelValue: 'continue',
      attachments: [],
      dialogMode: 'auto',
      modelProvider: 'model',
      modeOptions: [],
      providerOptions: [{ label: 'Model A', value: 'model' }],
      contextRatio: 0,
      isDark: false,
      workspaceMode: true,
      ...overrides,
    },
    global: {
      plugins: [
        createI18n({
          legacy: false,
          locale: 'en',
          missingWarn: false,
          fallbackWarn: false,
          messages: {
            en: {
              chat: { send: 'Send', stop: 'Stop' },
              taskWorkspace: {
                followUp: 'Add instruction',
                inputName: 'Message current session',
                removeAttachment: 'Remove attachment: {name}',
              },
            },
          },
        }),
      ],
      stubs: {
        ChatSkillPicker: true,
        ChatEnqueueMessage: true,
        QInput: input,
        QBtn: button,
        QCardSection: box,
        QBanner: box,
        QIcon: box,
        QTooltip: box,
        QChip: box,
        QCircularProgress: box,
        QSelect: box,
        QItem: box,
        QItemLabel: box,
        QItemSection: box,
      },
    },
  });
  mounted.push(wrapper);
  return wrapper;
}

describe('task workspace composer', () => {
  it('names the input and exposes an explicitly named attachment removal action', async () => {
    const wrapper = setup({ attachments: [{ id: 'file', name: 'brief.txt', progress: 1 }] });
    expect(wrapper.find('textarea').attributes('aria-label')).toBe('Message current session');
    await wrapper.find('button[aria-label="Remove attachment: brief.txt"]').trigger('click');
    expect(wrapper.emitted('remove-attachment')).toEqual([['file']]);
  });
  it('shows skills, attachments, model and send in compact mode', () => {
    const wrapper = setup({
      attachments: [{ id: 'file', name: 'brief.txt', progress: 1 }],
      selectedSkillSlugs: ['research'],
    });
    expect(wrapper.find('.composer-card--compact').exists()).toBe(true);
    expect(wrapper.text()).toContain('brief.txt');
    expect(wrapper.text()).toContain('research');
    expect(wrapper.text()).toContain('Model A');
    expect(wrapper.find('button[aria-label="Send"]').exists()).toBe(true);
  });

  it('uses the normal send action for Enter even while running, preserving skill processing', async () => {
    const wrapper = setup({ isRunnerActive: true, selectedSkillSlugs: ['research'] });
    await wrapper.find('textarea').trigger('keydown', { key: 'Enter' });
    expect(wrapper.emitted('send')).toHaveLength(1);
    expect(wrapper.emitted('enqueue-message')).toBeUndefined();
    expect(wrapper.find('button[aria-label="Stop"]').exists()).toBe(true);
  });

  it('does not send during IME composition or Shift+Enter', async () => {
    const wrapper = setup();
    await wrapper.find('textarea').trigger('keydown', { key: 'Enter', isComposing: true });
    await wrapper.find('textarea').trigger('keydown', { key: 'Enter', shiftKey: true });
    expect(wrapper.emitted('send')).toBeUndefined();
  });

  it('blocks a running file follow-up and a pending upload for keyboard and button', async () => {
    const wrapper = setup({ isRunnerActive: true, attachments: [{ id: 'file', name: 'brief.txt', progress: 1 }] });
    expect(wrapper.find('button[aria-label="Add instruction"]').attributes('disabled')).toBeDefined();
    await wrapper.find('textarea').trigger('keydown', { key: 'Enter' });
    expect(wrapper.emitted('send')).toBeUndefined();
    await wrapper.setProps({
      isRunnerActive: false,
      attachments: [{ id: 'pending', name: 'upload.txt', progress: 0.1 }],
    });
    expect(wrapper.find('button[aria-label="Send"]').attributes('disabled')).toBeDefined();
    await wrapper.find('textarea').trigger('keydown', { key: 'Enter' });
    expect(wrapper.emitted('send')).toBeUndefined();
  });
});
