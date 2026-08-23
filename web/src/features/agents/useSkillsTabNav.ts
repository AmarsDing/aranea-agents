import { onBeforeUnmount, onMounted, ref } from 'vue';

/**
 * Skill/工具 Tab 页内锚点导航：IntersectionObserver scrollspy + 平滑滚动定位。
 * 触发线对齐固定头（52px）+ 黏性导航高度，底部收窄避免多区同时命中。
 */
export function useSkillsTabNav(sectionIds: string[]) {
  const activeId = ref(sectionIds[0] ?? '');
  let observer: IntersectionObserver | null = null;

  function selectSection(id: string) {
    activeId.value = id;
    document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  }

  /** 滚到页底时末区可能够不到触发线，兜底激活最后一项。 */
  function onScroll() {
    if (window.innerHeight + window.scrollY >= document.documentElement.scrollHeight - 4) {
      activeId.value = sectionIds[sectionIds.length - 1] ?? activeId.value;
    }
  }

  onMounted(() => {
    const sections = sectionIds
      .map((id) => document.getElementById(id))
      .filter((el): el is HTMLElement => Boolean(el));
    if (sections.length && typeof IntersectionObserver !== 'undefined') {
      observer = new IntersectionObserver(
        (entries) => {
          for (const entry of entries) {
            if (entry.isIntersecting) activeId.value = entry.target.id;
          }
        },
        { rootMargin: '-132px 0px -70% 0px', threshold: 0 },
      );
      for (const el of sections) observer.observe(el);
    }
    window.addEventListener('scroll', onScroll, { passive: true });
  });

  onBeforeUnmount(() => {
    observer?.disconnect();
    observer = null;
    window.removeEventListener('scroll', onScroll);
  });

  return { activeId, selectSection };
}
