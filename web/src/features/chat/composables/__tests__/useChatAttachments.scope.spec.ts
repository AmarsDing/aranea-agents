import { beforeEach, describe, expect, it, vi } from 'vitest';
import { defineComponent, ref } from 'vue';
import { flushPromises, mount } from '@vue/test-utils';
import { useChatAttachments } from '../useChatAttachments';
import type { ArtifactMeta } from '../../../artifact/types';

const upload = vi.hoisted(() => vi.fn());
vi.mock('../../../../stores/artifact', () => ({ useArtifactStore: () => ({ upload }) }));
vi.mock('quasar', () => ({ useQuasar: () => ({ notify: vi.fn() }) }));
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }));
vi.mock('../../../artifact/fileBase64', () => ({ readFileAsBase64: async () => 'aGVsbG8=' }));

beforeEach(() => vi.resetAllMocks());
function setup() {
  const session = ref<string | undefined>('A');
  let attachments!: ReturnType<typeof useChatAttachments>;
  const wrapper = mount(
    defineComponent({
      setup() {
        attachments = useChatAttachments(session);
        return () => null;
      },
    }),
  );
  return { session, attachments, wrapper };
}
function file() {
  return new File(['hello'], 'a.txt', { type: 'text/plain' });
}

describe('attachment draft ownership', () => {
  it('keeps a late upload in the originating session', async () => {
    let resolve!: (meta: Partial<ArtifactMeta>) => void;
    upload.mockReturnValueOnce(
      new Promise((yes) => {
        resolve = yes;
      }),
    );
    const { session, attachments, wrapper } = setup();
    const pending = attachments.uploadFile(file());
    await flushPromises();
    session.value = 'B';
    resolve({ id: 'uploaded-A' });
    await pending;
    expect(attachments.attachments.value).toEqual([]);
    session.value = 'A';
    expect(attachments.attachments.value[0]?.id).toBe('uploaded-A');
    expect(attachments.attachments.value[0]?.progress).toBe(1);
    wrapper.unmount();
  });

  it('does not reattach a file removed before its upload finishes', async () => {
    let resolve!: (meta: Partial<ArtifactMeta>) => void;
    upload.mockReturnValueOnce(
      new Promise((yes) => {
        resolve = yes;
      }),
    );
    const { attachments, wrapper } = setup();
    const pending = attachments.uploadFile(file());
    await flushPromises();
    attachments.removeAttachment(attachments.attachments.value[0]!.id);
    resolve({ id: 'removed-file' });
    await pending;
    expect(attachments.attachments.value).toEqual([]);
    wrapper.unmount();
  });
});
