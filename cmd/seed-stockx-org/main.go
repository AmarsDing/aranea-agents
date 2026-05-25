// seed-stockx-org seeds Daily Stock Analysis org tree + agent employees into SQLite.
//
// Usage:
//
//	go run ./cmd/seed-stockx-org
//	go run ./cmd/seed-stockx-org --dry-run
//
// Stop `go run ./cmd/admin` before apply (SQLite lock on Windows).
package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/agentcategory"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "print plan only")
	flag.Parse()

	dbPath := resolveSQLitePath()
	fmt.Printf("sqlite: %s\n", dbPath)
	if *dryRun {
		fmt.Println("mode: dry-run")
	}

	ctx := context.Background()
	entClient, rawDB, cleanup, err := data.OpenSQLiteEntClient(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	store := data.NewCLIData(entClient, rawDB)
	catRepo := data.NewAgentCategoryRepo(store)
	agentRepo := data.NewAgentRepo(store)
	catUC := biz.NewAgentCategoryUsecase(catRepo)
	agentUC := biz.NewAgentUsecase(agentRepo, nil, nil)

	plan := buildPlan()
	if *dryRun {
		printPlan(plan)
		return
	}

	idByKey := map[string]string{}
	now := time.Now().UTC().Format(time.RFC3339)

	for _, node := range plan.categories {
		existing, err := findCategoryByKey(ctx, entClient, node.key)
		if err != nil {
			fmt.Fprintf(os.Stderr, "lookup %s: %v\n", node.key, err)
			os.Exit(1)
		}
		if existing != "" {
			idByKey[node.key] = existing
			fmt.Printf("skip category (exists): %s\n", node.key)
			continue
		}
		parentID := ""
		if node.parentKey != "" {
			parentID = idByKey[node.parentKey]
			if parentID == "" {
				fmt.Fprintf(os.Stderr, "missing parent %s for %s\n", node.parentKey, node.key)
				os.Exit(1)
			}
		}
		created, err := catUC.Create(ctx, biz.AgentCategory{
			ID:          newID(),
			Key:         node.key,
			Name:        node.name,
			Description: node.description,
			SortOrder:   node.sortOrder,
			ParentID:    parentID,
			Level:       node.level,
			IsSystem:    true,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "create category %s: %v\n", node.key, err)
			os.Exit(1)
		}
		idByKey[node.key] = created.ID
		fmt.Printf("created category: %s (%s)\n", node.key, created.ID)
	}

	for _, emp := range plan.agents {
		_, err := agentRepo.GetAgentByAgentKey(ctx, emp.agentKey)
		if err == nil {
			fmt.Printf("skip agent (exists): %s\n", emp.agentKey)
			continue
		}
		if err != nil && err != sql.ErrNoRows {
			fmt.Fprintf(os.Stderr, "lookup agent %s: %v\n", emp.agentKey, err)
			os.Exit(1)
		}
		posID := idByKey[emp.positionKey]
		if posID == "" {
			fmt.Fprintf(os.Stderr, "missing position %s for agent %s\n", emp.positionKey, emp.agentKey)
			os.Exit(1)
		}
		created, err := agentUC.Create(ctx, biz.Agent{
			ID:                 newID(),
			AgentKey:           emp.agentKey,
			DisplayName:        emp.displayName,
			Provider:           emp.provider,
			Model:              emp.model,
			AgentDescription:   emp.description,
			CategoryPositionID: posID,
			Status:             "active",
			CreatedBy:          "seed-stockx-org",
			CreatedAt:          now,
			UpdatedAt:          now,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "create agent %s: %v\n", emp.agentKey, err)
			os.Exit(1)
		}
		fmt.Printf("created agent: %s (%s) -> %s\n", emp.agentKey, created.ID, emp.positionKey)
	}

	fmt.Println("done")
}

func resolveSQLitePath() string {
	if v := strings.TrimSpace(os.Getenv("ARANEA_SQLITE_PATH")); v != "" {
		if strings.HasPrefix(v, "file:") {
			return strings.TrimPrefix(v, "file:")
		}
		return v
	}
	wd, _ := os.Getwd()
	return filepath.Join(wd, "cmd", "data", "arenea.sqlite")
}

func findCategoryByKey(ctx context.Context, client *ent.Client, key string) (string, error) {
	row, err := client.AgentCategory.Query().
		Where(
			agentcategory.CategoryKeyEQ(key),
			agentcategory.DeletedAtEQ(""),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}
	return row.ID, nil
}

func newID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

type catNode struct {
	key, name, description, level, parentKey string
	sortOrder                                int
}

type agentSeed struct {
	agentKey, displayName, description, positionKey, provider, model string
}

type seedPlan struct {
	categories []catNode
	agents     []agentSeed
}

func buildPlan() seedPlan {
	// 公司 = industry；部门 = department；岗位 = position（Agent 绑定叶子）
	const (
		companyKey = "stockx-company"
		deptCoord  = "stockx-dept-coordination"
		deptData   = "stockx-dept-data"
		deptResearch = "stockx-dept-research"
		deptOutput = "stockx-dept-output"
	)

	categories := []catNode{
		{
			key:         companyKey,
			name:        "Stockx AI 投研",
			description: "Daily Stock Analysis 场景 — AI 股票分析投研团队",
			level:       "industry",
			sortOrder:   10,
		},
		{
			key:         deptCoord,
			name:        "调度管理部",
			description: "任务拆分、团队调度与报告评审",
			level:       "department",
			parentKey:   companyKey,
			sortOrder:   10,
		},
		{
			key:         deptData,
			name:        "数据采集部",
			description: "行情、财务、资金、新闻等数据统一采集与归一化",
			level:       "department",
			parentKey:   companyKey,
			sortOrder:   20,
		},
		{
			key:         deptResearch,
			name:        "多维分析部",
			description: "技术、基本面、资金、消息、情绪、行业、风险与因子分析",
			level:       "department",
			parentKey:   companyKey,
			sortOrder:   30,
		},
		{
			key:         deptOutput,
			name:        "报告输出部",
			description: "图表构建、报告撰写与多渠道推送",
			level:       "department",
			parentKey:   companyKey,
			sortOrder:   40,
		},
	}

	positions := []struct {
		key, name, dept, desc string
		order                 int
	}{
		{"stockx-pos-coordinator", "主控调度员", deptCoord, "任务拆分、调度成员 Agent、决定是否需要追问", 10},
		{"stockx-pos-critic", "评审员", deptCoord, "对报告草稿打分、提出修改意见", 20},
		{"stockx-pos-data-collector", "数据采集员", deptData, "统一拉取行情/财务/资金/新闻/公告数据，归一化输出", 10},
		{"stockx-pos-technical-analyst", "技术分析师", deptResearch, "K 线形态、均线、MACD/KDJ/RSI、量价关系、趋势/支撑/压力", 10},
		{"stockx-pos-fundamental-analyst", "基本面分析师", deptResearch, "财报、ROE/PE/PB/PEG、盈利质量、行业地位", 20},
		{"stockx-pos-money-flow-analyst", "资金面分析师", deptResearch, "北向资金、龙虎榜、主力净流入、融资融券", 30},
		{"stockx-pos-news-analyst", "消息面分析师", deptResearch, "公司公告、研报、政策、行业新闻、突发事件", 40},
		{"stockx-pos-sentiment-analyst", "情绪面分析师", deptResearch, "雪球/股吧舆情、热度趋势、关键词共现", 50},
		{"stockx-pos-industry-analyst", "行业分析师", deptResearch, "行业景气、产业链上下游、板块轮动、政策驱动", 60},
		{"stockx-pos-risk-assessor", "风险评估师", deptResearch, "波动率、最大回撤、Beta、集中度风险、ST/退市预警", 70},
		{"stockx-pos-quant-factor", "因子计算员", deptResearch, "多因子计算（动量/反转/质量/价值/盈利预期）", 80},
		{"stockx-pos-chart-builder", "图表构建员", deptOutput, "生成 K 线图、财务图表、组合热力图", 10},
		{"stockx-pos-report-writer", "报告撰写员", deptOutput, "把多 Agent 输出汇总为结构化 Markdown / 飞书卡片", 20},
	}
	for _, p := range positions {
		categories = append(categories, catNode{
			key:         p.key,
			name:        p.name,
			description: p.desc,
			level:       "position",
			parentKey:   p.dept,
			sortOrder:   p.order,
		})
	}

	defaultProvider := envOr("STOCKX_SEED_PROVIDER", "openai")
	defaultModel := envOr("STOCKX_SEED_MODEL", "gpt-4o-mini")

	agentDefs := []struct {
		key, name, posKey, desc, provider, model string
	}{
		{"agent-coordinator", "主控调度员", "stockx-pos-coordinator", "Daily Stock Analysis 主控：任务拆分与成员调度", defaultProvider, "gpt-4o"},
		{"agent-critic", "评审员", "stockx-pos-critic", "报告质量评审与修改建议", defaultProvider, defaultModel},
		{"agent-data-collector", "数据采集员", "stockx-pos-data-collector", "拉取并归一化行情/财务/资金/新闻数据", defaultProvider, defaultModel},
		{"agent-technical-analyst", "技术分析师", "stockx-pos-technical-analyst", "技术面：趋势、形态、支撑压力、量价", defaultProvider, defaultModel},
		{"agent-fundamental-analyst", "基本面分析师", "stockx-pos-fundamental-analyst", "基本面：财报、估值、盈利质量", defaultProvider, "gpt-4o"},
		{"agent-money-flow-analyst", "资金面分析师", "stockx-pos-money-flow-analyst", "资金面：北向、龙虎榜、主力流向", defaultProvider, defaultModel},
		{"agent-news-analyst", "消息面分析师", "stockx-pos-news-analyst", "消息面：公告、研报、政策、新闻", defaultProvider, defaultModel},
		{"agent-sentiment-analyst", "情绪面分析师", "stockx-pos-sentiment-analyst", "情绪面：雪球/股吧舆情与热度", defaultProvider, defaultModel},
		{"agent-industry-analyst", "行业分析师", "stockx-pos-industry-analyst", "行业：景气、产业链、板块轮动", defaultProvider, defaultModel},
		{"agent-risk-assessor", "风险评估师", "stockx-pos-risk-assessor", "风险：波动、回撤、Beta、集中度", defaultProvider, defaultModel},
		{"agent-quant-factor", "因子计算员", "stockx-pos-quant-factor", "量化因子：动量/价值/质量/盈利预期", defaultProvider, "gpt-4o"},
		{"agent-chart-builder", "图表构建员", "stockx-pos-chart-builder", "K 线、财务与组合图表生成", defaultProvider, defaultModel},
		{"agent-report-writer", "报告撰写员", "stockx-pos-report-writer", "汇总多维分析并输出 Markdown/飞书卡片", defaultProvider, "gpt-4o"},
	}

	agents := make([]agentSeed, 0, len(agentDefs))
	for _, a := range agentDefs {
		agents = append(agents, agentSeed{
			agentKey:    a.key,
			displayName: a.name,
			description: a.desc,
			positionKey: a.posKey,
			provider:    a.provider,
			model:       a.model,
		})
	}

	return seedPlan{categories: categories, agents: agents}
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func printPlan(plan seedPlan) {
	fmt.Println("--- categories ---")
	for _, c := range plan.categories {
		parent := c.parentKey
		if parent == "" {
			parent = "(root)"
		}
		fmt.Printf("[%s] %s (%s) parent=%s order=%d\n", c.level, c.name, c.key, parent, c.sortOrder)
	}
	fmt.Println("--- agents ---")
	for _, a := range plan.agents {
		fmt.Printf("%s | %s | pos=%s | %s/%s\n", a.agentKey, a.displayName, a.positionKey, a.provider, a.model)
	}
}
