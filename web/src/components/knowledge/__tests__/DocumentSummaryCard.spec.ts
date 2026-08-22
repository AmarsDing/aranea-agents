import { describe, expect, it, vi } from 'vitest';
import { mount } from '@vue/test-utils';
import DocumentSummaryCard from '../DocumentSummaryCard.vue';

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (k: string) => k }),
}));

describe('DocumentSummaryCard', () => {
  it('hides when empty', () => {
    const w = mount(DocumentSummaryCard, { props: {} });
    expect(w.find('[data-test="doc-summary-card"]').exists()).toBe(false);
  });

  it('renders type, summary and tags', () => {
    const w = mount(DocumentSummaryCard, {
      props: { summary: '退款三个工作日', docType: 'faq', tags: ['refund'] },
    });
    expect(w.text()).toContain('faq');
    expect(w.text()).toContain('退款三个工作日');
    expect(w.text()).toContain('refund');
  });
});
