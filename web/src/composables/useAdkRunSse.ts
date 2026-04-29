import { ref, shallowRef } from "vue";
import { runSse, type AdkLlmResponse, type AdkRunSseRequest } from "../api/adk";
import { useI18n } from "vue-i18n";
import { Notify } from "quasar";

/**
 * 聊天页等：封装 run_sse 状态机，单页可多次发起，自动取消上一轮。
 */
export function useAdkRunSse() {
  const { t } = useI18n();
  const loading = ref(false);
  const lastError = shallowRef<unknown | null>(null);
  const events = shallowRef<AdkLlmResponse[]>([]);
  let current: ReturnType<typeof runSse> | null = null;

  function cancel() {
    current?.cancel();
    current = null;
  }

  function send(
    requestBody: AdkRunSseRequest,
    options?: { appendEvents?: boolean; onEvent?: (e: AdkLlmResponse) => void; silent?: boolean }
  ) {
    cancel();
    if (!options?.appendEvents) {
      events.value = [];
    }
    lastError.value = null;
    loading.value = true;

    current = runSse(
      { ...requestBody, streaming: requestBody.streaming ?? true },
      {
        onEvent: (e) => {
          events.value = [...events.value, e];
          options?.onEvent?.(e);
        },
        onError: (e) => {
          lastError.value = e;
          if (!options?.silent) {
            const msg = e instanceof Error ? e.message : String(e);
            void Notify.create({ type: "negative", message: t("adk.sseError", { msg }) });
          }
        },
        onComplete: () => {
          loading.value = false;
          current = null;
        }
      }
    );

    return current;
  }

  return { loading, lastError, events, send, cancel };
}
