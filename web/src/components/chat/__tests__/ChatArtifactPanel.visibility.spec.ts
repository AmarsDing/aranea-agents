import { describe, expect, it } from 'vitest';
import { defineComponent, onMounted, onUnmounted } from 'vue';
import { mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import ChatArtifactPanel from '../ChatArtifactPanel.vue';

describe('preview visibility lifecycle', () => {
  it('unmounts hidden media while retaining the selected artifact for reopening', async () => {
    let mounted = 0;
    let disposed = 0;
    const preview = defineComponent({
      setup() {
        onMounted(() => mounted++);
        onUnmounted(() => disposed++);
        return () => null;
      },
    });
    const box = { template: '<div><slot /></div>' };
    const wrapper = mount(ChatArtifactPanel, {
      props: {
        artifacts: [],
        total: 0,
        loading: false,
        loadingMore: false,
        hasMore: false,
        error: '',
        target: { id: 'video-A', surface: 'panel', sessionId: 'A' },
        sessionId: 'A',
        previewActive: true,
        versions: [],
        versionsLoading: false,
        versionsError: '',
      },
      global: {
        plugins: [
          createI18n({ legacy: false, locale: 'en', messages: { en: {} }, missingWarn: false, fallbackWarn: false }),
        ],
        stubs: {
          ArtifactPreview: preview,
          QIcon: box,
          QSpace: box,
          QBtn: box,
          QBtnDropdown: box,
          QList: box,
          QItem: box,
          QItemSection: box,
          QItemLabel: box,
          QSpinnerDots: box,
        },
        directives: { closePopup: {} },
      },
    });
    expect(mounted).toBe(1);
    await wrapper.setProps({ previewActive: false });
    expect(disposed).toBe(1);
    expect(wrapper.props('target')?.id).toBe('video-A');
    await wrapper.setProps({ previewActive: true });
    expect(mounted).toBe(2);
    wrapper.unmount();
    expect(disposed).toBe(2);
  });
});
