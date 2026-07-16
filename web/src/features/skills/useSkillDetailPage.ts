import { onMounted, ref } from 'vue';
import { useQuasar } from 'quasar';
import { useRouter } from 'vue-router';
import { useSkillsStore } from '../../stores/skills';
import type { Skill, SkillHealthMetric, SkillVersionDetail } from './types';

export function useSkillDetailPage(skillId: string) {
  const $q = useQuasar();
  const router = useRouter();
  const skillsStore = useSkillsStore();

  const skill = ref<Skill | null>(null);
  const health = ref<SkillHealthMetric | null>(null);
  const versions = ref<SkillVersionDetail[]>([]);
  const loadingSkill = ref(false);
  const loadingHealth = ref(false);
  const loadingVersions = ref(false);
  const rollingId = ref('');
  const skillError = ref('');
  const healthError = ref('');
  const versionsError = ref('');
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

  async function loadVersions() {
    loadingVersions.value = true;
    versionsError.value = '';
    try {
      const result = await skillsStore.loadVersions(skillId, 1, 50);
      versions.value = result.items;
    } catch (err) {
      versionsError.value = err instanceof Error ? err.message : '加载版本失败';
    } finally {
      loadingVersions.value = false;
    }
  }

  async function rollbackVersion(version: SkillVersionDetail) {
    const ok = await new Promise<boolean>((resolve) => {
      $q.dialog({
        title: '回滚版本',
        message: `确认回滚到 ${version.version || version.id}？将新建版本并保留历史。`,
        cancel: { label: '取消', flat: true, noCaps: true },
        ok: { label: '回滚', noCaps: true, color: 'primary' },
        persistent: true,
      })
        .onOk(() => resolve(true))
        .onCancel(() => resolve(false));
    });
    if (!ok) return;
    rollingId.value = version.id;
    try {
      const updated = await skillsStore.rollbackVersion(skillId, version.id);
      skill.value = updated;
      $q.notify({ type: 'positive', message: '已回滚并生成新版本' });
      await loadVersions();
      await loadSkill();
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '回滚失败' });
    } finally {
      rollingId.value = '';
    }
  }

  function goBack() {
    router.push({ name: 'skills' });
  }

  onMounted(() => {
    void loadSkill();
    void loadHealth();
    void loadVersions();
  });

  return {
    router,
    skill,
    health,
    versions,
    bodyMarkdown,
    loadingSkill,
    loadingHealth,
    loadingVersions,
    rollingId,
    skillError,
    healthError,
    versionsError,
    loadSkill,
    loadHealth,
    loadVersions,
    rollbackVersion,
    goBack,
  };
}
