import { ref, type Ref } from "vue";
import { useQuasar } from "quasar";
import { uploadArtifact } from "../../artifact/api";
import type { ChatAttachment } from "../../../components/chat/types";

async function readFileAsBase64(file: File): Promise<string> {
  const buf = await file.arrayBuffer();
  const bytes = new Uint8Array(buf);
  const chunks: string[] = [];
  const chunkSize = 8192;
  for (let i = 0; i < bytes.length; i += chunkSize) {
    const slice = bytes.subarray(i, i + chunkSize);
    chunks.push(String.fromCharCode(...slice));
  }
  return btoa(chunks.join(""));
}

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
      const tempId = `pending-${Date.now()}-${file.name}`;
      const record: ChatAttachment = { id: tempId, name: file.name, progress: 0.1 };
      attachments.value.push(record);
      try {
        const meta = await uploadArtifact({
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
