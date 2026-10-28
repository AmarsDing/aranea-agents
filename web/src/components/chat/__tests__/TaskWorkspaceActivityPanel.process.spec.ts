import { describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import { defineComponent, h, type PropType } from 'vue';
import { createI18n } from 'vue-i18n';
import TaskWorkspaceActivityPanel from '../TaskWorkspaceActivityPanel.vue';
import type { TaskWorkspaceProcessView } from '../../../features/chat/types';
import type { Task, Turn, Step } from '../../../features/chat/v2Types';

const button = defineComponent({
  props: { label: { type: String, default: '' }, disable: Boolean },
  emits: ['click'],
  setup(props, { slots, emit }) {
    return () =>
      h('button', { disabled: props.disable, onClick: (e: Event) => emit('click', e) }, [
        props.label,
        slots.default?.(),
      ]);
  },
});
const select = defineComponent({
  props: {
    label: { type: String, default: '' },
    modelValue: { type: String, default: '' },
    options: { type: Array as PropType<{ value: string; label: string }[]>, default: () => [] },
  },
  emits: ['update:modelValue'],
  setup(props, { emit }) {
    return () =>
      h(
        'select',
        {
          'aria-label': props.label,
          value: props.modelValue,
          onChange: (e: Event) => emit('update:modelValue', (e.target as HTMLSelectElement).value),
        },
        (props.options as { value: string; label: string }[]).map((row) =>
          h('option', { value: row.value }, row.label),
        ),
      );
  },
});
const box = defineComponent({ template: '<div><slot/></div>' });
const task = { ID: 'T', SessionID: 'S', Seq: 1, Status: 'completed', UserMessage: 'Earlier work' } as Task;
const turn = { ID: 'turn', TaskID: 'T', SessionID: 'S', AgentKey: 'agent', Seq: 1, Status: 'completed' } as Turn;
const step = {
  ID: 'step',
  TaskID: 'T',
  TurnID: 'turn',
  SessionID: 'S',
  Kind: 'confirm',
  ToolName: 'write_file',
  Status: 'tool_blocked',
} as Step;
const view: TaskWorkspaceProcessView = {
  task,
  turn,
  turns: [turn],
  steps: [step],
  member: null,
  state: 'ready',
  manual: true,
  missingSelection: false,
  canLocateTurn: true,
};
function setup(process = view) {
  return mount(TaskWorkspaceActivityPanel, {
    props: { tasks: [task, { ...task, ID: 'T2', Seq: 2 }], pendingSteps: [], incomplete: false, process },
    global: {
      plugins: [
        createI18n({ legacy: false, locale: 'en', missingWarn: false, fallbackWarn: false, messages: { en: {} } }),
      ],
      stubs: {
        QBtn: button,
        QSelect: select,
        QList: box,
        QItem: box,
        QItemSection: box,
        QItemLabel: box,
        QIcon: true,
        QSpace: true,
      },
    },
  });
}
describe('process selection action boundaries', () => {
  it('emits reading selection without navigating or creating an approval action', async () => {
    const wrapper = setup();
    await wrapper.get('select[aria-label="taskWorkspace.chooseTask"]').setValue('T2');
    expect(wrapper.emitted('select-task')).toEqual([['T2']]);
    expect(wrapper.emitted('locate')).toBeUndefined();
    expect(
      wrapper
        .findAll('button')
        .map((b) => b.text())
        .join(' '),
    ).not.toMatch(/Approve|允许/);
    const locate = wrapper
      .findAll('button')
      .find((b) => b.attributes('aria-label') === 'taskWorkspace.openOriginalStep')!;
    await locate.trigger('click');
    expect(wrapper.emitted('locate')).toEqual([['T', 'step']]);
    wrapper.unmount();
  });
  it('keeps an unlocatable member turn read-only and opens only its actual member record', async () => {
    const wrapper = setup({
      ...view,
      canLocateTurn: false,
      member: { ID: 'member' } as NonNullable<TaskWorkspaceProcessView['member']>,
    });
    expect(wrapper.find('[aria-label="taskWorkspace.openOriginalStep"]').exists()).toBe(false);
    const open = wrapper.findAll('button').find((b) => b.text() === 'taskWorkspace.openMemberDetails')!;
    await open.trigger('click');
    expect(wrapper.emitted('open-member')).toEqual([['T', 'member']]);
    expect(wrapper.emitted('locate-turn')).toBeUndefined();
    wrapper.unmount();
  });
  it('shows loading failure and explicit retry without claiming there are no steps', async () => {
    const wrapper = setup({ ...view, state: 'error', steps: [] });
    expect(wrapper.get('[role="alert"]').text()).toContain('taskWorkspace.processLoadFailed');
    expect(wrapper.text()).not.toContain('taskWorkspace.noProcessSteps');
    await wrapper
      .findAll('button')
      .find((b) => b.text() === 'taskWorkspace.retryProcess')!
      .trigger('click');
    expect(wrapper.emitted('retry-process')).toHaveLength(1);
    wrapper.unmount();
  });
  it('keeps a missing saved task explicit when the current list is empty', async () => {
    const wrapper = setup({ ...view, task: null, turn: null, missingSelection: true });
    await wrapper.setProps({ tasks: [] });
    expect(wrapper.text()).toContain('taskWorkspace.processUnavailable');
    expect(wrapper.findAll('button').some((button) => button.text() === 'taskWorkspace.followCurrentTask')).toBe(true);
    wrapper.unmount();
  });
});
