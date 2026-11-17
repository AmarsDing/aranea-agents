// useAppConfirm / AppConfirmHost — 命令式确认框契约：
// Promise 挂起到用户操作、确认 true、取消/Esc false、连续调用串行不串台。
import { describe, it, expect, beforeEach } from 'vitest';
import { flushPromises, mount } from '@vue/test-utils';
import { defineComponent, nextTick } from 'vue';
import AppConfirmHost from '../../components/common/AppConfirmHost.vue';
import { confirm, state } from '../useAppConfirm';

const DialogStub = defineComponent({
  name: 'AppConfirmDialog',
  props: {
    modelValue: { type: Boolean, default: false },
    title: { type: String, default: '' },
    message: { type: String, default: '' },
    tone: { type: String, default: '' },
    confirmLabel: { type: String, default: '' },
    cancelLabel: { type: String, default: '' },
    icon: { type: String, default: '' },
    loading: { type: Boolean, default: false },
  },
  emits: ['update:modelValue', 'confirm'],
  template: '<div class="app-confirm-dialog-stub" :data-visible="String(modelValue)" />',
});

function mountHost() {
  const wrapper = mount(AppConfirmHost, { global: { stubs: { AppConfirmDialog: DialogStub } } });
  return { wrapper, dialog: wrapper.findComponent(DialogStub) };
}

/** 记录 Promise 是否已 settle 及其结果，用于断言"挂起中"。 */
function track(promise: Promise<boolean>) {
  const box: { settled: boolean; value: boolean | null } = { settled: false, value: null };
  void promise.then((value) => {
    box.settled = true;
    box.value = value;
  });
  return box;
}

beforeEach(() => {
  state.visible = false;
  state.loading = false;
  state.title = '';
  state.message = '';
  state.tone = 'danger';
  state.confirmLabel = '';
  state.cancelLabel = '';
  state.icon = '';
});

describe('useAppConfirm', () => {
  it('confirm() 挂起直到用户确认，确认返回 true 并收起', async () => {
    const { dialog } = mountHost();
    const box = track(confirm({ title: '删除 Agent', message: '确定删除？', tone: 'danger', confirmLabel: '删除' }));

    await nextTick();
    await flushPromises();
    expect(box.settled).toBe(false);
    expect(state.visible).toBe(true);
    expect(state.title).toBe('删除 Agent');
    expect(state.message).toBe('确定删除？');
    expect(state.tone).toBe('danger');
    expect(state.confirmLabel).toBe('删除');
    expect(dialog.props('modelValue')).toBe(true);

    dialog.vm.$emit('confirm');
    await flushPromises();
    expect(box.settled).toBe(true);
    expect(box.value).toBe(true);
    expect(state.visible).toBe(false);
  });

  it('取消键返回 false', async () => {
    const { dialog } = mountHost();
    const box = track(confirm({ title: '删除', message: '取消测试' }));
    await flushPromises();
    expect(box.settled).toBe(false);

    dialog.vm.$emit('update:modelValue', false);
    await flushPromises();
    expect(box.value).toBe(false);
    expect(state.visible).toBe(false);
  });

  it('Esc / 任意关闭（无确认）同样返回 false，Promise 不会悬挂', async () => {
    const { dialog } = mountHost();
    const box = track(confirm({ title: '离开页面', message: '未保存的更改将丢失' }));
    await flushPromises();

    // AppDialog 的 Esc 会走 update:modelValue=false，与取消键同一条路径。
    dialog.vm.$emit('update:modelValue', false);
    await flushPromises();
    expect(box.settled).toBe(true);
    expect(box.value).toBe(false);
  });

  it('连续调用串行：第二次排队，各自拿到自己的结果', async () => {
    const { dialog } = mountHost();
    const first = track(confirm({ title: '第一次' }));
    const second = track(confirm({ title: '第二次' }));
    await flushPromises();

    expect(state.title).toBe('第一次');
    expect(second.settled).toBe(false);

    dialog.vm.$emit('confirm');
    await flushPromises();
    expect(first.value).toBe(true);
    // 第二次此刻才打开，且未继承第一次的结果
    expect(state.title).toBe('第二次');
    expect(state.visible).toBe(true);
    expect(second.settled).toBe(false);

    dialog.vm.$emit('update:modelValue', false);
    await flushPromises();
    expect(second.value).toBe(false);
    expect(first.value).toBe(true);
    expect(state.visible).toBe(false);
  });
});
