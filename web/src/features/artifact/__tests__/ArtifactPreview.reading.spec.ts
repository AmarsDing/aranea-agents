import { afterEach, describe, expect, it, vi } from 'vitest';
import { mount, type VueWrapper } from '@vue/test-utils';
import { computed, defineComponent, ref } from 'vue';
import { createI18n } from 'vue-i18n';
import ArtifactPreview from '../ArtifactPreview.vue';
import type { ArtifactPreview as Preview } from '../types';

const preview = ref<Preview | null>(null);
vi.mock('../useArtifactPreview', () => ({
  useArtifactPreview: () => ({
    preview,
    loading: ref(false),
    error: ref(''),
    mediaError: ref(''),
    kindIcon: computed(() => 'description'),
    formatBytes: () => '1 KB',
    loadPreview: vi.fn(),
  }),
}));
const button = defineComponent({
  props: { label: { type: String, default: '' }, disable: Boolean },
  template: '<button :disabled="disable">{{label}}<slot /></button>',
});
const wrappers: VueWrapper[] = [];
afterEach(() => {
  for (const wrapper of wrappers.splice(0)) wrapper.unmount();
  vi.restoreAllMocks();
});
function setup() {
  preview.value = {
    meta: { id: 'file', name: 'plan.md', version: 0, session_id: 'A' },
    preview_kind: 'text',
    text_content: '# Delivery\n\nReadable **plan**.\n\n<script>alert(1)</script>\n\n[unsafe](javascript:alert(1))',
  } as Preview;
  const wrapper = mount(ArtifactPreview, {
    props: { artifactId: 'file', allowQuote: true },
    global: {
      plugins: [
        createI18n({
          legacy: false,
          locale: 'en',
          missingWarn: false,
          fallbackWarn: false,
          messages: {
            en: {
              taskWorkspace: {
                viewSource: 'View source',
                readDocument: 'Read document',
                quoteSelection: 'Quote selection',
              },
            },
          },
        }),
      ],
      stubs: { QBtn: button, QIcon: true, QTooltip: true, QSpinner: true, QSpace: true },
    },
  });
  wrappers.push(wrapper);
  return wrapper;
}
describe('document reading and keyboard selection', () => {
  it('renders safe Markdown and allows inspection of the original source', async () => {
    const wrapper = setup();
    expect(wrapper.find('h1').text()).toBe('Delivery');
    expect(wrapper.find('script').exists()).toBe(false);
    expect(wrapper.find('a[href^="javascript:"]').exists()).toBe(false);
    await wrapper
      .findAll('button')
      .find((b) => b.text().includes('View source'))!
      .trigger('click');
    expect(wrapper.find('pre').text()).toContain('# Delivery');
  });
  it('quotes selected visible text with version zero and invalidates it on a new preview', async () => {
    const wrapper = setup();
    const heading = wrapper.find('h1').element;
    vi.spyOn(window, 'getSelection').mockReturnValue({
      rangeCount: 1,
      anchorNode: heading.firstChild,
      focusNode: heading.firstChild,
      toString: () => 'Delivery',
    } as unknown as Selection);
    await wrapper.find('[tabindex="0"]').trigger('keyup', { key: 'ArrowRight', shiftKey: true });
    const quote = wrapper.findAll('button').find((b) => b.text().includes('Quote selection'))!;
    await quote.trigger('click');
    expect(wrapper.emitted('quote')?.[0]?.[0]).toMatchObject({ text: 'Delivery', meta: { version: 0, id: 'file' } });
    preview.value = { ...preview.value!, meta: { ...preview.value!.meta, id: 'new-file', version: 1 } };
    await wrapper.vm.$nextTick();
    expect(quote.attributes('disabled')).toBeDefined();
  });
});
