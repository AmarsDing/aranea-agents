import { nextTick, ref, watch, type Ref } from 'vue';
import type { AgentRuntimeConfigForm } from './agentRuntimeConfig';

export type SkillRuntimeForm = AgentRuntimeConfigForm['skillRuntime'];

export function parseSkillRuntimeForm(raw?: string): SkillRuntimeForm {
  const out: SkillRuntimeForm = {
    intent_routing_enabled: true,
    intent_max_paths: 3,
    max_skills_in_toolset: 32,
    allowed_slugs: [],
    denied_slugs: [],
    allowed_tags: [],
  };
  try {
    const o = JSON.parse(String(raw ?? '{}').trim() || '{}');
    if (typeof o.intent_routing_enabled === 'boolean') out.intent_routing_enabled = o.intent_routing_enabled;
    if (typeof o.intent_max_paths === 'number' && Number.isFinite(o.intent_max_paths)) {
      const n = Math.floor(o.intent_max_paths);
      if (n >= 1 && n <= 32) out.intent_max_paths = n;
    }
    if (typeof o.max_skills_in_toolset === 'number' && Number.isFinite(o.max_skills_in_toolset)) {
      const n = Math.floor(o.max_skills_in_toolset);
      if (n >= 1 && n <= 256) out.max_skills_in_toolset = n;
    }
    const strList = (v: unknown) => (Array.isArray(v) ? v.map((x) => String(x).trim()).filter(Boolean) : []);
    out.allowed_slugs = strList(o.allowed_slugs);
    out.denied_slugs = strList(o.denied_slugs);
    out.allowed_tags = strList(o.allowed_tags);
  } catch {
    /* keep defaults */
  }
  return out;
}

function normSkillSlug(s: string): string {
  return String(s ?? '')
    .trim()
    .toLowerCase();
}

function reconcileSkillSlugListsDenyWins(rt: SkillRuntimeForm) {
  const denySet = new Set(rt.denied_slugs.map(normSkillSlug).filter(Boolean));
  rt.allowed_slugs = rt.allowed_slugs.filter((a) => !denySet.has(normSkillSlug(a)));
  const allowSet = new Set(rt.allowed_slugs.map(normSkillSlug).filter(Boolean));
  rt.denied_slugs = rt.denied_slugs.filter((d) => !allowSet.has(normSkillSlug(d)));
}

export function normalizeSkillRuntimeState(rt: SkillRuntimeForm) {
  rt.intent_max_paths = Math.min(32, Math.max(1, Math.floor(Number(rt.intent_max_paths) || 3)));
  rt.max_skills_in_toolset = Math.min(256, Math.max(1, Math.floor(Number(rt.max_skills_in_toolset) || 32)));
  for (const key of ['allowed_slugs', 'denied_slugs', 'allowed_tags'] as const) {
    if (!Array.isArray(rt[key])) rt[key] = [];
    rt[key] = rt[key].map((x) => String(x).trim()).filter(Boolean);
  }
  reconcileSkillSlugListsDenyWins(rt);
}

export function stringifySkillRuntimeJSON(rt: SkillRuntimeForm): string {
  normalizeSkillRuntimeState(rt);
  return JSON.stringify({
    intent_routing_enabled: rt.intent_routing_enabled,
    intent_max_paths: rt.intent_max_paths,
    max_skills_in_toolset: rt.max_skills_in_toolset,
    allowed_slugs: [...rt.allowed_slugs],
    denied_slugs: [...rt.denied_slugs],
    allowed_tags: [...rt.allowed_tags],
  });
}

/** Watches allow/deny slug lists so deny wins (matches backend Layer A). */
export function useSkillRuntimeSlugSync(config: { skillRuntime: SkillRuntimeForm }) {
  const skillSlugListsSyncing = ref(false);

  watch(
    () => config.skillRuntime.allowed_slugs,
    (allowed) => {
      if (skillSlugListsSyncing.value) return;
      skillSlugListsSyncing.value = true;
      try {
        const allowSet = new Set((allowed ?? []).map(normSkillSlug).filter(Boolean));
        config.skillRuntime.denied_slugs = config.skillRuntime.denied_slugs.filter(
          (d) => !allowSet.has(normSkillSlug(d)),
        );
      } finally {
        void nextTick(() => {
          skillSlugListsSyncing.value = false;
        });
      }
    },
    { deep: true },
  );

  watch(
    () => config.skillRuntime.denied_slugs,
    (denied) => {
      if (skillSlugListsSyncing.value) return;
      skillSlugListsSyncing.value = true;
      try {
        const denySet = new Set((denied ?? []).map(normSkillSlug).filter(Boolean));
        config.skillRuntime.allowed_slugs = config.skillRuntime.allowed_slugs.filter(
          (a) => !denySet.has(normSkillSlug(a)),
        );
      } finally {
        void nextTick(() => {
          skillSlugListsSyncing.value = false;
        });
      }
    },
    { deep: true },
  );

  function normalizeWithSync() {
    skillSlugListsSyncing.value = true;
    try {
      normalizeSkillRuntimeState(config.skillRuntime);
    } finally {
      void nextTick(() => {
        skillSlugListsSyncing.value = false;
      });
    }
  }

  return { normalizeWithSync };
}

export function resetSkillRuntimeDefaults(config: AgentRuntimeConfigForm, notify: (msg: string) => void) {
  Object.assign(config.skillRuntime, parseSkillRuntimeForm('{}'));
  normalizeSkillRuntimeState(config.skillRuntime);
  notify('Skill 策略已恢复默认（尚未保存）');
}
