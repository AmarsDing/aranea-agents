import { describe, expect, it, vi } from 'vitest';
import { mount } from '@vue/test-utils';
import CapCard from '../CapCard.vue';

const global = { mocks: { t: (k: string) => k }, stubs: { 'q-icon': true, 'q-checkbox': true } };

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (k: string) => k }) }));

describe('CapCard', () => {
  it('shows the checkbox only when checked is passed', () => {
    const plain = mount(CapCard, { props: { icon: 'build', code: 'k', title: 'T' }, global });
    expect(plain.find('q-checkbox-stub').exists()).toBe(false);
    const batch = mount(CapCard, { props: { icon: 'build', code: 'k', title: 'T', checked: false }, global });
    expect(batch.find('q-checkbox-stub').exists()).toBe(true);
  });

  it('opens on click and Enter, falls back to a placeholder description', async () => {
    const w = mount(CapCard, { props: { icon: 'build', code: 'k', title: 'T' }, global });
    expect(w.find('.cap-card__desc--empty').text()).toBe('cap.common.noDescription');
    await w.trigger('click');
    await w.trigger('keydown', { key: 'Enter' });
    expect(w.emitted('open')).toHaveLength(2);
  });
});
