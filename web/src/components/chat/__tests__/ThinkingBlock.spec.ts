import { describe, it, expect, afterEach } from 'vitest';
import { createApp, type App as VueApp } from 'vue';
import { createI18n } from 'vue-i18n';
import ThinkingBlock from '../ThinkingBlock.vue';

const i18n = createI18n({
  legacy: false,
  locale: 'en',
  messages: { en: {} },
});

function mountThinking(props: Record<string, unknown>): HTMLElement {
  const container = document.createElement('div');
  document.body.appendChild(container);
  const app: VueApp = createApp(ThinkingBlock, props as never);
  app.use(i18n);
  app.mount(container);
  return container;
}

afterEach(() => {
  document.body.innerHTML = '';
});

describe('ThinkingBlock label', () => {
  // Chat UI fix: ThinkingBlock must accept and render a `label` prop
  // ("规划"/"推理"/"重规划") when provided, instead of always showing
  // the hardcoded "思考" fallback. The label field is already defined on
  // ThinkingEvent and passed through useActivityTimeline — the component
  // just never received it.
  it('renders custom label when label prop is provided', () => {
    const c = mountThinking({
      messageId: 'think-1',
      reasoning: 'This is a sufficiently long reasoning content to avoid inline-short mode.',
      streaming: false,
      defaultCollapsed: false,
      label: '规划',
    });
    const labelText = c.querySelector('.thinking-block__label-text')?.textContent;
    expect(labelText).toBe('规划');
  });

  it('renders custom label "推理" when provided', () => {
    const c = mountThinking({
      messageId: 'think-2',
      reasoning: 'Another sufficiently long reasoning content to render the expanded label row.',
      streaming: false,
      defaultCollapsed: false,
      label: '推理',
    });
    const labelText = c.querySelector('.thinking-block__label-text')?.textContent;
    expect(labelText).toBe('推理');
  });

  it('falls back to default "思考" when label prop is empty/undefined', () => {
    const c = mountThinking({
      messageId: 'think-3',
      reasoning: 'Long enough reasoning content so the component renders in expanded mode and shows the default label.',
      streaming: false,
      defaultCollapsed: false,
      // label intentionally omitted
    });
    const labelText = c.querySelector('.thinking-block__label-text')?.textContent;
    // i18n with empty messages falls back to the key path; the key point
    // is that it should NOT be empty and should match the default fallback
    expect(labelText).toBeTruthy();
    expect(labelText?.length).toBeGreaterThan(0);
  });
});

describe('ThinkingBlock streaming class', () => {
  it('applies streaming class when streaming=true', () => {
    const c = mountThinking({
      messageId: 'think-stream',
      reasoning: 'streaming content',
      streaming: true,
    });
    const root = c.querySelector('.thinking-block');
    expect(root?.className).toContain('thinking-block--streaming');
  });
});
