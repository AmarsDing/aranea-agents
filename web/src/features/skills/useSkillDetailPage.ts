import { onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useSkillsStore } from '../../stores/skills';
import type { Skill, SkillHealthMetric } from './types';

export function useSkillDetailPage(skillId: string) {
  const router = useRouter();
  const skillsStore = useSkillsStore();

  const skill = ref<Skill | null>(null);
  const health = ref<SkillHealthMetric | null>(null);
  const loadingSkill = ref(false);
  const loadingHealth = ref(false);
  const skillError = ref('');
  const healthError = ref('');
  const bodyMarkdown = ref('');

  async function loadSkill() {
    loadingSkill.value = true;
    skillError.value = '';
    try {
      const result = await skillsStore.loadSkill(skillId);
      skill.value = result.skill;
      bodyMarkdown.value = result.bodyMarkdown;
    } catch (err) {
      skillError.value = err instanceof Error ? err.message : '加载 Skill 失败';
    } finally {
      loadingSkill.value = false;
    }
  }

  async function loadHealth() {
    loadingHealth.value = true;
    healthError.value = '';
    try {
      health.value = await skillsStore.loadSkillHealth(skillId);
    } catch (err) {
      healthError.value = err instanceof Error ? err.message : '加载健康数据失败';
    } finally {
      loadingHealth.value = false;
    }
  }

  function goBack() {
    router.push({ name: 'skills' });
  }

  onMounted(() => {
    void loadSkill();
    void loadHealth();
  });

  return {
    router,
    skill,
    health,
    bodyMarkdown,
    loadingSkill,
    loadingHealth,
    skillError,
    healthError,
    loadSkill,
    loadHealth,
    goBack,
  };
}
