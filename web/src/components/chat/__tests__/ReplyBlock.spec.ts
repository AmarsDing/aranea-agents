import { describe, it, expect, afterEach } from 'vitest';
import { createApp, type App as VueApp } from 'vue';
import { createI18n } from 'vue-i18n';
import ReplyBlock from '../ReplyBlock.vue';
import type { ReplyEvent } from '../../../features/chat/streamEventTypes';

const i18n = createI18n({
  legacy: false,
  locale: 'en',
  messages: { en: {} },
});

/** Build a minimal ReplyEvent with sensible defaults. */
function makeReply(overrides: Partial<ReplyEvent> & { id?: string } = {}): ReplyEvent {
  return {
    id: overrides.id ?? 'reply-1',
    kind: 'reply',
    content: '',
    streaming: false,
    isFinal: false,
    ...overrides,
  } as ReplyEvent;
}

function mountReply(activity: ReplyEvent): HTMLElement {
  const container = document.createElement('div');
  document.body.appendChild(container);
  const app: VueApp = createApp(ReplyBlock, { activity });
  app.use(i18n);
  app.mount(container);
  return container;
}

afterEach(() => {
  document.body.innerHTML = '';
});

describe('ReplyBlock streaming state', () => {
  it('applies reply-block--streaming class on root div when streaming', () => {
    const c = mountReply(makeReply({ streaming: true }));
    const root = c.querySelector('.reply-block');
    expect(root).toBeTruthy();
    expect(root?.className).toContain('reply-block--streaming');
  });

  it('does NOT apply streaming class when not streaming', () => {
    const c = mountReply(makeReply({ streaming: false }));
    const root = c.querySelector('.reply-block');
    expect(root?.className).not.toContain('reply-block--streaming');
  });
});

describe('ReplyBlock label', () => {
  it('shows intermediate reply label when isFinal=false', () => {
    const c = mountReply(makeReply({ isFinal: false }));
    const labelText = c.querySelector('.reply-block__label-text')?.textContent;
    expect(labelText).toBeTruthy();
    // i18n key fallback: when translation missing, falls back to key path
    // The key point: it should NOT be empty
    expect(labelText?.length).toBeGreaterThan(0);
  });

  it('shows final reply label when isFinal=true', () => {
    const c = mountReply(makeReply({ isFinal: true }));
    const labelText = c.querySelector('.reply-block__label-text')?.textContent;
    expect(labelText).toBeTruthy();
    expect(labelText?.length).toBeGreaterThan(0);
  });
});
