import { describe, expect, it } from 'vitest';
import { shallowMount } from '@vue/test-utils';
import { createPinia } from 'pinia';
import { createI18n } from 'vue-i18n';
import { defineComponent } from 'vue';
import ChatMessagePanel from '../ChatMessagePanel.vue';

describe('composer visibility contract', () => {
  it('shows input by default, while preserving an explicit hide request', async () => {
    const wrapper = shallowMount(ChatMessagePanel, {
      props: {
        modelValue: '',
        messages: [],
        attachments: [],
        dialogMode: 'auto',
        modelProvider: '',
        modeOptions: [],
        providerOptions: [],
        sessionTitle: 'Test',
        contextRatio: 0,
        isDark: false,
      },
      global: {
        plugins: [
          createPinia(),
          createI18n({ legacy: false, locale: 'en', missingWarn: false, fallbackWarn: false, messages: { en: {} } }),
        ],
        stubs: {
          ChatMessageList: defineComponent({
            setup(_props, { expose }) {
              expose({ getScrollTarget: () => null, useVirtualScroll: false });
            },
            template: '<div />',
          }),
          QCard: { template: '<div><slot/></div>' },
          QCardSection: { template: '<div><slot/></div>' },
          QBanner: true,
          QIcon: true,
          QBtn: true,
          QTooltip: true,
          QBadge: true,
          QSpinnerDots: true,
        },
      },
    });
    expect(wrapper.findComponent({ name: 'ChatComposer' }).exists()).toBe(true);
    await wrapper.setProps({ composerVisible: false });
    expect(wrapper.findComponent({ name: 'ChatComposer' }).exists()).toBe(false);
    wrapper.unmount();
  });
});
