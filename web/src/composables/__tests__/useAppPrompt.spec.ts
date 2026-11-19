// useAppPrompt / AppPromptHost — 命令式输入框契约：
// Promise 挂起到用户提交、提交返回去空白值、取消/Esc 返回 null、必填与串行行为。
import { describe, it, expect, beforeEach } from 'vitest';
import { flushPromises, mount } from '@vue/test-utils';
import { defineComponent, nextTick } from 'vue';
import AppPromptHost from '../../components/common/AppPromptHost.vue';
import { prompt, promptState } from '../useAppPrompt';

const DialogStub = defineComponent({
  name: 'AppPromptDialog',
  props: {
    modelValue: { type: Boolean, default: false },
    title: { type: String, default: '' },
    message: { type: String, default: '' },
    label: { type: String, default: '' },
    placeholder: { type: String, default: '' },
    type: { type: String, default: 'text' },
    items: { type: Array, default: () => [] },
    defaultValue: { type: String, default: '' },
    required: { type: Boolean, default: false },
    confirmLabel: { type: String, default: '' },
    cancelLabel: { type: String, default: '' },
    tone: { type: String, default: '' },
    persistent: { type: Boolean, default: true },
    loading: { type: Boolean, default: false },
  },
  emits: ['update:modelValue', 'submit'],
  template: '<div class="app-prompt-dialog-stub" :data-visible="String(modelValue)" />',
});

function mountHost() {
  const wrapper = mount(AppPromptHost, { global: { stubs: { AppPromptDialog: DialogStub } } });
  return { dialog: wrapper.findComponent(DialogStub) };
}

/** 记录 Promise 是否已 settle 及其结果，用于断言「挂起中」。 */
function track(promise: Promise<string | null>) {
  const box: { settled: boolean; value: string | null | undefined } = { settled: false, value: undefined };
  void promise.then((value) => {
    box.settled = true;
    box.value = value;
  });
  return box;
}

beforeEach(() => {
  promptState.visible = false;
  promptState.title = '';
  promptState.items = [];
});

describe('useAppPrompt', () => {
  it('prompt() 挂起直到提交，提交返回值并收起', async () => {
    const { dialog } = mountHost();
    const box = track(prompt({ title: '新建目录', label: '目录名', defaultValue: 'docs' }));

    await nextTick();
    await flushPromises();
    expect(box.settled).toBe(false);
    expect(promptState.visible).toBe(true);
    expect(promptState.title).toBe('新建目录');
    expect(promptState.defaultValue).toBe('docs');
    expect(dialog.props('modelValue')).toBe(true);

    dialog.vm.$emit('submit', 'guides');
    await flushPromises();
    expect(box.settled).toBe(true);
    expect(box.value).toBe('guides');
    expect(promptState.visible).toBe(false);
  });

  it('text 提交值去首尾空白，radio 原样返回', async () => {
    const { dialog } = mountHost();
    const text = track(prompt({ title: '重命名' }));
    await flushPromises();
    dialog.vm.$emit('submit', '  新标题  ');
    await flushPromises();
    expect(text.value).toBe('新标题');

    const radio = track(prompt({ title: '选择落点', type: 'radio', items: [{ label: '当前', value: 'current' }] }));
    await flushPromises();
    expect(promptState.defaultValue).toBe('current');
    dialog.vm.$emit('submit', 'current');
    await flushPromises();
    expect(radio.value).toBe('current');
  });

  it('取消 / Esc 返回 null，Promise 不悬挂', async () => {
    const { dialog } = mountHost();
    const box = track(prompt({ title: '离开页面' }));
    await flushPromises();

    dialog.vm.$emit('update:modelValue', false);
    await flushPromises();
    expect(box.settled).toBe(true);
    expect(box.value).toBeNull();
  });

  it('radio 默认选中第一个可选项；全部禁用时默认值为空', async () => {
    const { dialog } = mountHost();
    const draft = track(
      prompt({
        title: '未保存',
        type: 'radio',
        items: [
          { label: '存草稿', value: 'draft', disabled: true },
          { label: '保存', value: 'save' },
        ],
      }),
    );
    await flushPromises();
    expect(dialog.props('defaultValue')).toBe('save');
    expect(dialog.props('required')).toBe(true);
    dialog.vm.$emit('update:modelValue', false);
    await flushPromises();
    expect(draft.value).toBeNull();

    const blocked = track(
      prompt({ title: '全禁用', type: 'radio', items: [{ label: '存草稿', value: 'draft', disabled: true }] }),
    );
    await flushPromises();
    expect(promptState.defaultValue).toBe('');
    dialog.vm.$emit('update:modelValue', false);
    await flushPromises();
    expect(blocked.value).toBeNull();
  });

  it('连续调用串行：第二次排队，各自拿到自己的结果', async () => {
    const { dialog } = mountHost();
    const first = track(prompt({ title: '第一次' }));
    const second = track(prompt({ title: '第二次' }));
    await flushPromises();

    expect(promptState.title).toBe('第一次');
    expect(second.settled).toBe(false);

    dialog.vm.$emit('submit', 'a');
    await flushPromises();
    expect(first.value).toBe('a');
    expect(promptState.title).toBe('第二次');
    expect(second.settled).toBe(false);

    dialog.vm.$emit('update:modelValue', false);
    await flushPromises();
    expect(second.value).toBeNull();
    expect(first.value).toBe('a');
  });
});
