<template>
  <!--
    顶栏第二行。工作台、看板：本模块的页签横向排开；Harness 的页面在二级侧栏里，这里不重复。
    当前页面有子页（运行记录、审计、合并前的旧页面等）时，下面再出一行子页签。
    #app-tab-actions：页面把自己的少量操作（选公司、导入、新建）用 <Teleport defer> 放到这一行右边，
    不再单独占一行页头。
  -->
  <!-- Harness 页面没有子页签时也渲染这一行，页面才有 #app-tab-actions 可放操作；没有内容时由样式收起 -->
  <nav
    v-if="trail"
    class="app-domain-tabs"
    :class="{ 'app-domain-tabs--sub': !showTabs }"
    :aria-label="t(trail.domain.labelKey)"
  >
    <div v-if="showTabs" class="app-domain-tabs__row">
      <ul class="app-domain-tabs__list" role="list">
        <li v-for="(page, index) in trail.domain.primaries" :key="page.to">
          <router-link
            :to="page.to"
            class="app-domain-tabs__tab"
            :title="`Alt+${index + 1}`"
            :aria-keyshortcuts="`Alt+${index + 1}`"
            :class="{ 'app-domain-tabs__tab--active': page.to === trail.primary.to }"
            :aria-current="page.to === trail.primary.to && !trail.child ? 'page' : undefined"
          >
            {{ t(page.tabLabelKey) }}
          </router-link>
        </li>
      </ul>
    </div>
    <div id="app-tab-actions" class="app-domain-tabs__actions" />
    <ul v-if="trail.primary.children.length" class="app-domain-tabs__sub" role="list">
      <li>
        <router-link
          :to="trail.primary.to"
          class="app-domain-tabs__subtab"
          :class="{ 'app-domain-tabs__subtab--active': !trail.child }"
          :aria-current="!trail.child ? 'page' : undefined"
        >
          {{ t('shell.nav.tab.overviewOf', { name: t(trail.primary.tabLabelKey) }) }}
        </router-link>
      </li>
      <li v-for="child in trail.primary.children" :key="child.to">
        <router-link
          :to="child.to"
          class="app-domain-tabs__subtab"
          :class="{ 'app-domain-tabs__subtab--active': trail.child?.to === child.to }"
          :aria-current="trail.child?.to === child.to ? 'page' : undefined"
        >
          {{ t(child.tabLabelKey) }}
        </router-link>
      </li>
    </ul>
  </nav>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue';
import { useI18n } from 'vue-i18n';
import { useRouter } from 'vue-router';
import type { DomainTrail } from '../../config/navDomains';

const props = defineProps<{ trail: DomainTrail | null }>();

const { t } = useI18n();

const router = useRouter();

const showTabs = computed(() => props.trail?.domain.module === 'workbench' || props.trail?.domain.module === 'board');

// Alt+1…9 直接切到第 n 个页签（Alt 组合不产生字符，在输入框里按也不会打进去）
function onKey(event: KeyboardEvent) {
  if (!event.altKey || event.ctrlKey || event.metaKey || event.shiftKey || !showTabs.value) return;
  const page = props.trail?.domain.primaries[Number(event.key) - 1];
  if (!page) return;
  event.preventDefault();
  void router.push(page.to);
}
onMounted(() => window.addEventListener('keydown', onKey));
onUnmounted(() => window.removeEventListener('keydown', onKey));
</script>
