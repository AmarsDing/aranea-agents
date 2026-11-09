package biz

// 本文件实现三项目整合 P1-7 的侧栏菜单目录与可见性裁剪（P0 详设 01 §6：
// `GET /v1/auth/menus` 按角色下发；前端过滤仅展示，后端硬判）。
//
// 目录内容与前端 `web/src/config/sideNav.ts` **逐项对齐**（path/icon/labelKey/
// exact），由 `menu_web_test.go` 的漂移测试守护：任一侧新增/删除条目而另一侧
// 未同步即测试失败。菜单的 8 组重排是 P5 的范围，本阶段保持现网 7 组结构。

// MenuEntry 是侧栏的一个可导航条目（与前端 NavItem 字段一一对应）。
type MenuEntry struct {
	// Key 稳定标识（前端渲染 key / 测试锚点）。
	Key string
	// Path 前端路由路径。
	Path string
	// LabelKey i18n 消息键。
	LabelKey string
	// Icon Material 图标名。
	Icon string
	// Exact 路由是否精确匹配（默认 false = 前缀匹配）。
	Exact bool
	// Permission 可见所需权限码；空 = 仅需登录。
	Permission string
	// Children 子条目（现网侧栏为两层，暂未使用）。
	Children []MenuEntry
}

// MenuGroup 是侧栏的一个分组（与前端 NavGroup 对应）。
type MenuGroup struct {
	// Key 分组稳定标识。
	Key string
	// LabelKey i18n 消息键。
	LabelKey string
	// Items 分组条目。
	Items []MenuEntry
}

// 权限码：本阶段只对**内置角色矩阵已授予**的域打码（pkg/auth/permissions.go
// builtinRolePermissions）。未列入矩阵的域（agent.*/tool.*/graph.*/mcp.*/
// skill.*/plugin.*/hook.*/a2a.*/channel.*/evaluation.*）
// 暂按「仅需登录」下发——打码会让除 superadmin 外的全部角色看不到该菜单
// （矩阵未授予 = 恒不可见），属于开放项而非收严项。待 P5 域授权矩阵落地后
// 逐条补码，届时仅需改动本文件常量与 sideNav.ts。
//
// system.self_improve 已出清单（评审 2026-11-09 A3）：/self-improvement 的
// 全部 rpc 在 permission_registry_gen.go 硬判 system.self_improve，菜单不
// 接码即「可见但全 403」；矩阵已授予 sysadmin（superadmin 经 * 覆盖），
// 打码后菜单可见性与 rpc 判定对齐。
const (
	permKnowledgeRead = "knowledge.read" // /knowledge、/artifacts
	permMemoryRead    = "memory.read"    // /memory
	permUsageRead     = "usage.read"     // /usage/events
	permSystemCron    = "system.cron"    // /cron
	permSystemMonitor = "system.monitor" // /monitor/logs
	permSystemSetting = "system.setting" // /settings
	// permSystemSelfImprove 自我改进控制台（评审 2026-11-09 A3 接码）。
	permSystemSelfImprove = "system.self_improve" // /self-improvement
	// P2-8：经营驾驶舱两页。ceoboard.view 已在内置角色矩阵（pkg/auth/permissions.go：
	// ceo 经 ceoboard.* 通配授予，oncall/viewer 显式授予），打码不收紧可见性。
	permCeoboardView = "ceoboard.view" // /ceo/overview、/ceo/oncall
	// E 阶段 W1a：凭据管理（高敏收窄仅 sysadmin，矩阵见 pkg/auth/permissions.go）。
	permCredentialManage = "credential.manage" // /credentials
)

// NavigationCatalog 返回侧栏菜单目录（显示顺序即数组顺序）。
func NavigationCatalog() []MenuGroup {
	return []MenuGroup{
		{
			Key:      "workspace",
			LabelKey: "menu.groupWorkspace",
			Items: []MenuEntry{
				{Key: "overview", Path: "/overview", Icon: "dashboard", LabelKey: "menu.overview"},
				{Key: "cockpit", Path: "/workbench", Icon: "track_changes", LabelKey: "menu.cockpit"},
				{Key: "ceoOverview", Path: "/ceo/overview", Icon: "monitoring", LabelKey: "menu.ceoOverview", Permission: permCeoboardView},
				{Key: "ceoOncall", Path: "/ceo/oncall", Icon: "engineering", LabelKey: "menu.ceoOncall", Permission: permCeoboardView},
				{Key: "taskWorkspace", Path: "/task-workspace", Icon: "view_quilt", LabelKey: "menu.taskWorkspace"},
				{Key: "usageEvents", Path: "/usage/events", Icon: "receipt_long", LabelKey: "menu.usageEvents", Permission: permUsageRead},
				{Key: "chat", Path: "/chat", Icon: "chat", LabelKey: "menu.chat"},
				{Key: "codingTasks", Path: "/coding-tasks", Icon: "terminal", LabelKey: "menu.codingTasks"},
				{Key: "companion", Path: "/companion", Icon: "record_voice_over", LabelKey: "menu.companion"},
				{Key: "sessions", Path: "/sessions", Icon: "history", LabelKey: "menu.sessions", Exact: false},
				{Key: "memory", Path: "/memory", Icon: "psychology", LabelKey: "menu.memory", Exact: false, Permission: permMemoryRead},
			},
		},
		{
			Key:      "agents",
			LabelKey: "menu.groupAgents",
			Items: []MenuEntry{
				{Key: "agents", Path: "/agents", Icon: "smart_toy", LabelKey: "menu.agents"},
				{Key: "organization", Path: "/settings/organization", Icon: "business", LabelKey: "menu.organization"},
				{Key: "team", Path: "/team", Icon: "groups", LabelKey: "menu.team"},
				{Key: "graphs", Path: "/graphs", Icon: "hub", LabelKey: "menu.graphs"},
			},
		},
		{
			Key:      "models",
			LabelKey: "menu.groupModels",
			Items: []MenuEntry{
				{Key: "models", Path: "/models", Icon: "model_training", LabelKey: "menu.models"},
				{Key: "channels", Path: "/channels", Icon: "hub", LabelKey: "menu.channels"},
			},
		},
		{
			Key:      "tools",
			LabelKey: "menu.groupTools",
			Items: []MenuEntry{
				{Key: "mcp", Path: "/mcp-servers", Icon: "extension", LabelKey: "menu.mcp"},
				{Key: "codingTools", Path: "/settings/coding-tools", Icon: "code", LabelKey: "menu.codingTools"},
				{Key: "tools", Path: "/tools", Icon: "handyman", LabelKey: "menu.tools"},
				{Key: "skills", Path: "/skills", Icon: "psychology", LabelKey: "menu.skills"},
				{Key: "skillTags", Path: "/skills/tags", Icon: "sell", LabelKey: "menu.skillTags"},
				{Key: "evolutionSuggestions", Path: "/skills/evolution-suggestions", Icon: "auto_fix_high", LabelKey: "menu.evolutionSuggestions"},
				{Key: "experienceReports", Path: "/skills/experience-reports", Icon: "assessment", LabelKey: "menu.experienceReports"},
				{Key: "plugins", Path: "/plugins", Icon: "tune", LabelKey: "menu.plugins"},
				{Key: "hooks", Path: "/hooks", Icon: "link", LabelKey: "menu.hooks"},
				{Key: "webhooks", Path: "/webhooks", Icon: "webhook", LabelKey: "menu.webhooks"},
				{Key: "credentials", Path: "/credentials", Icon: "key", LabelKey: "menu.credentials", Permission: permCredentialManage},
				{Key: "a2a", Path: "/a2a", Icon: "swap_horiz", LabelKey: "menu.a2a"},
			},
		},
		{
			Key:      "mediaCommerce",
			LabelKey: "menu.groupMediaCommerce",
			Items: []MenuEntry{
				{Key: "ecommerceLoop", Path: "/ecommerce-loop", Icon: "shopping_bag", LabelKey: "menu.ecommerceLoop"},
				{Key: "contentInbox", Path: "/content-inbox", Icon: "inbox", LabelKey: "menu.contentInbox"},
				{Key: "reviewQueue", Path: "/review-queue", Icon: "rate_review", LabelKey: "menu.reviewQueue"},
				{Key: "opportunityBoard", Path: "/opportunity-board", Icon: "storefront", LabelKey: "menu.opportunityBoard"},
				{Key: "sourcing", Path: "/sourcing", Icon: "travel_explore", LabelKey: "menu.sourcing"},
			},
		},
		{
			Key:      "knowledge",
			LabelKey: "menu.groupKnowledge",
			Items: []MenuEntry{
				{Key: "knowledge", Path: "/knowledge", Icon: "menu_book", LabelKey: "menu.knowledge", Permission: permKnowledgeRead},
				{Key: "artifacts", Path: "/artifacts", Icon: "inventory_2", LabelKey: "menu.artifacts", Permission: permKnowledgeRead},
				{Key: "evaluation", Path: "/evaluation", Icon: "fact_check", LabelKey: "menu.evaluation"},
			},
		},
		{
			Key:      "ops",
			LabelKey: "menu.groupOps",
			Items: []MenuEntry{
				{Key: "cron", Path: "/cron", Icon: "schedule", LabelKey: "menu.cron", Permission: permSystemCron},
				{Key: "monitor", Path: "/monitor/logs", Icon: "monitor_heart", LabelKey: "menu.monitor", Exact: false, Permission: permSystemMonitor},
				{Key: "selfImprovement", Path: "/self-improvement", Icon: "precision_manufacturing", LabelKey: "menu.selfImprovement", Permission: permSystemSelfImprove},
				{Key: "settings", Path: "/settings", Icon: "settings", LabelKey: "menu.settings", Permission: permSystemSetting},
			},
		},
	}
}

// FilterMenuGroups 按 allow 谓词裁剪目录（P1-7）：permission 为空表示「仅需登录」，
// 恒可见；条目全部被裁掉的分组整组省略。allow 为 nil 时视为全不可见（诚实失败：
// 没有主体就没有可下发的菜单，而不是下发全量）。
//
// 返回的切片与条目均为值拷贝，调用方可安全改写而不污染目录。
func FilterMenuGroups(groups []MenuGroup, allow func(code string) bool) []MenuGroup {
	out := make([]MenuGroup, 0, len(groups))
	for _, g := range groups {
		items := make([]MenuEntry, 0, len(g.Items))
		for _, it := range g.Items {
			if it.Permission == "" || (allow != nil && allow(it.Permission)) {
				items = append(items, it)
			}
		}
		if len(items) == 0 {
			continue
		}
		group := g
		group.Items = items
		out = append(out, group)
	}
	return out
}
