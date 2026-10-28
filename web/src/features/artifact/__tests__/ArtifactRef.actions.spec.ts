import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { mount, shallowMount, flushPromises } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import ArtifactRef from '../ArtifactRef.vue';
import ArtifactRefCard from '../../../components/artifact/ArtifactRefCard.vue';
import { useArtifactStore } from '../../../stores/artifact';

const { notify } = vi.hoisted(() => ({ notify: vi.fn() }));
vi.mock('quasar', () => ({ useQuasar: () => ({ notify }) }));
const i18n = () =>
  createI18n({
    legacy: false,
    locale: 'en-US',
    messages: {
      'en-US': {
        artifact: { refCard: { defaultName: 'Artifact' }, preview: { download: 'Download' } },
        uiSystem: { operations: { downloadFailed: 'Download failed. Please retry.' } },
      },
    },
  });

beforeEach(() => {
  setActivePinia(createPinia());
  notify.mockClear();
});

describe('artifact reference actions', () => {
  it('keeps preview and download as separate keyboard-focusable controls', async () => {
    const wrapper = mount(ArtifactRefCard, {
      props: { artifactId: 'A', name: 'report.pdf' },
      global: {
        plugins: [i18n()],
        stubs: {
          QBtn: { template: '<button type="button"><slot /></button>' },
          QIcon: true,
          QTooltip: true,
        },
      },
    });
    expect(wrapper.findAll('button')).toHaveLength(2);
    expect(wrapper.find('[role="button"] button').exists()).toBe(false);
    await wrapper.get('.artifact-ref-card__preview').trigger('click');
    expect(wrapper.emitted('preview')).toEqual([['A']]);
    expect(wrapper.emitted('download')).toBeUndefined();
    await wrapper.get('button[aria-label="Download"]').trigger('click');
    expect(wrapper.emitted('download')).toEqual([['A']]);
    wrapper.unmount();
  });

  it('opens the original artifact target through the container', () => {
    const store = useArtifactStore();
    const open = vi.spyOn(store, 'openArtifactPreview');
    const wrapper = shallowMount(ArtifactRef, { props: { artifactId: 'A' }, global: { plugins: [i18n()] } });
    wrapper.getComponent(ArtifactRefCard).vm.$emit('preview', 'A');
    expect(open).toHaveBeenCalledWith('A');
    wrapper.unmount();
  });

  it('locks duplicate downloads and shows a recoverable failure', async () => {
    let fail!: (reason: Error) => void;
    const store = useArtifactStore();
    const download = vi.spyOn(store, 'download').mockImplementation(
      () =>
        new Promise((_, reject) => {
          fail = reject;
        }),
    );
    const wrapper = shallowMount(ArtifactRef, { props: { artifactId: 'A' }, global: { plugins: [i18n()] } });
    const card = wrapper.getComponent(ArtifactRefCard);
    card.vm.$emit('download', 'A');
    card.vm.$emit('download', 'A');
    expect(download).toHaveBeenCalledTimes(1);
    fail(new Error('offline'));
    await flushPromises();
    expect(card.props('downloading')).toBe(false);
    expect(notify).toHaveBeenCalledWith({ type: 'negative', message: 'Download failed. Please retry.' });
    download.mockResolvedValueOnce();
    card.vm.$emit('download', 'A');
    await flushPromises();
    expect(download).toHaveBeenCalledTimes(2);
    wrapper.unmount();
  });
});
