import { computed, onMounted, reactive, ref, watch } from 'vue';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import { useRouter } from 'vue-router';
import { usePlatformStore } from '../../stores/platform';
import { confirm } from '../../composables/useAppConfirm';
import { isForbiddenApiError, parseKratosApiError } from '../../utils/kratosError';
import { listAgents } from '../agents/api';
import type { Agent } from '../agents/types';
import { listTeams } from '../teams/api';
import type { Team } from '../teams/types';
import type { PlatformResourceInput } from '../platform/types';
import {
  buildTaxonomyKey,
  nextTaxonomySortOrder,
  parseIsSystem,
  type TaxonomyLevel,
} from '../platform/taxonomyTreeUtils';
import { TEAM_AGENT_LIST_LIMIT } from '../constants/queryLimits';
import type { TreeRailCompany } from '../../components/organization/treeRail';
import {
  agentsByNode,
  blockingTeams,
  companiesOf,
  companyCounts,
  companyLeadOf,
  companyMatchCount,
  deleteImpact,
  deptCounts,
  deptLeadOf,
  deptMatchCount,
  departmentsOf,
  findNode,
  isOfficeDept,
  moveSibling,
  positionMatches,
  positionsOf,
  teamsByDepartment,
  type OrgNode,
} from './orgChart';

const RESOURCE = 'organization' as const;
const SELECTION_KEY = 'aranea:org-chart:selection:v1';

type Selection = { company: string; dept: string };

function readSelection(): Selection {
  try {
    const raw = JSON.parse(localStorage.getItem(SELECTION_KEY) || '{}') as Partial<Selection>;
    return { company: String(raw.company ?? ''), dept: String(raw.dept ?? '') };
  } catch {
    return { company: '', dept: '' };
  }
}

function writeSelection(sel: Selection) {
  try {
    localStorage.setItem(SELECTION_KEY, JSON.stringify(sel));
  } catch {
    /* 选择记忆只是便利，存不了就算了 */
  }
}

/** 检视面板：看一个节点 / 编辑它 / 在某个父节点下新建。 */
export type OrgInspector =
  | { mode: 'closed' }
  | { mode: 'view'; nodeId: string }
  | { mode: 'edit'; nodeId: string }
  | { mode: 'create'; level: TaxonomyLevel; parentId: string };

export type OrgNodeForm = { name: string; description: string; enabled: boolean };

/**
 * 组织架构页：公司页签 → 部门栏 → 部门详情（主管、岗位与坐岗 Agent、所属团队）→ 右侧检视面板。
 * 数据：组织树走 platform store（Team 页也读同一棵树），Agent 与团队各读一次列表。
 */
export function useOrgChart() {
  const $q = useQuasar();
  const { t } = useI18n();
  const router = useRouter();
  const platformStore = usePlatformStore();

  const loading = ref(false);
  const loadError = ref('');
  const forbidden = ref(false);
  const saving = ref(false);
  const agents = ref<Agent[]>([]);
  const teams = ref<Team[]>([]);
  /** Agent / 团队读不到时只少了挂岗与团队信息，组织树照常可用。 */
  const sideError = ref('');
  const keyword = ref('');
  const selection = reactive<Selection>(readSelection());
  const inspector = ref<OrgInspector>({ mode: 'closed' });
  const form = reactive<OrgNodeForm>({ name: '', description: '', enabled: true });

  const tree = computed(() => platformStore.taxonomyTree);
  const q = computed(() => keyword.value.trim().toLowerCase());
  const byNode = computed(() => agentsByNode(agents.value));
  const byDept = computed(() => teamsByDepartment(teams.value, agents.value, tree.value));

  const companies = computed(() =>
    companiesOf(tree.value).map((node) => ({
      node,
      counts: companyCounts(node, byNode.value, byDept.value),
      matches: q.value ? companyMatchCount(node, q.value, byNode.value) : null,
    })),
  );
  const company = computed<OrgNode | null>(
    () => companies.value.find((c) => c.node.id === selection.company)?.node ?? companies.value[0]?.node ?? null,
  );
  const companyLead = computed(() => (company.value ? companyLeadOf(company.value, agents.value) : null));
  const companyStats = computed(() => companies.value.find((c) => c.node.id === company.value?.id)?.counts ?? null);

  const departments = computed(() =>
    departmentsOf(company.value)
      .map((node) => ({
        node,
        office: isOfficeDept(node, company.value),
        counts: deptCounts(node, byNode.value, byDept.value),
        matches: q.value ? deptMatchCount(node, q.value, byNode.value) : null,
      }))
      .filter((d) => d.matches === null || d.matches > 0),
  );
  // 没选部门 = 公司概览（部门卡片）；选中的部门被搜索滤掉时也回到概览
  const dept = computed<OrgNode | null>(
    () => departments.value.find((d) => d.node.id === selection.dept)?.node ?? null,
  );
  /** 关键数点出来的筛选：空缺 / 已停用（概览里筛部门，部门里筛岗位） */
  const statFilter = ref<'' | 'vacant' | 'off'>('');
  const overviewDepts = computed(() =>
    departments.value.filter(
      (d) => !statFilter.value || (statFilter.value === 'vacant' ? d.counts.vacant > 0 : !d.node.enabled),
    ),
  );

  // 编制树：所有公司 → 部门，数字是岗位数（搜索时是命中数），空缺挂琥珀标签
  const railCompanies = computed<TreeRailCompany[]>(() =>
    companies.value.map(({ node, counts, matches }) => ({
      id: node.id,
      name: node.name,
      enabled: node.enabled,
      count: matches ?? counts.positions,
      departments: departmentsOf(node)
        .map((d) => ({
          d,
          c: deptCounts(d, byNode.value, byDept.value),
          m: q.value ? deptMatchCount(d, q.value, byNode.value) : null,
        }))
        .filter(({ m }) => m === null || m > 0)
        .map(({ d, c, m }) => ({
          id: d.id,
          name: d.name,
          enabled: d.enabled,
          count: m ?? c.positions,
          icon: isOfficeDept(d, node) ? 'shield' : undefined,
          tag: c.vacant ? t('orgChart.vacantShort', { n: c.vacant }) : undefined,
          tagTitle: c.vacant ? t('orgChart.vacantCount', { n: c.vacant }) : undefined,
        })),
    })),
  );
  const selectedKey = computed(() => dept.value?.id ?? company.value?.id ?? '');
  const deptIsOffice = computed(() => (dept.value ? isOfficeDept(dept.value, company.value) : false));
  const deptLead = computed(() => (dept.value ? deptLeadOf(dept.value, agents.value, byNode.value) : null));
  const deptTeams = computed(() => (dept.value ? (byDept.value.get(dept.value.id) ?? []) : []));
  const positions = computed(() => {
    const all = positionsOf(dept.value);
    const deptHit = q.value && dept.value && dept.value.name.toLowerCase().includes(q.value);
    return all
      .filter((p) => deptHit || positionMatches(p, q.value, byNode.value))
      .map((node) => ({ node, agents: byNode.value.get(node.id) ?? [] }))
      .filter(
        ({ node, agents: seated }) =>
          !statFilter.value || (statFilter.value === 'vacant' ? !seated.length : !node.enabled),
      );
  });
  const positionTotal = computed(() => positionsOf(dept.value).length);

  // 选中项落到实际存在的节点上并记住（换公司时部门跟着回到该公司的第一个）。
  watch([company, dept], ([c, d]) => {
    const next = { company: c?.id ?? '', dept: d?.id ?? '' };
    if (next.company !== selection.company || next.dept !== selection.dept) Object.assign(selection, next);
    writeSelection(next);
  });

  const inspected = computed<OrgNode | null>(() => {
    const ins = inspector.value;
    return ins.mode === 'view' || ins.mode === 'edit' ? findNode(tree.value, ins.nodeId) : null;
  });
  const inspectedAgents = computed(() => (inspected.value ? (byNode.value.get(inspected.value.id) ?? []) : []));
  const inspectedLevel = computed<TaxonomyLevel>(() => {
    const ins = inspector.value;
    if (ins.mode === 'create') return ins.level;
    return (inspected.value?.level as TaxonomyLevel) || 'position';
  });
  const inspectorParent = computed<OrgNode | null>(() => {
    const ins = inspector.value;
    if (ins.mode === 'create') return ins.parentId ? findNode(tree.value, ins.parentId) : null;
    return inspected.value?.parent_id ? findNode(tree.value, inspected.value.parent_id) : null;
  });

  function fail(err: unknown, fallbackKey: string) {
    const message = parseKratosApiError(err).message || t(fallbackKey);
    $q.notify({ type: 'negative', message });
  }

  async function load(opts?: { silent?: boolean }) {
    if (!opts?.silent) loading.value = true;
    loadError.value = '';
    forbidden.value = false;
    sideError.value = '';
    const side = Promise.allSettled([listAgents({ limit: TEAM_AGENT_LIST_LIMIT }), listTeams()]);
    try {
      await platformStore.loadTaxonomyTree(RESOURCE);
    } catch (err) {
      forbidden.value = isForbiddenApiError(err);
      loadError.value = parseKratosApiError(err).message || t('orgChart.loadFailed');
    }
    const [agentRes, teamRes] = await side;
    if (agentRes.status === 'fulfilled') agents.value = agentRes.value;
    if (teamRes.status === 'fulfilled') teams.value = teamRes.value;
    if (agentRes.status === 'rejected' || teamRes.status === 'rejected') sideError.value = t('orgChart.sidePartial');
    if (!opts?.silent) loading.value = false;
  }

  onMounted(() => void load());

  function selectCompany(id: string) {
    selection.company = id;
    selection.dept = '';
    statFilter.value = '';
    if (inspector.value.mode !== 'closed') inspector.value = { mode: 'closed' };
  }

  /** 编制树可以直接点别家公司的部门：公司跟着切过去 */
  function selectDept(id: string) {
    const parent = findNode(tree.value, id)?.parent_id;
    if (parent && parent !== selection.company) selection.company = parent;
    selection.dept = id;
    statFilter.value = '';
  }

  function onRailSelect(key: string) {
    if (companiesOf(tree.value).some((c) => c.id === key)) selectCompany(key);
    else selectDept(key);
  }

  function toggleStatFilter(key: string) {
    statFilter.value = statFilter.value === key ? '' : (key as 'vacant' | 'off');
  }

  function inspect(node: OrgNode) {
    inspector.value = { mode: 'view', nodeId: node.id };
  }

  function openEdit(node: OrgNode) {
    Object.assign(form, { name: node.name, description: node.description || '', enabled: node.enabled });
    inspector.value = { mode: 'edit', nodeId: node.id };
  }

  function openCreate(level: TaxonomyLevel, parent: OrgNode | null) {
    Object.assign(form, { name: '', description: '', enabled: true });
    inspector.value = { mode: 'create', level, parentId: parent?.id ?? '' };
  }

  function closeInspector() {
    inspector.value = { mode: 'closed' };
  }

  async function save() {
    const name = form.name.trim();
    if (!name) {
      $q.notify({ type: 'warning', message: t('orgChart.nameRequired') });
      return;
    }
    const ins = inspector.value;
    saving.value = true;
    try {
      if (ins.mode === 'edit') {
        await platformStore.editResource(RESOURCE, ins.nodeId, {
          name,
          description: form.description.trim(),
          enabled: form.enabled,
        });
        await load({ silent: true });
        inspector.value = { mode: 'view', nodeId: ins.nodeId };
      } else if (ins.mode === 'create') {
        const parent = ins.parentId ? findNode(tree.value, ins.parentId) : null;
        const siblings = parent ? (parent.children ?? []) : companiesOf(tree.value);
        const payload: PlatformResourceInput = {
          key: buildTaxonomyKey(ins.level, name, ins.parentId),
          name,
          description: form.description.trim(),
          enabled: form.enabled,
          sort_order: nextTaxonomySortOrder(siblings),
          parent_id: ins.parentId,
          level: ins.level,
          config_json: '{}',
          metadata_json: '{}',
        };
        const created = await platformStore.addResource(RESOURCE, payload);
        await load({ silent: true });
        if (ins.level === 'company') selectCompany(created.id);
        if (ins.level === 'department') selectDept(created.id);
        inspector.value = { mode: 'view', nodeId: created.id };
      }
      $q.notify({ type: 'positive', message: t('orgChart.saved') });
    } catch (err) {
      fail(err, 'orgChart.saveFailed');
    } finally {
      saving.value = false;
    }
  }

  async function toggleEnabled(node: OrgNode) {
    if (parseIsSystem(node)) {
      $q.notify({ type: 'warning', message: t('organizationPage.systemNodeToggleDisabled') });
      return;
    }
    try {
      await platformStore.editResource(RESOURCE, node.id, { enabled: !node.enabled });
      await load({ silent: true });
      $q.notify({ type: 'positive', message: t(node.enabled ? 'orgChart.disabled' : 'orgChart.enabled') });
    } catch (err) {
      fail(err, 'orgChart.saveFailed');
    }
  }

  /** 部门的同级（不含总经理办公室，它固定在最前）或岗位的同级。 */
  function movableSiblings(node: OrgNode): OrgNode[] {
    const parent = findNode(tree.value, node.parent_id);
    if (node.level === 'position') return positionsOf(parent);
    if (node.level === 'department') return departmentsOf(parent).filter((d) => !isOfficeDept(d, parent));
    return companiesOf(tree.value);
  }

  function canMove(node: OrgNode, delta: -1 | 1) {
    return moveSibling(movableSiblings(node), node.id, delta) !== null;
  }

  async function move(node: OrgNode, delta: -1 | 1) {
    const ids = moveSibling(movableSiblings(node), node.id, delta);
    if (!ids) return;
    try {
      await platformStore.reorderOrganizationNodes(ids);
      await load({ silent: true });
    } catch (err) {
      fail(err, 'orgChart.reorderFailed');
    }
  }

  function deleteMessage(node: OrgNode): string {
    const impact = deleteImpact(node, byNode.value, byDept.value);
    if (node.level === 'position') {
      const seated = (byNode.value.get(node.id) ?? []).map((a) => a.display_name).join('、');
      return seated
        ? t('orgChart.deletePositionSeated', { name: node.name, agents: seated })
        : t('orgChart.deletePositionEmpty', { name: node.name });
    }
    return t(node.level === 'company' ? 'orgChart.deleteCompanyImpact' : 'orgChart.deleteDeptImpact', {
      name: node.name,
      ...impact,
    });
  }

  async function remove(node: OrgNode) {
    if (parseIsSystem(node)) {
      $q.notify({ type: 'warning', message: t('orgChart.systemNodeDelete') });
      return;
    }
    // 后端会拒绝：先说清楚是哪几个团队挡着，不让用户确认完才看到英文报错。
    const blocking = blockingTeams(node, byDept.value);
    if (blocking.length) {
      $q.notify({ type: 'warning', message: t('orgChart.deleteBlocked', { teams: blocking.join('、') }) });
      return;
    }
    const ok = await confirm({
      title: t('orgChart.deleteTitle', { level: t(`orgChart.level.${node.level}`) }),
      message: deleteMessage(node),
      confirmLabel: t('orgChart.delete'),
      cancelLabel: t('orgChart.cancel'),
      tone: 'danger',
    });
    if (!ok) return;
    try {
      await platformStore.removeResource(RESOURCE, node.id);
      if (inspected.value?.id === node.id) closeInspector();
      await load({ silent: true });
      $q.notify({ type: 'positive', message: t('orgChart.deleted', { name: node.name }) });
    } catch (err) {
      fail(err, 'orgChart.deleteFailed');
    }
  }

  function openAgent(agent: Agent) {
    void router.push({ name: 'agent-settings', params: { id: agent.id } });
  }

  function openTeams(node: OrgNode, teamId?: string) {
    void router.push({ name: 'team', query: { department: node.id, ...(teamId ? { team: teamId } : {}) } });
  }

  return {
    loading,
    loadError,
    forbidden,
    sideError,
    saving,
    keyword,
    companies,
    company,
    companyLead,
    companyStats,
    departments,
    dept,
    deptIsOffice,
    deptLead,
    deptTeams,
    positions,
    positionTotal,
    inspector,
    inspected,
    inspectedAgents,
    inspectedLevel,
    inspectorParent,
    form,
    agents,
    byNode,
    byDept,
    load,
    selectCompany,
    selectDept,
    onRailSelect,
    railCompanies,
    selectedKey,
    statFilter,
    toggleStatFilter,
    overviewDepts,
    inspect,
    openEdit,
    openCreate,
    closeInspector,
    save,
    toggleEnabled,
    canMove,
    move,
    remove,
    openAgent,
    openTeams,
    isSystem: parseIsSystem,
  };
}
