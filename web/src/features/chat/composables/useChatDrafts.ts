import { reactive, watch, type Ref } from 'vue';
import type { ChatDraftRecord } from '../chatDraftPersistence';

type Draft = ChatDraftRecord;

/** A send receipt consumes only its captured revision, including after route changes. */
export function useChatDrafts(
  input: Ref<string>,
  entityKey: () => string,
  sessionId: () => string | undefined,
  persistence?: { records: Map<string, Draft>; persist: () => void },
) {
  const records = persistence?.records ?? reactive(new Map<string, Draft>());
  const persist = () => persistence?.persist();
  let active: Draft | undefined;
  let restoring = false;
  const key = (entity: string, sid?: string) => JSON.stringify([entity, sid ?? null]);

  watch(
    input,
    (text) => {
      if (restoring || !active || text === active.text) return;
      active.text = text;
      active.revision++;
      active.updatedAt = Date.now();
      persist();
    },
    { flush: 'sync' },
  );

  watch(
    () => [entityKey(), sessionId()] as const,
    ([entity, sid]) => {
      const draftKey = key(entity, sid);
      if (!records.has(draftKey)) records.set(draftKey, { text: '', revision: 0, updatedAt: Date.now() });
      active = records.get(draftKey)!;
      restoring = true;
      input.value = active.text;
      restoring = false;
    },
    { immediate: true, flush: 'sync' },
  );
  // A receipt from an unmounted page may consume the record currently restored elsewhere.
  watch(
    () => records.get(key(entityKey(), sessionId()))?.text,
    () => {
      active = records.get(key(entityKey(), sessionId()));
      if (input.value === (active?.text ?? '')) return;
      restoring = true;
      input.value = active?.text ?? '';
      restoring = false;
    },
    { flush: 'sync' },
  );

  function capture(): () => void {
    const origin = active;
    const revision = origin?.revision;
    return () => {
      if (!origin || origin.revision !== revision) return;
      origin.text = '';
      origin.revision++;
      origin.updatedAt = Date.now();
      persist();
      if (active === origin) {
        restoring = true;
        input.value = '';
        restoring = false;
      }
    };
  }

  async function createSession(create: () => Promise<string | undefined>): Promise<string | undefined> {
    const entity = entityKey();
    const origin = active;
    const originKey = key(entity, sessionId());
    const sid = await create();
    if (!records.has(originKey)) return sid;
    if (sid && origin) {
      const target = key(entity, sid);
      const existing = records.get(target);
      if (!existing || (existing.revision === 0 && !existing.text)) {
        records.set(target, origin);
        if (originKey !== target) records.delete(originKey);
        persist();
        if (entityKey() === entity && sessionId() === sid) {
          active = origin;
          restoring = true;
          input.value = origin.text;
          restoring = false;
        }
      }
    }
    return sid;
  }

  return { capture, createSession };
}
