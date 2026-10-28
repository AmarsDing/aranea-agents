import { computed, onScopeDispose, reactive, ref, watch, type Ref } from 'vue';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import { useArtifactStore } from '../../../stores/artifact';
import { validateArtifactFileSize } from '../../artifact/limits';
import { readFileAsBase64 } from '../../artifact/fileBase64';
import type { ChatAttachment } from '../../../components/chat/types';

export function useChatAttachments(sessionId: Ref<string | undefined>) {
  const $q = useQuasar();
  const { t } = useI18n();
  const fileRef = ref<HTMLInputElement | null>(null);
  const attachments = ref<ChatAttachment[]>([]);
  const drafts = reactive(new Map<string, ChatAttachment[]>());
  const hasAttachments = computed(
    () => attachments.value.length > 0 || [...drafts.values()].some((rows) => rows.length > 0),
  );
  function beforeUnload(event: BeforeUnloadEvent) {
    if (!hasAttachments.value) return;
    event.preventDefault();
    event.returnValue = '';
  }
  if (typeof window !== 'undefined') {
    window.addEventListener('beforeunload', beforeUnload);
    onScopeDispose(() => window.removeEventListener('beforeunload', beforeUnload));
  }
  watch(
    sessionId,
    (sid, previous) => {
      if (previous) drafts.set(previous, attachments.value);
      attachments.value = sid ? (drafts.get(sid) ?? []) : [];
    },
    { flush: 'sync' },
  );

  function pickFile() {
    fileRef.value?.click();
  }

  function removeAttachment(id: string) {
    const target = attachments.value.find((item) => item.id === id);
    if (target?.timer) clearInterval(target.timer);
    const index = attachments.value.findIndex((item) => item.id === id);
    if (index >= 0) attachments.value.splice(index, 1);
  }

  async function onFileChange(event: Event) {
    const input = event.target as HTMLInputElement;
    if (!input.files?.length) return;

    const files = Array.from(input.files);
    const sid = sessionId.value ?? '';
    input.value = '';
    for (const file of files) await uploadFile(file, sid);
  }

  async function uploadFile(file: File, sid = sessionId.value ?? '') {
    if (!sid) {
      $q.notify({ type: 'warning', message: t('taskWorkspace.selectSessionFirst') });
      return;
    }

    const sizeErr = validateArtifactFileSize(file.size);
    if (sizeErr) {
      $q.notify({ type: 'warning', message: sizeErr });
      return;
    }

    const tempId = `pending-${Date.now()}-${file.name}`;
    const rows = sid === sessionId.value ? attachments.value : (drafts.get(sid) ?? []);
    drafts.set(sid, rows);
    rows.push({ id: tempId, name: file.name, mime_type: file.type, progress: 0.1 });
    const record = rows[rows.length - 1]!;
    try {
      const artifactStore = useArtifactStore();
      const meta = await artifactStore.upload({
        session_id: sid,
        name: file.name,
        mime_type: file.type || 'application/octet-stream',
        data_base64: await readFileAsBase64(file),
      });
      if (rows.includes(record)) {
        record.id = meta.id;
        record.progress = 1;
      }
    } catch (e) {
      const index = rows.indexOf(record);
      if (index >= 0) rows.splice(index, 1);
      $q.notify({
        type: 'negative',
        message: e instanceof Error ? e.message : t('taskWorkspace.uploadFailed'),
      });
    }
  }

  return {
    fileRef,
    attachments,
    hasAttachments,
    pickFile,
    onFileChange,
    uploadFile,
    removeAttachment,
  };
}
