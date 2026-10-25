<template>
  <aside class="wb-roster">
    <div class="wb-roster__section-label">{{ t('workbench.roster.title') }}</div>
    <div class="wb-roster__scroll">
      <template v-if="roster.depts.length || roster.unassigned.length">
        <div v-for="dept in roster.depts" :key="dept.id" class="wb-roster__dept">
          <div class="wb-roster__dept-name" :data-bus-dept="dept.id">{{ dept.name }}</div>
          <template v-for="pos in dept.positions" :key="pos.id">
            <div class="wb-roster__pos-name">{{ pos.name }}</div>
            <button
              v-for="a in pos.agents"
              :key="a.key"
              type="button"
              class="wb-roster__agent"
              :class="{
                'wb-roster__agent--waiting': waiting.has(a.key),
                'wb-roster__agent--active': isLive(a.key),
              }"
              @click="$emit('select', a.key)"
            >
              <i class="wb-roster__dot" :style="dotStyle(a.key)" />
              <span class="wb-roster__at">@{{ a.name }}</span>
              <span class="wb-roster__word">{{ statusWord(a.key) }}</span>
            </button>
          </template>
          <button
            v-for="a in dept.floatAgents"
            :key="a.key"
            type="button"
            class="wb-roster__agent wb-roster__agent--float"
            :class="{ 'wb-roster__agent--waiting': waiting.has(a.key) }"
            @click="$emit('select', a.key)"
          >
            <i class="wb-roster__dot" :style="dotStyle(a.key)" />
            <span class="wb-roster__at">@{{ a.name }}</span>
            <span class="wb-roster__word">{{ statusWord(a.key) }}</span>
          </button>
        </div>
        <div v-if="roster.unassigned.length" class="wb-roster__dept">
          <div class="wb-roster__dept-name">{{ t('workbench.roster.unassigned') }}</div>
          <button
            v-for="a in roster.unassigned"
            :key="a.key"
            type="button"
            class="wb-roster__agent wb-roster__agent--float"
            :class="{ 'wb-roster__agent--waiting': waiting.has(a.key) }"
            @click="$emit('select', a.key)"
          >
            <i class="wb-roster__dot" :style="dotStyle(a.key)" />
            <span class="wb-roster__at">@{{ a.name }}</span>
            <span class="wb-roster__word">{{ statusWord(a.key) }}</span>
          </button>
        </div>
      </template>
      <div v-else-if="!loaded" class="wb-roster__empty">
        <p>{{ t('workbench.roster.loading') }}</p>
      </div>
      <div v-else class="wb-roster__empty">
        <p>{{ t('workbench.roster.empty') }}</p>
        <q-btn
          flat
          dense
          size="sm"
          no-caps
          color="primary"
          :label="t('workbench.roster.configure')"
          @click="$emit('open-organization')"
        />
      </div>
    </div>
    <div class="wb-roster__section-label wb-roster__source-label">{{ t('workbench.map.source') }}</div>
    <div class="wb-roster__sources">
      <div
        v-for="line in bizLines"
        :key="line.id"
        class="wb-roster__source"
        :class="{ dim: !line.connected }"
        :data-source="line.id"
      >
        <i class="wb-roster__src-state" :class="{ on: line.connected }" />
        <span class="wb-roster__src-name">{{ t(`workbench.lines.${line.id}`) }}</span>
        <span class="wb-roster__src-note">{{
          line.connected ? (line.pending > 0 ? String(line.pending) : '·') : t('workbench.sources.pending')
        }}</span>
      </div>
    </div>
  </aside>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { AgentLiveState, BizLineCard, RosterModel } from '../../features/workbench/types';

const props = defineProps<{
  roster: RosterModel;
  live: Map<string, AgentLiveState>;
  waiting: Set<string>;
  bizLines: BizLineCard[];
  /** 花名册首载完成前先显载入占位，避免空态 CTA 闪屏 */
  loaded: boolean;
}>();

defineEmits<{
  select: [agentKey: string];
  'open-organization': [];
}>();

const { t } = useI18n();

function isLive(key: string): boolean {
  const st = props.live.get(key);
  return !!st && st.statusKind !== 'idle';
}

function statusWord(key: string): string {
  const st = props.live.get(key);
  if (!st || st.statusKind === 'idle') return '';
  if (st.statusWord) return st.statusWord;
  return t(`workbench.row.${st.statusKind}`);
}

/** 角色色：稳定 hash → --agent-palette-*（形状×颜色双编码的颜色通道） */
function dotStyle(key: string): Record<string, string> {
  let h = 0;
  for (let i = 0; i < key.length; i += 1) h = (h * 31 + key.charCodeAt(i)) >>> 0;
  const idx = h % 9;
  return { background: `var(--agent-palette-${idx}, var(--wb-flow))` };
}
</script>

<style scoped lang="sass">
.wb-roster
  display: flex
  flex-direction: column
  height: 100%
  min-height: 0
  border-right: 1px solid var(--wb-line-hairline)
  background: color-mix(in srgb, var(--canvas-base) 92%, var(--color-text-primary))
.wb-roster__section-label
  flex: none
  padding: 10px 10px 6px
  font-size: var(--wb-micro-label)
  letter-spacing: 0.16em
  text-transform: uppercase
  color: var(--color-text-tertiary, var(--color-text-secondary))
.wb-roster__scroll
  flex: 1
  min-height: 0
  overflow-y: auto
  padding: 0 4px
.wb-roster__dept
  margin-bottom: 8px
.wb-roster__dept-name
  padding: 4px 8px
  font-size: 11px
  font-weight: 600
  color: var(--color-text-secondary)
  letter-spacing: 0.06em
.wb-roster__pos-name
  padding: 2px 8px 2px 18px
  font-size: 10px
  color: var(--color-text-tertiary, var(--color-text-secondary))
.wb-roster__agent
  display: flex
  align-items: center
  gap: 6px
  width: calc(100% - 4px)
  margin: 1px 2px
  padding: 4px 8px 4px 20px
  border: 1px dashed transparent
  border-radius: var(--wb-radius)
  background: transparent
  color: inherit
  font-size: var(--wb-caption)
  cursor: pointer
  text-align: left
  &:hover
    background: color-mix(in srgb, var(--color-text-primary) 5%, transparent)
.wb-roster__agent--waiting
  border-color: var(--wb-await-line)
.wb-roster__agent--active .wb-roster__word
  color: var(--wb-flow)
.wb-roster__agent--float
  padding-left: 8px
.wb-roster__dot
  flex: none
  width: 7px
  height: 7px
  border-radius: 50%
.wb-roster__at
  flex: none
  font-weight: 600
.wb-roster__word
  flex: 1
  min-width: 0
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap
  text-align: right
  color: var(--color-text-secondary)
  font-size: 10px
.wb-roster__empty
  padding: 12px 10px
  font-size: var(--wb-caption)
  color: var(--color-text-secondary)
.wb-roster__source-label
  border-top: 1px solid var(--wb-line-hairline)
.wb-roster__sources
  flex: none
  padding: 0 10px 10px
.wb-roster__source
  display: flex
  align-items: center
  gap: 6px
  padding: 4px 0
  font-size: var(--wb-caption)
  &.dim
    opacity: 0.45
.wb-roster__src-state
  width: 6px
  height: 6px
  border-radius: 50%
  background: var(--color-text-icon-muted)
  &.on
    background: var(--wb-ok)
.wb-roster__src-name
  flex: 1
  font-weight: 600
.wb-roster__src-note
  color: var(--color-text-secondary)
  font-variant-numeric: tabular-nums
</style>
