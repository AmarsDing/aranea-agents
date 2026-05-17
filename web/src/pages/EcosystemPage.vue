<template>
  <q-page class="app-page-cream ecosystem-page q-pa-md">
    <section class="ecosystem-hero q-mb-md">
      <div class="ecosystem-hero__content">
        <q-chip dense color="primary" text-color="white" icon="storefront">Agent Commerce</q-chip>
        <h1 class="ecosystem-title">生态商城</h1>
        <p class="ecosystem-subtitle">
          面向 Agent、Skill、Team 的交易平台：发现可复用能力，评估可信度，一键安装到当前工作区。
        </p>
        <div class="row q-gutter-sm q-mt-md">
          <q-btn unelevated rounded color="primary" icon="shopping_bag" label="浏览商品" @click="scrollToCatalog" />
          <q-btn outline rounded color="primary" icon="publish" label="发布能力" @click="publishOpen = true" />
        </div>
      </div>
      <q-card flat bordered class="ecosystem-hero-card">
        <q-card-section>
          <div class="text-caption text-grey-7">Marketplace GMV</div>
          <div class="text-h4 text-weight-bold q-mt-xs">¥128,400</div>
          <div class="text-caption text-positive q-mt-xs">+24% 本月</div>
        </q-card-section>
        <q-separator />
        <q-card-section class="row q-col-gutter-sm">
          <div v-for="metric in heroMetrics" :key="metric.label" class="col-4">
            <div class="text-caption text-grey-7">{{ metric.label }}</div>
            <div class="text-subtitle1 text-weight-bold">{{ metric.value }}</div>
          </div>
        </q-card-section>
      </q-card>
    </section>

    <div class="row q-col-gutter-md q-mb-md">
      <div v-for="item in categoryCards" :key="item.type" class="col-12 col-md-4">
        <q-card flat bordered class="ecosystem-category-card cursor-pointer" @click="activeType = item.type">
          <q-card-section class="row items-start q-gutter-md no-wrap">
            <q-avatar :color="item.color" text-color="white" :icon="item.icon" />
            <div class="col">
              <div class="row items-center justify-between">
                <div class="text-subtitle1 text-weight-bold">{{ item.title }}</div>
                <q-badge outline :color="item.color">{{ item.count }}</q-badge>
              </div>
              <div class="text-caption text-grey-7 q-mt-xs">{{ item.caption }}</div>
            </div>
          </q-card-section>
        </q-card>
      </div>
    </div>

    <div class="row q-col-gutter-md">
      <div class="col-12 col-lg-8">
        <q-card ref="catalogRef" flat bordered class="ecosystem-catalog-card">
          <q-card-section class="row q-col-gutter-sm items-center">
            <div class="col-12 col-md-5">
              <q-input v-model="keyword" dense outlined clearable debounce="250" placeholder="搜索 Agent、Skill、Team">
                <template #prepend><q-icon name="search" /></template>
              </q-input>
            </div>
            <div class="col-12 col-sm-6 col-md-3">
              <q-btn-toggle
                v-model="activeType"
                spread
                no-caps
                rounded
                unelevated
                toggle-color="primary"
                :options="typeOptions"
              />
            </div>
            <div class="col-12 col-sm-6 col-md-2">
              <q-select v-model="pricing" dense outlined emit-value map-options label="价格" :options="pricingOptions" />
            </div>
            <div class="col-12 col-md-2">
              <q-select v-model="sortBy" dense outlined emit-value map-options label="排序" :options="sortOptions" />
            </div>
          </q-card-section>
          <q-separator />
          <q-card-section>
            <div class="row q-col-gutter-md">
              <div v-for="product in filteredProducts" :key="product.id" class="col-12 col-md-6">
                <q-card flat bordered class="market-product-card">
                  <q-card-section>
                    <div class="row items-start q-gutter-sm no-wrap">
                      <q-avatar :color="typeMeta(product.type).color" text-color="white" :icon="typeMeta(product.type).icon" />
                      <div class="col">
                        <div class="row items-center q-gutter-xs">
                          <div class="text-subtitle1 text-weight-bold ellipsis">{{ product.name }}</div>
                          <q-icon v-if="product.verified" name="verified" color="primary" size="18px" />
                        </div>
                        <div class="text-caption text-grey-7">{{ product.creator }} · {{ typeMeta(product.type).label }}</div>
                      </div>
                      <q-chip dense :color="product.price === 0 ? 'positive' : 'amber-8'" text-color="white">
                        {{ product.price === 0 ? "免费" : `¥${product.price}` }}
                      </q-chip>
                    </div>

                    <div class="text-body2 q-mt-md product-description">{{ product.description }}</div>

                    <div class="row q-gutter-xs q-mt-md">
                      <q-chip v-for="tag in product.tags" :key="tag" dense outline color="primary">{{ tag }}</q-chip>
                    </div>

                    <div class="row items-center justify-between q-mt-md">
                      <div class="row items-center q-gutter-md text-caption text-grey-7">
                        <span><q-icon name="star" color="amber" /> {{ product.rating }}</span>
                        <span><q-icon name="download" /> {{ product.installs }}</span>
                        <span><q-icon name="security" /> {{ product.trustScore }}</span>
                      </div>
                      <q-btn dense rounded unelevated color="primary" label="查看" @click="openProduct(product)" />
                    </div>
                  </q-card-section>
                </q-card>
              </div>
            </div>
          </q-card-section>
        </q-card>
      </div>

      <div class="col-12 col-lg-4">
        <div class="column q-gutter-md">
          <q-card flat bordered class="ecosystem-side-card">
            <q-card-section>
              <div class="text-subtitle1 text-weight-bold">交易治理</div>
              <div class="text-body2 text-grey-7 q-mt-sm">
                商城商品以安装包和版本为核心，发布前经过权限声明、运行沙箱、签名校验和人工审核。
              </div>
            </q-card-section>
            <q-list dense separator>
              <q-item v-for="policy in policies" :key="policy.title">
                <q-item-section avatar><q-icon :name="policy.icon" color="primary" /></q-item-section>
                <q-item-section>
                  <q-item-label>{{ policy.title }}</q-item-label>
                  <q-item-label caption>{{ policy.caption }}</q-item-label>
                </q-item-section>
              </q-item>
            </q-list>
          </q-card>

          <q-card flat bordered class="ecosystem-side-card">
            <q-card-section>
              <div class="text-subtitle1 text-weight-bold">本周榜单</div>
            </q-card-section>
            <q-list>
              <q-item v-for="(item, index) in topProducts" :key="item.id" clickable @click="openProduct(item)">
                <q-item-section avatar>
                  <q-avatar color="primary" text-color="white" size="32px">{{ index + 1 }}</q-avatar>
                </q-item-section>
                <q-item-section>
                  <q-item-label>{{ item.name }}</q-item-label>
                  <q-item-label caption>{{ item.installs }} installs · {{ item.rating }} rating</q-item-label>
                </q-item-section>
              </q-item>
            </q-list>
          </q-card>

          <q-card flat bordered class="ecosystem-side-card publish-card">
            <q-card-section>
              <div class="text-subtitle1 text-weight-bold">发布流程</div>
              <q-timeline color="primary" layout="dense" class="q-mt-sm">
                <q-timeline-entry v-for="step in publishSteps" :key="step.title" :title="step.title" :subtitle="step.caption" />
              </q-timeline>
            </q-card-section>
          </q-card>
        </div>
      </div>
    </div>

    <q-dialog v-model="detailOpen">
      <q-card class="product-detail-card">
        <q-card-section v-if="selectedProduct" class="row items-start justify-between q-gutter-md">
          <div class="row items-start q-gutter-md">
            <q-avatar :color="typeMeta(selectedProduct.type).color" text-color="white" :icon="typeMeta(selectedProduct.type).icon" size="52px" />
            <div>
              <div class="text-h6">{{ selectedProduct.name }}</div>
              <div class="text-caption text-grey-7">{{ selectedProduct.creator }} · {{ typeMeta(selectedProduct.type).label }}</div>
            </div>
          </div>
          <q-btn flat dense round icon="close" aria-label="关闭详情" v-close-popup />
        </q-card-section>
        <q-separator />
        <q-card-section v-if="selectedProduct" class="q-gutter-md">
          <q-banner rounded class="bg-primary text-white">{{ selectedProduct.description }}</q-banner>
          <div class="row q-col-gutter-sm">
            <div class="col-6"><b>评分：</b>{{ selectedProduct.rating }}</div>
            <div class="col-6"><b>安装：</b>{{ selectedProduct.installs }}</div>
            <div class="col-6"><b>信任分：</b>{{ selectedProduct.trustScore }}</div>
            <div class="col-6"><b>价格：</b>{{ selectedProduct.price === 0 ? "免费" : `¥${selectedProduct.price}` }}</div>
          </div>
          <q-expansion-item default-open label="安装后能力">
            <q-list dense>
              <q-item v-for="capability in selectedProduct.capabilities" :key="capability">
                <q-item-section avatar><q-icon name="check_circle" color="positive" /></q-item-section>
                <q-item-section>{{ capability }}</q-item-section>
              </q-item>
            </q-list>
          </q-expansion-item>
          <div class="row justify-end q-gutter-sm">
            <q-btn flat rounded label="加入收藏" icon="favorite_border" />
            <q-btn unelevated rounded color="primary" icon="download" label="安装到工作区" />
          </div>
        </q-card-section>
      </q-card>
    </q-dialog>

    <q-dialog v-model="publishOpen">
      <q-card class="product-detail-card">
        <q-card-section class="row items-center justify-between">
          <div>
            <div class="text-h6">发布能力到商城</div>
            <div class="text-caption text-grey-7">选择 Agent、Skill 或 Team 后生成可审核的商品包。</div>
          </div>
          <q-btn flat dense round icon="close" aria-label="关闭发布" v-close-popup />
        </q-card-section>
        <q-separator />
        <q-card-section class="q-gutter-md">
          <q-select v-model="publishType" outlined label="商品类型" :options="publishTypeOptions" />
          <q-input v-model="publishName" outlined label="商品名称" />
          <q-input v-model="publishDesc" outlined type="textarea" label="商品描述" />
          <q-banner rounded class="bg-grey-2 text-grey-9">
            后续接入真实后端时，这里会生成版本快照、权限声明、定价方案和审核任务。
          </q-banner>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat label="取消" v-close-popup />
          <q-btn unelevated color="primary" label="提交审核" v-close-popup />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";

type ProductType = "agent" | "skill" | "team";

type MarketProduct = {
  id: string;
  type: ProductType;
  name: string;
  creator: string;
  description: string;
  tags: string[];
  price: number;
  rating: number;
  installs: string;
  trustScore: number;
  verified: boolean;
  capabilities: string[];
};

const catalogRef = ref();
const keyword = ref("");
const activeType = ref<"all" | ProductType>("all");
const pricing = ref("all");
const sortBy = ref("featured");
const detailOpen = ref(false);
const publishOpen = ref(false);
const publishType = ref("");
const publishName = ref("");
const publishDesc = ref("");
const selectedProduct = ref<MarketProduct | null>(null);

const typeOptions = [
  { label: "全部", value: "all" },
  { label: "Agent", value: "agent" },
  { label: "Skill", value: "skill" },
  { label: "Team", value: "team" }
];

const pricingOptions = [
  { label: "全部", value: "all" },
  { label: "免费", value: "free" },
  { label: "付费", value: "paid" }
];

const sortOptions = [
  { label: "精选", value: "featured" },
  { label: "评分", value: "rating" },
  { label: "安装量", value: "installs" }
];

const publishTypeOptions = ["Agent 模板", "Skill 包", "Team 编排"];

const products: MarketProduct[] = [
  {
    id: "agent-growth-writer",
    type: "agent",
    name: "增长文案 Agent",
    creator: "Arenea Studio",
    description: "面向落地页、广告投放和邮件营销的多语种增长文案 Agent，内置 A/B 版本生成策略。",
    tags: ["Marketing", "Copywriting", "多语种"],
    price: 99,
    rating: 4.9,
    installs: "12.4k",
    trustScore: 98,
    verified: true,
    capabilities: ["生成多版本营销文案", "按品牌语气改写", "输出渠道化投放素材"]
  },
  {
    id: "skill-figma-qa",
    type: "skill",
    name: "Figma 设计验收 Skill",
    creator: "Design Ops Lab",
    description: "读取设计稿与前端页面截图，自动检查视觉偏差、间距、颜色和组件一致性。",
    tags: ["Design QA", "Figma", "前端验收"],
    price: 0,
    rating: 4.8,
    installs: "8.7k",
    trustScore: 95,
    verified: true,
    capabilities: ["生成视觉差异报告", "检查设计 token 偏差", "输出修复建议"]
  },
  {
    id: "team-product-launch",
    type: "team",
    name: "新品发布 Team",
    creator: "Launch Guild",
    description: "由市场研究、定位、文案、设计审核和复盘 Agent 组成的新品发布编排团队。",
    tags: ["Team", "Launch", "Orchestration"],
    price: 299,
    rating: 4.7,
    installs: "4.1k",
    trustScore: 92,
    verified: true,
    capabilities: ["自动拆解发布计划", "协调多 Agent 产出", "生成复盘报告"]
  },
  {
    id: "agent-data-analyst",
    type: "agent",
    name: "数据分析 Agent",
    creator: "Metric Forge",
    description: "连接数据源后生成指标解释、异常归因和经营建议，适合运营与管理看板分析。",
    tags: ["Analytics", "BI", "报告"],
    price: 149,
    rating: 4.6,
    installs: "6.9k",
    trustScore: 90,
    verified: false,
    capabilities: ["指标归因", "生成周报", "解释趋势异常"]
  },
  {
    id: "skill-notion-brief",
    type: "skill",
    name: "Notion 知识沉淀 Skill",
    creator: "Workspace Kit",
    description: "将对话、会议、代码变更整理为结构化 Notion 文档，自动补全背景和行动项。",
    tags: ["Notion", "Knowledge", "自动归档"],
    price: 49,
    rating: 4.8,
    installs: "9.2k",
    trustScore: 96,
    verified: true,
    capabilities: ["整理会议纪要", "沉淀项目决策", "生成任务清单"]
  },
  {
    id: "team-code-review",
    type: "team",
    name: "代码评审 Team",
    creator: "DevTools Collective",
    description: "Planner、Reviewer、Test Runner 多角色协作，覆盖代码风险、测试缺口和发布建议。",
    tags: ["Code Review", "CI", "Quality"],
    price: 0,
    rating: 4.9,
    installs: "15.8k",
    trustScore: 97,
    verified: true,
    capabilities: ["并行审查代码", "定位测试缺口", "生成 PR 评审意见"]
  }
];

const heroMetrics = [
  { label: "商品", value: "326" },
  { label: "创作者", value: "84" },
  { label: "安装", value: "57k" }
];

const policies = [
  { icon: "fact_check", title: "审核发布", caption: "版本、描述、权限声明必须可追溯" },
  { icon: "gpp_good", title: "可信安装", caption: "签名包 + 工作区权限确认" },
  { icon: "payments", title: "交易分账", caption: "支持免费、买断、订阅与企业授权" }
];

const publishSteps = [
  { title: "打包", caption: "选择当前工作区能力并生成版本快照" },
  { title: "审核", caption: "检查权限、质量、说明和安全边界" },
  { title: "上架", caption: "展示评分、安装量、版本与收入数据" }
];

const categoryCards = computed(() => [
  {
    type: "agent" as const,
    title: "Agent 模板",
    icon: "smart_toy",
    color: "primary",
    count: products.filter((item) => item.type === "agent").length,
    caption: "可直接安装的行业 Agent、角色提示词与运行配置。"
  },
  {
    type: "skill" as const,
    title: "Skill 包",
    icon: "psychology",
    color: "deep-purple",
    count: products.filter((item) => item.type === "skill").length,
    caption: "封装文件、工具、流程和领域知识的可复用能力。"
  },
  {
    type: "team" as const,
    title: "Team 编排",
    icon: "groups",
    color: "teal",
    count: products.filter((item) => item.type === "team").length,
    caption: "多 Agent 协作拓扑、角色分工和任务编排模板。"
  }
]);

const filteredProducts = computed(() => {
  const q = keyword.value.trim().toLowerCase();
  const rows = products.filter((item) => {
    const matchesType = activeType.value === "all" || item.type === activeType.value;
    const matchesPrice = pricing.value === "all" || (pricing.value === "free" ? item.price === 0 : item.price > 0);
    const matchesKeyword = !q || [item.name, item.creator, item.description, ...item.tags].some((value) => value.toLowerCase().includes(q));
    return matchesType && matchesPrice && matchesKeyword;
  });
  return [...rows].sort((a, b) => {
    if (sortBy.value === "rating") return b.rating - a.rating;
    if (sortBy.value === "installs") return parseInstalls(b.installs) - parseInstalls(a.installs);
    return Number(b.verified) - Number(a.verified) || b.trustScore - a.trustScore;
  });
});

const topProducts = computed(() => [...products].sort((a, b) => b.rating - a.rating).slice(0, 4));

function typeMeta(type: ProductType) {
  if (type === "skill") return { label: "Skill", icon: "psychology", color: "deep-purple" };
  if (type === "team") return { label: "Team", icon: "groups", color: "teal" };
  return { label: "Agent", icon: "smart_toy", color: "primary" };
}

function parseInstalls(value: string) {
  return value.endsWith("k") ? Number(value.replace("k", "")) * 1000 : Number(value);
}

function openProduct(product: MarketProduct) {
  selectedProduct.value = product;
  detailOpen.value = true;
}

function scrollToCatalog() {
  catalogRef.value?.$el?.scrollIntoView({ behavior: "smooth", block: "start" });
}
</script>

<style scoped>
.ecosystem-page {
  min-height: 100%;
}

.ecosystem-hero {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(280px, 380px);
  gap: 20px;
  align-items: stretch;
}

.ecosystem-hero__content {
  padding: clamp(18px, 4vw, 36px);
  border-radius: 28px;
  background: linear-gradient(135deg, rgba(37, 99, 235, 0.12), rgba(20, 184, 166, 0.12));
  border: 1px solid rgba(37, 99, 235, 0.14);
}

.ecosystem-title {
  margin: 12px 0 0;
  font-size: clamp(34px, 5vw, 58px);
  line-height: 1;
  font-weight: 900;
  letter-spacing: -0.04em;
}

.ecosystem-subtitle {
  max-width: 680px;
  margin: 14px 0 0;
  font-size: 1rem;
  color: currentColor;
  opacity: 0.76;
}

.ecosystem-hero-card,
.ecosystem-category-card,
.ecosystem-catalog-card,
.ecosystem-side-card,
.market-product-card {
  height: 100%;
}

.market-product-card {
  transition: transform 0.16s ease, box-shadow 0.16s ease;
}

.market-product-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 14px 34px rgba(37, 99, 235, 0.16);
}

.product-description {
  min-height: 44px;
}

.product-detail-card {
  width: min(760px, 92vw);
}

@media (max-width: 900px) {
  .ecosystem-hero {
    grid-template-columns: 1fr;
  }
}

:global(body.body--dark) .ecosystem-page {
  background: linear-gradient(160deg, #0b1220 0%, #111827 48%, #0f172a 100%);
  color: #e5e7eb;
}

:global(body.body--dark) .ecosystem-hero__content {
  background: linear-gradient(135deg, rgba(37, 99, 235, 0.2), rgba(20, 184, 166, 0.16));
  border-color: rgba(148, 163, 184, 0.16);
}

:global(body.body--dark) .ecosystem-page .q-card {
  background: rgba(17, 24, 39, 0.88) !important;
  border-color: rgba(148, 163, 184, 0.16);
  box-shadow: 0 12px 34px rgba(0, 0, 0, 0.32);
}

:global(body.body--dark) .ecosystem-page .q-field__control {
  background: rgba(30, 41, 59, 0.72);
  border-color: rgba(148, 163, 184, 0.16);
}

:global(body.body--dark) .ecosystem-page .text-grey-7 {
  color: #94a3b8 !important;
}
</style>
