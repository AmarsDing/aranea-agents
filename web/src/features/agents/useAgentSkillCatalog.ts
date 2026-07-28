import { ref } from 'vue';
import { listSkills, listSkillTags } from '../skills/api';
import { getCodeExecutorCapabilities } from '../monitor/api';
import type { CodeExecutorCapability } from '../monitor/types';

/** Skill slug options and code-executor capabilities for Agent settings. */
export function useAgentSkillCatalog() {
  const skillSlugOptions = ref<{ label: string; value: string }[]>([]);
  const loadingSkillSlugs = ref(false);
  const skillTagOptions = ref<string[]>([]);
  const loadingSkillTags = ref(false);
  const codeExecutorCapabilities = ref<CodeExecutorCapability[]>([]);

  async function loadSkillSlugOptions() {
    loadingSkillSlugs.value = true;
    try {
      const data = await listSkills({ page: 1, page_size: 500 });
      const seen = new Set<string>();
      const next: { label: string; value: string }[] = [];
      for (const s of data.items) {
        const slug = String(s.slug ?? '').trim();
        if (!slug || seen.has(slug)) continue;
        seen.add(slug);
        const statusTip =
          s.status === 'published'
            ? '已发布'
            : s.status === 'draft'
              ? '草稿'
              : s.status === 'archived'
                ? '已归档'
                : s.status;
        next.push({
          label: `${s.name || slug} · ${slug} · ${statusTip}`,
          value: slug,
        });
      }
      skillSlugOptions.value = next;
    } catch {
      skillSlugOptions.value = [];
    } finally {
      loadingSkillSlugs.value = false;
    }
  }

  /** 标签字典选项源：规范标签名（字典 + 使用中），供 allowed_tags 下拉。 */
  async function loadSkillTagOptions() {
    loadingSkillTags.value = true;
    try {
      const tags = await listSkillTags();
      skillTagOptions.value = tags.map((t) => t.name).filter(Boolean);
    } catch {
      skillTagOptions.value = [];
    } finally {
      loadingSkillTags.value = false;
    }
  }

  async function loadCodeExecutorCapabilities() {
    try {
      codeExecutorCapabilities.value = await getCodeExecutorCapabilities();
    } catch {
      codeExecutorCapabilities.value = [];
    }
  }

  return {
    skillSlugOptions,
    loadingSkillSlugs,
    loadSkillSlugOptions,
    skillTagOptions,
    loadingSkillTags,
    loadSkillTagOptions,
    codeExecutorCapabilities,
    loadCodeExecutorCapabilities,
  };
}
