import { defineStore } from "pinia";
import { ref } from "vue";
import {
  sendMessage,
  listChatOptions,
  stopGeneration,
  type ChatOption
} from "../../features/chat/api";
import type { Message, SendMessageOptions } from "../../features/chat/types";

export const useChatStore = defineStore("chat", () => {
  const messages = ref<Message[]>([]);
  const chatOptions = ref<ChatOption[]>([]);
  const sending = ref(false);
  const activeSessionId = ref<string | null>(null);

  async function loadChatOptions(type?: string) {
    chatOptions.value = await listChatOptions(type);
  }

  async function send(sessionId: string, content: string, opts?: SendMessageOptions) {
    sending.value = true;
    try {
      return await sendMessage({ session_id: sessionId, content, options: opts });
    } finally {
      sending.value = false;
    }
  }

  async function stop(sessionId: string) {
    return stopGeneration(sessionId);
  }

  function appendMessage(msg: Message) {
    messages.value.push(msg);
  }

  function clearMessages() {
    messages.value = [];
  }

  function setActiveSession(id: string | null) {
    activeSessionId.value = id;
    if (!id) clearMessages();
  }

  return { messages, chatOptions, sending, activeSessionId, loadChatOptions, send, stop, appendMessage, clearMessages, setActiveSession };
});
