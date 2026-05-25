import { ref, type Ref } from "vue";
import { useQuasar } from "quasar";
import { useArtifactStore } from "../../../stores/artifact";
import { validateArtifactFileSize } from "../../artifact/limits";
import { readFileAsBase64 } from "../../artifact/fileBase64";
import type { ChatAttachment } from "../../../components/chat/types";

export function useChatAttachments(sessionId: Ref<string | undefined>) {
  const $q = useQuasar();
  const fileRef = ref<HTMLInputElement | null>(null);
  const attachments = ref<ChatAttachment[]>([]);

  function pickFile() {
    fileRef.value?.click();
  }

  function removeAttachment(id: string) {
    const target = attachments.value.find((item) => item.id === id);
    if (target?.timer) clearInterval(target.timer);
    attachments.value = attachments.value.filter((item) => item.id !== id);
  }

  async function onFileChange(event: Event) {
    const input = event.target as HTMLInputElement;
    if (!input.files?.length) return;

    const sid = sessionId.value ?? "";
    if (!sid) {
      $q.notify({ type: "warning", message: "请先创建或选择会话再上传附件" });
      input.value = "";
      return;
    }

    for (const file of Array.from(input.files)) {
      const sizeErr = validateArtifactFileSize(file.size);
      if (sizeErr) {
        $q.notify({ type: "warning", message: sizeErr });
        continue;
      }
      const tempId = `pending-${Date.now()}-${file.name}`;
      const record: ChatAttachment = { id: tempId, name: file.name, progress: 0.1 };
      attachments.value.push(record);
      try {
        const artifactStore = useArtifactStore();
        const meta = await artifactStore.upload({
          session_id: sid,
          name: file.name,
          mime_type: file.type || "application/octet-stream",
          data_base64: await readFileAsBase64(file),
        });
        record.id = meta.id;
        record.progress = 1;
      } catch (e) {
        attachments.value = attachments.value.filter((item) => item.id !== tempId);
        $q.notify({
          type: "negative",
          message: e instanceof Error ? e.message : "附件上传失败",
        });
      }
    }
    input.value = "";
  }

  return {
    fileRef,
    attachments,
    pickFile,
    onFileChange,
    removeAttachment,
  };
}
