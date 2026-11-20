<template>
  <div :class="shell ? shellClasses : undefined">
    <!-- 视图条：即时筛选（只筛已读到的这一页）+ 命中计数 + 卡片 / 表格切换 -->
    <div v-if="showBar" class="app-registry-bar">
      <label v-if="quickEnabled" class="app-registry-quick">
        <q-icon name="bolt" size="16px" aria-hidden="true" />
        <input
          v-model="quick"
          class="app-registry-quick__input"
          type="search"
          autocomplete="off"
          :placeholder="t('registryView.quickPlaceholder')"
          :aria-label="t('registryView.quickLabel')"
          @keydown.esc.stop="quick = ''"
          @keydown.down.prevent="focusCard(0)"
        />
      </label>
      <span class="app-registry-bar__count" aria-live="polite">
        {{
          quick.trim()
            ? t('registryView.matched', { shown: shownCount, total: totalCount })
            : t('registryView.total', { total: totalCount })
        }}
      </span>
      <div
        v-if="cardsAvailable"
        class="app-registry-viewswitch"
        role="group"
        :aria-label="t('registryView.switchLabel')"
      >
        <button
          type="button"
          :aria-pressed="mode === 'cards'"
          :aria-label="t('registryView.cards')"
          :title="t('registryView.cards')"
          @click="setMode('cards')"
        >
          <q-icon name="grid_view" size="18px" aria-hidden="true" />
        </button>
        <button
          type="button"
          :aria-pressed="mode === 'table'"
          :aria-label="t('registryView.table')"
          :title="t('registryView.table')"
          @click="setMode('table')"
        >
          <q-icon name="view_list" size="18px" aria-hidden="true" />
        </button>
      </div>
    </div>

    <q-table v-bind="tableBind" :grid="mode === 'cards'" :flat="flat" :dense="dense" :class="tableClasses">
      <template v-for="name in forwardedSlotNames" :key="name" #[name]="slotData">
        <slot :name="name" v-bind="slotData ?? {}" />
      </template>
      <!-- 统一空/错态：父级显式提供 #no-data 时以父级为准 -->
      <template #no-data>
        <slot name="no-data">
          <AppStatePanel
            :variant="error ? 'error' : 'empty'"
            :title="error ? '' : emptyTitle"
            :description="error || emptyDescription"
            :icon="error ? '' : emptyIcon"
            :action-label="error ? t('shell.common.retry') : emptyActionLabel"
            :action-icon="error ? 'refresh' : emptyActionIcon"
            dense
            @action="error ? emit('retry') : emit('empty-action')"
          />
        </slot>
      </template>
      <template v-for="colName in resizableHeaderColumns" :key="colName" #[headerCellSlot(colName)]="slotData">
        <q-th :props="slotData" class="app-registry-th-resizable">
          {{ slotData.col.label }}
          <span
            class="app-registry-col-resize"
            role="separator"
            tabindex="0"
            aria-orientation="vertical"
            :aria-label="t('uiSystem.resizeColumn', { label: slotData.col.label })"
            @pointerdown.stop.prevent="onResizeStart(String(colName), $event)"
            @keydown="onResizeKeydown(String(colName), $event)"
          />
        </q-th>
      </template>

      <!-- 卡片视图：同一批列换一种排法。单元格沿用各页已有的 body-cell 插槽，不重复写渲染逻辑 -->
      <template v-if="mode === 'cards' && !slots.item" #item="item">
        <div class="app-registry-card-wrap">
          <article
            class="app-registry-card"
            :class="{ 'app-registry-card--selected': item.selected, 'app-registry-card--clickable': hasRowClick }"
            tabindex="0"
            @click="onCardClick($event, item)"
            @keydown="onCardKey($event, item)"
          >
            <header class="app-registry-card__head">
              <q-checkbox
                v-if="hasSelection"
                :model-value="item.selected"
                dense
                :aria-label="t('registryView.select')"
                @update:model-value="item.selected = Boolean($event)"
                @click.stop
              />
              <div v-if="split.title" class="app-registry-card__title app-registry-card__cell">
                <slot
                  v-if="slots[`body-cell-${split.title.name}`]"
                  :name="`body-cell-${split.title.name}`"
                  v-bind="cellProps(item, split.title)"
                />
                <span v-else>{{ text(split.title, item.row) }}</span>
              </div>
              <div v-if="split.status" class="app-registry-card__status app-registry-card__cell">
                <slot
                  v-if="slots[`body-cell-${split.status.name}`]"
                  :name="`body-cell-${split.status.name}`"
                  v-bind="cellProps(item, split.status)"
                />
                <span v-else>{{ text(split.status, item.row) }}</span>
              </div>
            </header>
            <div v-if="split.metric" class="app-registry-card__metric">
              <span class="app-registry-card__metric-label">{{ split.metric.label }}</span>
              <div class="app-registry-card__metric-value app-registry-card__cell">
                <slot
                  v-if="slots[`body-cell-${split.metric.name}`]"
                  :name="`body-cell-${split.metric.name}`"
                  v-bind="cellProps(item, split.metric)"
                />
                <span v-else>{{ text(split.metric, item.row) }}</span>
              </div>
            </div>
            <div v-if="split.meta.length" class="app-registry-card__meta">
              <span
                v-for="col in split.meta"
                :key="col.name"
                class="app-registry-card__meta-item app-registry-card__cell"
                :title="col.label"
              >
                <slot
                  v-if="slots[`body-cell-${col.name}`]"
                  :name="`body-cell-${col.name}`"
                  v-bind="cellProps(item, col)"
                />
                <span v-else>{{ text(col, item.row) }}</span>
              </span>
            </div>
            <dl v-if="split.body.length" class="app-registry-card__body">
              <div v-for="col in split.body" :key="col.name" class="app-registry-card__field">
                <dt>{{ col.label }}</dt>
                <dd class="app-registry-card__cell">
                  <slot
                    v-if="slots[`body-cell-${col.name}`]"
                    :name="`body-cell-${col.name}`"
                    v-bind="cellProps(item, col)"
                  />
                  <span v-else>{{ text(col, item.row) }}</span>
                </dd>
              </div>
            </dl>
            <!-- 操作条：平时收起，悬停或键盘聚焦到卡片时浮出；触屏下常显 -->
            <footer
              v-if="split.action && slots[`body-cell-${split.action.name}`]"
              class="app-registry-card__actions app-registry-card__cell"
              @click.stop
              @keydown.stop
            >
              <slot :name="`body-cell-${split.action.name}`" v-bind="cellProps(item, split.action)" />
            </footer>
          </article>
        </div>
      </template>
    </q-table>
  </div>
</template>

<script setup lang="ts">
import { computed, getCurrentInstance, onMounted, ref, useAttrs, useSlots } from 'vue';
import { useI18n } from 'vue-i18n';
import type { QTableProps } from 'quasar';
import {
  cellValue,
  defaultViewMode,
  loadViewMode,
  nextCardIndex,
  quickFilterRows,
  saveViewMode,
  splitColumns,
  type RegistryColumnLike,
  type RegistryViewMode,
} from '../../features/ui/registryView';
import { useResizableRegistryColumns } from '../../features/ui/useResizableRegistryColumns';
import AppStatePanel from '../common/AppStatePanel.vue';

defineOptions({ inheritAttrs: false });

type Row = Record<string, unknown>;
type CardColumn = RegistryColumnLike & { name: string; label: string };
type CardItem = { row: Row; rowIndex: number; selected: boolean; cols: CardColumn[] };

const props = withDefaults(
  defineProps<{
    /** 玻璃外壳；设为 false 时由父级 panel / dialog 提供容器 */
    shell?: boolean;
    /** 使用 .app-data-table-shell（嵌套面板内表格） */
    dataShell?: boolean;
    /** q-table 附加类名，如 skill-table、plugins-table */
    tableClass?: string;
    /** 使用外部分页条时隐藏 q-table 底栏 */
    externalPagination?: boolean;
    dense?: boolean;
    flat?: boolean;
    /** 表头拖拽调整列宽（默认开启） */
    resizable?: boolean;
    /** localStorage 持久化 key；同页多表时请显式区分 */
    columnPersistKey?: string;
    /** 数据来源失败信息；非空时表格体渲染错误态并提供重试 */
    error?: string;
    emptyTitle?: string;
    emptyDescription?: string;
    emptyIcon?: string;
    emptyActionLabel?: string;
    emptyActionIcon?: string;
    /**
     * 视图：auto = 页面级登记表按列形态决定默认卡片或表格，用户可切换并被记住；
     * table = 固定表格，不出视图条（明细、日志、嵌在别处的小表）。
     */
    view?: 'auto' | 'table';
    /** 服务端分页时的总条数；不传就按当前页的行数计 */
    total?: number;
  }>(),
  {
    shell: true,
    dataShell: false,
    tableClass: '',
    externalPagination: true,
    dense: true,
    flat: true,
    resizable: true,
    columnPersistKey: '',
    error: '',
    emptyTitle: '',
    emptyDescription: '',
    emptyIcon: 'inbox',
    emptyActionLabel: '',
    emptyActionIcon: '',
    view: 'auto',
    total: undefined,
  },
);

const emit = defineEmits<{ retry: []; 'empty-action': [] }>();

const attrs = useAttrs();
const { t } = useI18n();
const slots = useSlots();
const instance = getCurrentInstance();

const sourceColumns = computed(() => {
  const bind = attrs as Record<string, unknown>;
  const columns = bind.columns as QTableProps['columns'] | undefined;
  return Array.isArray(columns) ? columns : undefined;
});

const { displayColumns, onResizeStart, onResizeKeydown } = useResizableRegistryColumns(() => sourceColumns.value, {
  enabled: () => props.resizable,
  persistKey: () => props.columnPersistKey || undefined,
});

// ——— 视图与即时筛选 ———
const bindAttrs = computed(() => attrs as Record<string, unknown>);
const rows = computed<Row[]>(() => (Array.isArray(bindAttrs.value.rows) ? (bindAttrs.value.rows as Row[]) : []));
const cardColumns = computed<CardColumn[]>(() => (sourceColumns.value ?? []) as unknown as CardColumn[]);
const split = computed(() => splitColumns(cardColumns.value));

// 页面级登记表才出视图条；嵌在面板 / 弹层里的小表、自绘整行的表保持原样。
// 很多页面自己提供外壳（shell=false），所以不看 shell；弹层、菜单、侧向抽屉里的表按小表处理。
const embedded = ref(false);
// 页面工具条里已经有搜索框时，不再叠一条即时筛选（同一页两个搜索框，用户不知道用哪个）。
const pageHasSearch = ref(false);
onMounted(() => {
  const root = instance?.proxy?.$el as HTMLElement | undefined;
  embedded.value = Boolean(root?.closest?.('.q-dialog, .q-menu, .q-drawer--right'));
  const page = root?.closest?.('.q-page');
  pageHasSearch.value = Boolean(
    page?.querySelector('.app-page-toolbar input, .app-page-toolbar__search, .cap-bar__search'),
  );
});
const pageLevel = computed(() => props.view === 'auto' && !props.dataShell && !embedded.value);
const cardsAvailable = computed(() => pageLevel.value && !slots.body && cardColumns.value.length > 0);
// 父级自己管筛选或走服务端分页时不叠加即时筛选（否则每敲一个字都会触发一次请求）。
const quickEnabled = computed(
  () => pageLevel.value && !pageHasSearch.value && bindAttrs.value.filter === undefined && !bindAttrs.value.onRequest,
);
const showBar = computed(() => (cardsAvailable.value || quickEnabled.value) && rows.value.length > 0);

const viewKey = computed(() => props.columnPersistKey || props.tableClass || 'default');
const chosen = ref<RegistryViewMode | null>(loadViewMode(viewKey.value));
const mode = computed<RegistryViewMode>(() => {
  if (!cardsAvailable.value) return 'table';
  return chosen.value ?? defaultViewMode(cardColumns.value);
});
function setMode(next: RegistryViewMode) {
  chosen.value = next;
  saveViewMode(viewKey.value, next);
}

const quick = ref('');
// 服务端分页时父级传 total（全部条数），否则只能数到当前这一页
const totalCount = computed(() => props.total ?? rows.value.length);
const shownCount = computed(() => quickFilterRows(rows.value, quick.value, cardColumns.value).length);

const hasSelection = computed(() => {
  const selection = bindAttrs.value.selection;
  return selection === 'single' || selection === 'multiple';
});
const hasRowClick = computed(() => typeof bindAttrs.value.onRowClick === 'function');

function text(col: CardColumn, row: Row): string {
  const value = cellValue(col, row);
  return value === undefined || value === null || value === '' ? '—' : String(value);
}

/**
 * 复用各页 body-cell 插槽时补齐它期望的入参（列、取值）。
 * 返回 any：这些插槽原本经 q-table 透传、入参即为 any，各页模板按自己的行类型取字段；
 * 这里若给出具体类型，会让 40 多处既有用法的行字段全部变成 unknown。
 */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function cellProps(item: CardItem, col: CardColumn): any {
  const live = item.cols.find((c) => c.name === col.name) ?? col;
  return { ...item, col: live, value: cellValue(col, item.row) };
}

function cardElements(): HTMLElement[] {
  const root = instance?.proxy?.$el as HTMLElement | undefined;
  return root ? Array.from(root.querySelectorAll<HTMLElement>('.app-registry-card')) : [];
}

function focusCard(index: number) {
  cardElements()[index]?.focus();
}

function onCardClick(event: MouseEvent, item: CardItem) {
  // 点在卡片内的控件（开关、链接、按钮）上不算点卡片
  if ((event.target as HTMLElement).closest('button, a, input, label, .q-toggle, .q-checkbox, [role="button"]')) return;
  (bindAttrs.value.onRowClick as ((e: Event, row: Row, index: number) => void) | undefined)?.(
    event,
    item.row,
    item.rowIndex,
  );
}

/** 方向键在卡片间移动，Enter 打开，空格勾选。只响应卡片本身的按键，不抢卡片内控件的。 */
function onCardKey(event: KeyboardEvent, item: CardItem) {
  if (event.target !== event.currentTarget) return;
  if (event.key === 'Enter' && hasRowClick.value) {
    (bindAttrs.value.onRowClick as (e: Event, row: Row, index: number) => void)(event, item.row, item.rowIndex);
    return;
  }
  if (event.key === ' ' && hasSelection.value) {
    event.preventDefault();
    item.selected = !item.selected;
    return;
  }
  if (!['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown', 'Home', 'End'].includes(event.key)) return;
  event.preventDefault();
  const cards = cardElements();
  const rects = cards.map((el) => {
    const r = el.getBoundingClientRect();
    return { left: r.left, top: r.top };
  });
  const next = nextCardIndex(rects, cards.indexOf(event.currentTarget as HTMLElement), event.key as 'ArrowLeft');
  cards[next]?.focus();
}

/** 将 columns.style 同步到 headerStyle，并应用用户拖拽后的列宽 */
const tableBind = computed<QTableProps>(() => {
  const bind = { ...(attrs as Record<string, unknown>) } as unknown as QTableProps;
  if (displayColumns.value) {
    bind.columns = displayColumns.value;
  }
  if (quickEnabled.value) {
    bind.filter = quick.value.trim() || undefined;
    bind.filterMethod = (list, terms) => quickFilterRows(list as Row[], String(terms ?? ''), cardColumns.value);
  }
  return bind;
});

const resizableHeaderColumns = computed(() => {
  if (!props.resizable || !displayColumns.value) return [];
  return displayColumns.value
    .map((col) => (col && typeof col === 'object' ? String(col.name ?? '') : ''))
    .filter((name) => name && !slots[`header-cell-${name}`]);
});

// no-data 由下方显式模板接管，避免同名插槽重复定义
const forwardedSlotNames = computed(() => Object.keys(slots).filter((name) => name !== 'no-data'));

function headerCellSlot(colName: string) {
  return `header-cell-${colName}`;
}

const shellClasses = computed(() => (props.dataShell ? 'app-data-table-shell' : 'app-registry-table-shell'));

const tableClasses = computed(() => {
  const classes = ['app-registry-table'];
  if (props.tableClass) classes.push(props.tableClass);
  if (props.externalPagination) classes.push('app-registry-table--external-pagination');
  if (props.resizable) classes.push('app-registry-table--resizable');
  if (mode.value === 'cards') classes.push('app-registry-table--cards');
  return classes;
});
</script>
