<template>
  <nav class="app-breadcrumbs" :aria-label="t('shell.crumbs.home')">
    <ol class="app-breadcrumbs__list">
      <li v-for="(crumb, index) in items" :key="`${crumb.label}-${index}`" class="app-breadcrumbs__item">
        <q-icon v-if="index > 0" name="chevron_right" size="14px" class="app-breadcrumbs__sep" aria-hidden="true" />
        <router-link
          v-if="crumb.to && index < items.length - 1"
          :to="crumb.to"
          class="app-breadcrumbs__link"
          :class="crumbClass(index)"
        >
          {{ crumb.label }}
        </router-link>
        <span
          v-else
          class="app-breadcrumbs__current"
          :class="crumbClass(index)"
          :aria-current="index === items.length - 1 ? 'page' : undefined"
        >
          {{ crumb.label }}
        </span>
      </li>
    </ol>
  </nav>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useRoute } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { findNavTrail } from '../../config/navView';

const route = useRoute();
const { t } = useI18n();

type Crumb = { label: string; to?: string; context?: boolean };

const items = computed<Crumb[]>(() => {
  const trail = findNavTrail(route.path);
  const titleKey = route.meta.titleKey as string | undefined;
  const pageTitle = titleKey ? t(titleKey) : '';

  // 首页不显示面包屑：它自己就是根
  if (route.path === '/workbench' || route.path === '/') {
    return [{ label: t('shell.crumbs.home') }];
  }

  const crumbs: Crumb[] = [{ label: t('shell.crumbs.home'), to: '/workbench' }];
  if (trail) {
    crumbs.push({ label: t(trail.group.labelKey), context: true });
    if (trail.section.labelKey) crumbs.push({ label: t(trail.section.labelKey), context: true });
    const itemLabel = t(trail.item.labelKey);
    crumbs.push({ label: itemLabel, to: trail.item.to });
    if (pageTitle && pageTitle !== itemLabel) crumbs.push({ label: pageTitle });
  } else if (pageTitle) {
    crumbs.push({ label: pageTitle });
  }

  return crumbs.filter((crumb, index) => index === 0 || crumb.label !== crumbs[index - 1].label);
});

function crumbClass(index: number) {
  const crumb = items.value[index];
  if (!crumb?.context) return '';
  return 'app-breadcrumbs__crumb--context';
}
</script>
