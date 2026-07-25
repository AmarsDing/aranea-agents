// web/src/components/chat/v2/__tests__/ClarifyBlock.spec.ts
// 设计：docs/development/1-chat.design.md §B.10.18.6（前端测试策略）
import { describe, it, expect, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { mount } from '@vue/test-utils';
import ClarifyBlock from '../ClarifyBlock.vue';
import TaskCard from '../TaskCard.vue';
import { useChatActivityStore } from '../../../../stores/chat/activityV2Store';
import type { Step, Task, ClarificationEnvelope } from '../../../../features/chat/v2Types';
import type { SubmitClarificationPayload } from '../../../../features/chat/types';

function mkStep(overrides: Partial<Step>): Step {
  return {
    ID: 'step-clarify-1',
    TurnID: '',
    TaskID: 'tk1',
    SessionID: 'sess-1',
    SpiritSessionID: 'sess-1',
    Kind: 'clarify',
    AuthorAgentKey: 'a1',
    Seq: 1,
    Version: 1,
    Content: '',
    Reasoning: '',
    ToolName: '',
    ToolCallID: '',
    ToolArgs: null,
    ToolResult: null,
    ToolDurationMs: 0,
    ToolErrorCode: '',
    Status: 'awaiting_input',
    IsFinal: false,
    StartedAt: '',
    CompletedAt: null,
    ...overrides,
  } as Step;
}

function envelopeContent(env: Partial<ClarificationEnvelope>): string {
  return JSON.stringify({ version: 1, kind: 'clarification', questions: [], answers: null, ...env });
}

const twoQuestions: ClarificationEnvelope['questions'] = [
  { question: '目标平台？', mode: 'single', options: ['Web', 'iOS'], recommended: ['Web'] },
  { question: '受众？', mode: 'multi', options: ['开发者', '设计师'], recommended: ['开发者'] },
];

describe('ClarifyBlock', () => {
  beforeEach(() => setActivePinia(createPinia()));

  it('renders first question with options and recommended chip', () => {
    const wrapper = mount(ClarifyBlock, {
      props: { step: mkStep({ Content: envelopeContent({ questions: twoQuestions }) }) },
    });
    expect(wrapper.text()).toContain('目标平台？');
    expect(wrapper.text()).toContain('Web');
    expect(wrapper.text()).toContain('iOS');
    // 推荐 chip（i18n 缺失时回退为 key）
    expect(wrapper.find('.clarify-block__recommended').exists()).toBe(true);
    // 页码 1/2
    expect(wrapper.find('.clarify-block__page').text()).toBe('1/2');
    // 第一页有「下一页」，无「完成」
    const buttons = wrapper.findAll('.clarify-block__btn');
    expect(buttons.some((b) => b.classes().includes('clarify-block__btn--finish'))).toBe(false);
  });

  it('paginates: next → last page shows finish; prev returns', async () => {
    const wrapper = mount(ClarifyBlock, {
      props: { step: mkStep({ Content: envelopeContent({ questions: twoQuestions }) }) },
    });
    const next = wrapper.findAll('.clarify-block__btn').find((b) => b.classes().includes('clarify-block__btn--next'));
    expect(next).toBeDefined();
    await next!.trigger('click');
    expect(wrapper.find('.clarify-block__page').text()).toBe('2/2');
    expect(wrapper.text()).toContain('受众？');
    // 最后一页：有「完成」，无「下一页」
    const buttons = wrapper.findAll('.clarify-block__btn');
    expect(buttons.some((b) => b.classes().includes('clarify-block__btn--finish'))).toBe(true);
    expect(buttons.some((b) => b.classes().includes('clarify-block__btn--next'))).toBe(false);
    // 上一页返回
    const prev = wrapper
      .findAll('.clarify-block__btn')
      .find((b) => !b.classes().includes('clarify-block__btn--finish'));
    await prev!.trigger('click');
    expect(wrapper.find('.clarify-block__page').text()).toBe('1/2');
  });

  it('single mode: select replaces; re-click deselects (back to empty)', async () => {
    const wrapper = mount(ClarifyBlock, {
      props: { step: mkStep({ Content: envelopeContent({ questions: twoQuestions }) }) },
    });
    const options = wrapper.findAll('.clarify-block__option');
    await options[0].trigger('click'); // 选 Web
    expect(wrapper.findAll('.clarify-block__option--selected')).toHaveLength(1);
    await options[1].trigger('click'); // 改选 iOS
    const selected = wrapper.findAll('.clarify-block__option--selected');
    expect(selected).toHaveLength(1);
    expect(selected[0].text()).toContain('iOS');
    await options[1].trigger('click'); // 再点取消
    expect(wrapper.findAll('.clarify-block__option--selected')).toHaveLength(0);
  });

  it('multi mode: toggles multiple options independently', async () => {
    const wrapper = mount(ClarifyBlock, {
      props: { step: mkStep({ Content: envelopeContent({ questions: twoQuestions }) }) },
    });
    await wrapper.find('.clarify-block__btn--next').trigger('click');
    const options = wrapper.findAll('.clarify-block__option');
    await options[0].trigger('click');
    await options[1].trigger('click');
    expect(wrapper.findAll('.clarify-block__option--selected')).toHaveLength(2);
    await options[0].trigger('click');
    expect(wrapper.findAll('.clarify-block__option--selected')).toHaveLength(1);
  });

  it('finish emits submit-clarification with selections + other; empty pages stay empty', async () => {
    const wrapper = mount(ClarifyBlock, {
      props: { step: mkStep({ Content: envelopeContent({ questions: twoQuestions }) }) },
    });
    // 第 1 页：选 Web
    await wrapper.findAll('.clarify-block__option')[0].trigger('click');
    // 第 2 页：不选，填 other
    await wrapper.find('.clarify-block__btn--next').trigger('click');
    await wrapper.find('.clarify-block__other').setValue('内部工具');
    await wrapper.find('.clarify-block__btn--finish').trigger('click');

    const emitted = wrapper.emitted('submit-clarification');
    expect(emitted).toBeTruthy();
    const payload = emitted![0][0] as SubmitClarificationPayload;
    expect(payload.sessionId).toBe('sess-1');
    expect(payload.stepId).toBe('step-clarify-1');
    expect(payload.answers).toEqual([
      { selected: ['Web'], other: '' },
      { selected: [], other: '内部工具' },
    ]);
  });

  it('renders read-only summary when completed: explicit / asRecommended / noPreference', () => {
    const questions: ClarificationEnvelope['questions'] = [
      { question: 'Q1', mode: 'single', options: ['a', 'b'], recommended: ['a'] },
      { question: 'Q2', mode: 'single', options: ['c'], recommended: ['c'] },
      { question: 'Q3', mode: 'multi', options: ['d'], recommended: [] },
    ];
    const step = mkStep({
      Status: 'completed',
      Content: envelopeContent({
        questions,
        answers: [
          { selected: ['b'], other: '' },
          { selected: null, other: '' },
          { selected: [], other: '' },
        ],
      }),
    });
    const wrapper = mount(ClarifyBlock, { props: { step } });
    const rows = wrapper.findAll('.clarify-block__qa');
    expect(rows).toHaveLength(3);
    expect(rows[0].find('.clarify-block__a').text()).toBe('b'); // 显式作答
    // Q2 留空 → 按推荐（i18n 缺失时回退 key，检查包含推荐值即可）
    expect(rows[1].find('.clarify-block__a').text()).toContain('c');
    expect(rows[2].find('.clarify-block__a').text()).toContain('noPreference');
    // 无交互按钮
    expect(wrapper.find('.clarify-block__btn').exists()).toBe(false);
  });

  it('renders nothing on malformed content (fail-open)', () => {
    const wrapper = mount(ClarifyBlock, {
      props: { step: mkStep({ Content: '{not-json' }) },
    });
    expect(wrapper.find('.clarify-block').exists()).toBe(false);
  });
});

describe('TaskCard clarify registration', () => {
  beforeEach(() => setActivePinia(createPinia()));

  it('renders ClarifyBlock for orphan clarify step (TurnID empty)', () => {
    const store = useChatActivityStore();
    store.upsertStep(mkStep({ Content: envelopeContent({ questions: twoQuestions }) }));
    const task: Task = {
      ID: 'tk1',
      SessionID: 'sess-1',
      UserMessage: 'hi',
      Status: 'running',
      Seq: 1,
      Version: 1,
      CreatedAt: '',
      UpdatedAt: '',
      CompletedAt: null,
    };
    const wrapper = mount(TaskCard, { props: { task } });
    expect(wrapper.findComponent(ClarifyBlock).exists()).toBe(true);
  });

  it('does not render ClarifyBlock for clarify step bound to a turn', () => {
    const store = useChatActivityStore();
    store.upsertStep(
      mkStep({ ID: 'step-bound', TurnID: 'turn-1', Content: envelopeContent({ questions: twoQuestions }) }),
    );
    const task: Task = {
      ID: 'tk1',
      SessionID: 'sess-1',
      UserMessage: 'hi',
      Status: 'running',
      Seq: 1,
      Version: 1,
      CreatedAt: '',
      UpdatedAt: '',
      CompletedAt: null,
    };
    const wrapper = mount(TaskCard, { props: { task } });
    expect(wrapper.findComponent(ClarifyBlock).exists()).toBe(false);
  });
});
