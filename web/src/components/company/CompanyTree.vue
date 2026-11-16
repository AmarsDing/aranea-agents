<template>
  <section class="co-tree" :aria-label="t('company.treeLabel')">
    <div class="co-tree__inner">
      <div class="co-spine">
        <button
          type="button"
          class="co-node co-node--me"
          :aria-pressed="selected === 'me'"
          @click="emit('select', 'me')"
        >
          <span class="co-node__row">
            <span class="co-node__name">{{ t('company.me.title') }}</span>
            <span v-if="tree.me.waiting > 0" class="co-chip co-chip--hot co-mono">
              {{ t('company.me.chip', { n: tree.me.waiting }) }}
            </span>
          </span>
          <span class="co-node__sub">{{ t('company.me.sub') }}</span>
        </button>

        <div class="co-flow co-mono">
          <span>↓ {{ t('company.flow.decide') }}</span>
          <span>↑ {{ t('company.flow.report') }}</span>
        </div>

        <button
          type="button"
          class="co-node co-node--ceo"
          :class="{ 'co-node--unset': !tree.ceo }"
          :aria-pressed="selected === 'ceo'"
          @click="emit('select', 'ceo')"
        >
          <span class="co-node__row">
            <span class="co-pulse" :class="{ 'co-pulse--on': tree.ceo?.ranToday }" aria-hidden="true" />
            <span class="co-node__main">
              <span class="co-node__name">{{ tree.ceo?.name ?? t('company.ceo.title') }}</span>
              <span class="co-node__sub">
                <template v-if="tree.ceo">
                  {{ t('company.ceo.role') }} ·
                  <span class="co-mono">{{ shortTime(tree.ceo.lastRunAt, now) || t('company.neverRan') }}</span>
                </template>
                <template v-else>{{ t(tree.ceoConfigured ? 'company.ceo.missing' : 'company.ceo.unset') }}</template>
              </span>
            </span>
          </span>
        </button>
        <div class="co-stub" />
      </div>

      <p v-if="tree.depts.length === 0" class="co-note">{{ t('company.noDepts') }}</p>
      <template v-else>
        <div class="co-bus" />
        <div class="co-depts" :style="{ '--co-cols': tree.depts.length }">
          <div
            v-for="(dept, index) in tree.depts"
            :key="dept.id"
            class="co-dept"
            :style="{ '--co-c': `var(--nt-domain-${(index % 5) + 1})` }"
          >
            <div class="co-dept__stub" />
            <div class="co-dept__edge co-mono">
              <span>↓ {{ t('company.edge.delegated', { n: dept.delegated }) }}</span>
              <span
                >↑
                {{
                  dept.waiting > 0 ? t('company.edge.waiting', { n: dept.waiting }) : t('company.edge.noWaiting')
                }}</span
              >
            </div>
            <button
              type="button"
              class="co-node co-node--dept"
              :aria-pressed="selected === `dept:${dept.id}`"
              @click="emit('select', `dept:${dept.id}`)"
            >
              <span class="co-node__row">
                <span class="co-dept__mark" aria-hidden="true" />
                <span class="co-node__name">{{ dept.name }}</span>
                <span class="co-state" :class="`co-state--${dept.state}`">{{ t(`company.state.${dept.state}`) }}</span>
              </span>
              <span class="co-node__row co-node__row--figure">
                <span class="co-figure co-mono"
                  >{{ dept.activeToday }}<small>/{{ dept.agentCount }}</small></span
                >
                <span class="co-node__sub">{{ t('company.dept.activeLabel') }}</span>
              </span>
              <span class="co-node__sub co-mono">
                {{
                  t('company.dept.rhythm', {
                    last: shortTime(dept.lastRunAt, now) || '—',
                    next: shortTime(dept.nextRunAt, now) || '—',
                  })
                }}
              </span>
            </button>

            <ul class="co-leaves">
              <li v-for="pos in visiblePositions(dept)" :key="pos.id">
                <button
                  type="button"
                  class="co-leaf"
                  :aria-pressed="selected === `pos:${dept.id}:${pos.id}`"
                  @click="emit('select', `pos:${dept.id}:${pos.id}`)"
                >
                  <span class="co-dot" :class="`co-dot--${pos.state}`" aria-hidden="true" />
                  <span class="co-leaf__name">{{ pos.name }}</span>
                  <span class="co-leaf__val co-mono" :class="{ 'co-leaf__val--hot': pos.state === 'attention' }">
                    {{ leafValue(pos) }}
                  </span>
                </button>
              </li>
              <li v-if="dept.positions.length > LEAF_LIMIT">
                <button type="button" class="co-link" :aria-expanded="expanded.has(dept.id)" @click="toggle(dept.id)">
                  {{
                    expanded.has(dept.id)
                      ? t('company.dept.collapse')
                      : t('company.dept.more', { n: dept.positions.length - LEAF_LIMIT })
                  }}
                </button>
              </li>
              <li v-if="dept.positions.length === 0" class="co-node__sub">{{ t('company.dept.noPositions') }}</li>
            </ul>
          </div>
        </div>
      </template>
    </div>
  </section>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { shortTime, type CompanyTree, type DeptNode, type PositionLeaf } from '../../features/company/tree';

const LEAF_LIMIT = 4;

defineProps<{
  tree: CompanyTree;
  now: Date;
  /** me / ceo / dept:<id> / pos:<deptId>:<posId> */
  selected: string;
}>();
const emit = defineEmits<{ select: [id: string] }>();

const { t } = useI18n();
const expanded = ref<Set<string>>(new Set());

function toggle(deptId: string) {
  const next = new Set(expanded.value);
  if (next.has(deptId)) next.delete(deptId);
  else next.add(deptId);
  expanded.value = next;
}

function visiblePositions(dept: DeptNode): PositionLeaf[] {
  return expanded.value.has(dept.id) ? dept.positions : dept.positions.slice(0, LEAF_LIMIT);
}

function leafValue(pos: PositionLeaf): string {
  if (pos.failed > 0) return t('company.leaf.failed', { n: pos.failed });
  if (pos.waiting > 0) return t('company.leaf.waiting', { n: pos.waiting });
  if (pos.agents.length === 0) return t('company.leaf.empty');
  return t('company.leaf.agents', { n: pos.agents.length });
}
</script>
