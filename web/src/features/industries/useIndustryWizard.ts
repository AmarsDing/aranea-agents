import { ref, computed, watch } from "vue"
// TECH-DEBT: direct API calls; acceptable for wizard composable scoped to create dialog — issue #industry-wizard
import { listIndustries, listDepartments, listPositions, getPositionPrompt, listPositionVariants } from "./api"
import type { Industry, Department, Position, PositionPromptResult } from "./types"

const VARIANT_LABELS: Record<string, string> = {
  general: "通用",
  code_review: "代码审查",
  architect: "架构设计",
  drafting: "正文起草",
  polishing: "润色修饰",
  data_driven: "数据驱动",
  ghostwriting: "代笔",
  factor: "因子研究",
  backtest: "回测执行",
  portfolio: "组合构建",
  ml_alpha: "ML Alpha",
  gameplay: "Gameplay",
  performance: "性能优化",
  network: "网络同步",
  ux_auditor: "UX 审计",
  type_system: "类型系统",
  migration: "迁移",
  optimization: "优化",
  audit: "审计",
  implementation: "实现",
  outline: "大纲设计",
  pacing: "节奏控制",
  creation: "角色创建",
  consistency: "一致性维护",
  review: "审核",
  compliance: "合规",
  engagement: "互动运营",
  planning: "策划",
  storyboard: "分镜",
  scriptwriting: "脚本编写",
  platform_adapt: "平台适配",
  editing: "剪辑",
  effects: "特效",
  template: "模板",
  motion: "动效",
  branding: "品牌",
  design: "设计",
  execution_algo: "执行算法",
  market_making: "做市",
  kernel_tuning: "内核调优",
  network_opt: "网络优化",
  research_platform: "研究平台",
  data_pipeline: "数据管道",
  trading_system: "交易系统",
  operations: "运维",
  premarket: "盘前",
  intraday: "盘中",
  bond_analysis: "债券分析",
  credit_rating: "信用评级",
  futures_strategy: "期货策略",
  options_pricing: "期权定价",
  regulatory: "合规审查",
  market_risk: "市场风险",
  credit_risk: "信用风险",
  anti_money_laundering: "反洗钱",
  strategic_allocation: "战略配置",
  client_profiling: "客户画像",
  portfolio_advice: "投资建议",
  wealth_profiling: "财富画像",
  product_design: "产品设计",
  crypto_analysis: "加密分析",
  hosting: "主持",
  interaction: "互动",
  script: "脚本",
  analytics: "分析",
  content: "内容",
  growth: "增长",
  planting: "种草",
  seo: "SEO",
  strategy: "策略",
  revenue: "变现",
  adapt: "适配",
  sync: "同步",
  course: "课程",
  magic_system: "魔法体系",
  geography_history: "地理历史",
}

export function useIndustryWizard() {
  const industries = ref<Industry[]>([])
  const departments = ref<Department[]>([])
  const positions = ref<Position[]>([])
  const selectedIndustryKey = ref("")
  const selectedDepartmentKey = ref("")
  const selectedPositionKey = ref("")
  const selectedVariant = ref("general")
  const promptResult = ref<PositionPromptResult | null>(null)
  const loadingIndustries = ref(false)
  const loadingDepartments = ref(false)
  const loadingPositions = ref(false)
  const loadingPrompt = ref(false)
  const loadingVariants = ref(false)
  const variantList = ref<string[]>([])

  async function loadIndustries() {
    loadingIndustries.value = true
    try {
      const result = await listIndustries()
      industries.value = result.items
    } finally {
      loadingIndustries.value = false
    }
  }

  async function loadDepartments() {
    if (!selectedIndustryKey.value) { departments.value = []; return }
    loadingDepartments.value = true
    try {
      const result = await listDepartments(selectedIndustryKey.value)
      departments.value = result.items
    } finally {
      loadingDepartments.value = false
    }
  }

  async function loadPositions() {
    if (!selectedDepartmentKey.value) { positions.value = []; return }
    loadingPositions.value = true
    try {
      const result = await listPositions(selectedIndustryKey.value, selectedDepartmentKey.value)
      positions.value = result.items
    } finally {
      loadingPositions.value = false
    }
  }

  async function loadPrompt() {
    if (!selectedIndustryKey.value || !selectedPositionKey.value) return
    loadingPrompt.value = true
    try {
      promptResult.value = await getPositionPrompt(
        selectedIndustryKey.value,
        selectedPositionKey.value,
        selectedVariant.value
      )
    } finally {
      loadingPrompt.value = false
    }
  }

  async function loadVariants() {
    if (!selectedIndustryKey.value || !selectedPositionKey.value) {
      variantList.value = ["general"]
      return
    }
    loadingVariants.value = true
    try {
      variantList.value = await listPositionVariants(selectedIndustryKey.value, selectedPositionKey.value)
    } catch {
      variantList.value = ["general"]
    } finally {
      loadingVariants.value = false
    }
  }

  watch(selectedIndustryKey, () => {
    selectedDepartmentKey.value = ""
    selectedPositionKey.value = ""
    selectedVariant.value = "general"
    promptResult.value = null
    departments.value = []
    positions.value = []
    variantList.value = ["general"]
    loadDepartments()
  })

  watch(selectedDepartmentKey, () => {
    selectedPositionKey.value = ""
    selectedVariant.value = "general"
    promptResult.value = null
    positions.value = []
    variantList.value = ["general"]
    loadPositions()
  })

  watch(selectedPositionKey, () => {
    selectedVariant.value = "general"
    promptResult.value = null
    loadVariants()
  })

  watch([selectedPositionKey, selectedVariant], () => {
    promptResult.value = null
    loadPrompt()
  })

  const availableVariants = computed(() => {
    if (!selectedPositionKey.value) return []
    return variantList.value.map(v => ({
      label: VARIANT_LABELS[v] ?? v,
      value: v,
    }))
  })

  const selectedIndustry = computed(() => industries.value.find(i => i.key === selectedIndustryKey.value))
  const selectedDepartment = computed(() => departments.value.find(d => d.key === selectedDepartmentKey.value))
  const selectedPosition = computed(() => positions.value.find(p => p.key === selectedPositionKey.value))

  function reset() {
    selectedIndustryKey.value = ""
    selectedDepartmentKey.value = ""
    selectedPositionKey.value = ""
    selectedVariant.value = "general"
    promptResult.value = null
    variantList.value = ["general"]
  }

  return {
    industries, departments, positions,
    selectedIndustryKey, selectedDepartmentKey, selectedPositionKey, selectedVariant,
    promptResult,
    loadingIndustries, loadingDepartments, loadingPositions, loadingPrompt, loadingVariants,
    availableVariants,
    selectedIndustry, selectedDepartment, selectedPosition,
    loadIndustries, reset,
  }
}
