import { ref, toValue, watch, type MaybeRefOrGetter } from 'vue';
import { validateModel } from '../platform/api';

/** Debounced validateModel for agent provider/model pairs (catalog + runtime). */
export function useAgentModelValidation(provider: MaybeRefOrGetter<string>, model: MaybeRefOrGetter<string>) {
  const checking = ref(false);
  const ok = ref<boolean | null>(null);
  const message = ref('');
  let timer: ReturnType<typeof setTimeout> | undefined;

  async function runValidate(): Promise<{ ok: boolean; message: string }> {
    const p = String(toValue(provider) ?? '').trim();
    const m = String(toValue(model) ?? '').trim();
    if (!p || !m) {
      ok.value = null;
      message.value = '';
      return { ok: false, message: '' };
    }
    checking.value = true;
    try {
      const result = await validateModel(p, m);
      ok.value = result.ok;
      message.value = result.message || (result.ok ? '模型可用' : '模型不可用');
      return { ok: result.ok, message: message.value };
    } catch (e) {
      ok.value = false;
      message.value = e instanceof Error ? e.message : '模型校验失败';
      return { ok: false, message: message.value };
    } finally {
      checking.value = false;
    }
  }

  function scheduleValidate() {
    ok.value = null;
    message.value = '';
    if (timer) clearTimeout(timer);
    timer = setTimeout(() => void runValidate(), 450);
  }

  watch(
    () => [String(toValue(provider) ?? '').trim(), String(toValue(model) ?? '').trim()] as const,
    ([p, m], prev) => {
      if (p === prev?.[0] && m === prev?.[1]) return;
      scheduleValidate();
    },
    { immediate: true },
  );

  return { checking, ok, message, runValidate, scheduleValidate };
}
