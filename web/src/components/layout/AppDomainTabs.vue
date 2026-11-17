<template>
  <!--
    页面顶部的领域页签：同一个一级去处下的页面横向排开，领域内切换不必回侧栏。
    当前页面有子页（运行记录、审计等）时，下面再出一行子页签。
  -->
  <nav v-if="trail" class="app-domain-tabs" :aria-label="t(trail.domain.labelKey)">
    <div class="app-domain-tabs__row">
      <span class="app-domain-tabs__domain">
        <q-icon :name="trail.domain.icon" size="18px" aria-hidden="true" />
        {{ t(trail.domain.labelKey) }}
      </span>
      <ul class="app-domain-tabs__list" role="list">
        <li v-for="page in trail.domain.primaries" :key="page.to">
          <router-link
            :to="page.to"
            class="app-domain-tabs__tab"
            :class="{ 'app-domain-tabs__tab--active': page.to === trail.primary.to }"
            :aria-current="page.to === trail.primary.to && !trail.child ? 'page' : undefined"
          >
            {{ t(page.tabLabelKey) }}
          </router-link>
        </li>
      </ul>
    </div>
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
import { useI18n } from 'vue-i18n';
import type { DomainTrail } from '../../config/navDomains';

defineProps<{ trail: DomainTrail | null }>();

const { t } = useI18n();
</script>
