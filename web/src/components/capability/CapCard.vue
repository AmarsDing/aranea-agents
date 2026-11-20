<!--
  构件卡（Harness › 能力 各目录页共用）：图标 + 代号 + 状态；名称；一句话用途；三个关键事实；标签；
  底部一句关系 + 一个主操作 + 更多。整卡可点（Enter 同效），开关、按钮、菜单各管各的点击。
  设计：docs/aranea-agents/design/2026-11-17-三大模块重设计/04-Harness系统.md 第 5.1 节
  注释放在 template 外：放在里面会让组件变成两个根节点，父级的 class / 事件落不到卡片上。
-->
<template>
  <article
    class="cap-card"
    :class="{ 'cap-card--selected': selected, 'cap-card--off': status?.tone === 'off' }"
    tabindex="0"
    :aria-label="title"
    @click="emit('open')"
    @keydown.enter.self.prevent="emit('open')"
  >
    <header class="cap-card__head">
      <q-checkbox
        v-if="checked !== undefined"
        class="cap-card__check"
        dense
        :model-value="checked"
        :aria-label="t('cap.common.select', { name: title })"
        @click.stop
        @update:model-value="emit('update:checked', Boolean($event))"
      />
      <span class="cap-card__icon" aria-hidden="true">
        <img v-if="iconUrl" :src="iconUrl" alt="" />
        <span v-else-if="glyph" class="cap-card__glyph">{{ glyph }}</span>
        <q-icon v-else :name="icon" size="18px" />
      </span>
      <span class="cap-card__code" :title="code">{{ code }}</span>
      <span v-if="status" class="cap-card__status" :title="status.hint || status.label">
        <i class="cap-dot" :class="`cap-dot--${status.tone}`" aria-hidden="true" />{{ status.label }}
      </span>
      <div v-if="$slots.switch" class="cap-card__switch" @click.stop @keydown.stop>
        <slot name="switch" />
      </div>
    </header>

    <h3 class="cap-card__title">{{ title }}</h3>
    <p class="cap-card__desc" :class="{ 'cap-card__desc--empty': !description }">
      {{ description || t('cap.common.noDescription') }}
    </p>

    <dl v-if="facts?.length" class="cap-card__facts">
      <div
        v-for="fact in facts"
        :key="fact.label"
        class="cap-card__fact"
        :class="fact.tone && `cap-tone--${fact.tone}`"
      >
        <dt>{{ fact.label }}</dt>
        <dd :title="fact.hint">{{ fact.value }}</dd>
      </div>
    </dl>

    <ul v-if="tags?.length" class="cap-card__tags" role="list">
      <li v-for="tag in tags" :key="tag.label" class="cap-tag" :class="tag.tone && `cap-tag--${tag.tone}`">
        {{ tag.label }}
      </li>
    </ul>

    <!-- 卡片自己要补的一小段（如探测失败的原因），放在事实和标签之后 -->
    <slot />

    <footer v-if="$slots.relation || $slots.actions" class="cap-card__foot">
      <span class="cap-card__relation"><slot name="relation" /></span>
      <div class="cap-card__actions" @click.stop @keydown.stop>
        <slot name="actions" />
      </div>
    </footer>
  </article>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { CapFact, CapStatus, CapTag } from './capTypes';

withDefaults(
  defineProps<{
    icon: string;
    iconUrl?: string;
    /** 一个字符的图标（Agent 的 emoji）；优先级在 iconUrl 之后、icon 之前 */
    glyph?: string;
    /** 等宽代号：工具 key、技能 slug、插件 key… */
    code: string;
    title: string;
    description?: string;
    status?: CapStatus;
    facts?: CapFact[];
    tags?: CapTag[];
    selected?: boolean;
    /** 传了才出勾选框（批量操作） */
    checked?: boolean;
  }>(),
  // 显式默认 undefined：否则 Vue 把缺省的布尔 prop 转成 false，勾选框就永远出现
  { checked: undefined },
);

const emit = defineEmits<{ open: []; 'update:checked': [value: boolean] }>();

const { t } = useI18n();
</script>
