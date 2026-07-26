package biz

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/jsonutil"
	"aranea-agents/pkg/loggateway"
)

// Memory-center aggregation (layer panorama + unified cross-layer graph).
// Design: docs/development/memory/memory.design.md §10.2.
// Kept in its own file so memory_admin_usecase.go does not grow further (AS-COG-01).

const (
	// layerOverviewScanLimit caps how many fact/entity rows are scanned for
	// MVP aggregation (today-added counts, hit_count sums, activity feed).
	layerOverviewScanLimit = 500
	// activityFeedLimit is the max number of merged activity events returned.
	activityFeedLimit = 20

	// unifiedGraphScanLimit caps entity/fact rows loaded for graph assembly.
	unifiedGraphScanLimit = 500
	// unifiedGraphMaxNodes caps BFS expansion (anti hairball).
	unifiedGraphMaxNodes = 150
	// unifiedGraphDefaultHops is the default BFS depth from the focus node.
	unifiedGraphDefaultHops = 2
	// unifiedGraphMaxHops caps user-supplied BFS depth.
	unifiedGraphMaxHops = 4

	// factLinkDefaultWeight is assigned to links_json-derived edges (no DB weight).
	factLinkDefaultWeight = 0.6
	// factSourceEdgeWeight is assigned to fact→episode provenance edges.
	factSourceEdgeWeight = 1.0
)

// MemoryLayerStat is one layer's card data for the memory-center panorama.
type MemoryLayerStat struct {
	Layer        string // L0..L4
	ItemCount    int32
	TodayAdded   int32
	RecallHits   int32
	Health       string // ok | warn
	HeadlineJSON string
}

// MemoryActionItem is a "needs attention" chip on the panorama page.
type MemoryActionItem struct {
	Kind      string // fact_conflict | evolution_pending | context_risk
	Count     int32
	TargetTab string // browse | governance | panorama
}

// MemoryActivityItem is one entry of the recent memory activity feed.
type MemoryActivityItem struct {
	Ts        string
	Kind      string // fact_extracted | episode_recorded | entity_created
	LayerFrom string
	LayerTo   string
	Summary   string
}

// MemoryLayerOverview is the aggregate returned by GetLayerOverview.
type MemoryLayerOverview struct {
	Layers       []MemoryLayerStat
	ActionItems  []MemoryActionItem
	ActivityFeed []MemoryActivityItem
}

// UnifiedGraphNode is a node of the cross-layer memory graph.
type UnifiedGraphNode struct {
	ID       string
	Layer    string // L2 | L3 | L4
	Kind     string // entity | fact | episode
	Label    string
	Weight   float64
	MetaJSON string
}

// UnifiedGraphEdge is an edge of the cross-layer memory graph.
type UnifiedGraphEdge struct {
	Source   string
	Target   string
	Type     string // entity_relation | entity_fact | fact_link | fact_source | fact_conflict
	Label    string
	Weight   float64
	Polarity string // INHIBIT | SUPPORTS | ""
}

// UnifiedMemoryGraph is the aggregate returned by GetUnifiedMemoryGraph.
type UnifiedMemoryGraph struct {
	Focus             string
	Nodes             []UnifiedGraphNode
	Edges             []UnifiedGraphEdge
	FilteredEdgeCount int32
	EmptyReason       string // no_memory_data | focus_not_found | ""
}

// ── Layer overview ────────────────────────────────────────────────────

// GetLayerOverview assembles the five-layer panorama cards, action items and
// the recent memory activity feed for one agent (optionally one session).
func (uc *MemoryAdminUsecase) GetLayerOverview(ctx context.Context, agentID, sessionID string) (*MemoryLayerOverview, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, apierror.BadRequest(apierror.DomainMemory, "agent id is required")
	}
	sessionID = strings.TrimSpace(sessionID)
	todayStart := time.Now().UTC().Truncate(24 * time.Hour)

	ov := &MemoryLayerOverview{}

	// --- L0: latest context assembly (session-scoped) ---
	l0 := MemoryLayerStat{Layer: "L0", Health: "ok"}
	contextUsagePct := 0
	compressStatus := "normal"
	if sessionID != "" {
		l0Rows, err := uc.admin.ListL0SnapshotRows(ctx, sessionID, agentID, 100)
		if err != nil {
			return nil, err
		}
		l0.ItemCount = int32(len(l0Rows))
		for _, raw := range l0Rows {
			m, _ := jsonutil.ParseMap(raw)
			if m == nil {
				continue
			}
			if memCreatedToday(jsonutil.IfaceStr(m, "created_at"), todayStart) {
				l0.TodayAdded++
			}
		}
		if len(l0Rows) > 0 {
			if m, _ := jsonutil.ParseMap(l0Rows[0]); m != nil {
				ratio := jsonutil.IfaceF64(m, "used_ratio")
				contextUsagePct = int(ratio*100 + 0.5)
				compressStatus = compressStatusFromWarnings(jsonutil.IfaceStr(m, "warning_codes_json"))
			}
		}
	}
	if compressStatus != "normal" {
		l0.Health = "warn"
	}
	l0.HeadlineJSON = marshalHeadline(map[string]any{
		"context_usage_pct": contextUsagePct,
		"compress_status":   compressStatus,
	}, uc.lg)

	// --- L1: active working-memory tasks (session-scoped) ---
	l1 := MemoryLayerStat{Layer: "L1", Health: "ok"}
	fieldCount := 0
	if sessionID != "" {
		taskRows, err := uc.admin.ListL1TaskRows(ctx, sessionID, agentID, "active", "")
		if err != nil {
			return nil, err
		}
		l1.ItemCount = int32(len(taskRows))
		for _, raw := range taskRows {
			m, _ := jsonutil.ParseMap(raw)
			if m == nil {
				continue
			}
			if memCreatedToday(jsonutil.IfaceStr(m, "created_at"), todayStart) {
				l1.TodayAdded++
			}
			fields, ferr := uc.admin.ListL1FieldRows(ctx, jsonutil.IfaceStr(m, "id"), false)
			if ferr != nil {
				uc.lg.Warn("L1 field count failed (best-effort)",
					loggateway.StepID("memory.center_l1_fields_fail"),
					loggateway.Err(ferr))
				continue
			}
			fieldCount += len(fields)
		}
	}
	l1.HeadlineJSON = marshalHeadline(map[string]any{
		"active_tasks": l1.ItemCount,
		"field_count":  fieldCount,
	}, uc.lg)

	// --- L2: episodes ---
	l2s := MemoryLayerStat{Layer: "L2", Health: "ok", HeadlineJSON: "{}"}
	var episodeFeedRows [][]byte
	if uc.l2AdminReader != nil {
		rows, total, today, err := uc.l2AdminReader.ListEpisodeRowsAdmin(ctx, agentID, sessionID, activityFeedLimit, 0)
		if err != nil {
			return nil, err
		}
		l2s.ItemCount, l2s.TodayAdded = total, today
		episodeFeedRows = rows
	}

	// --- L3: facts ---
	l3 := MemoryLayerStat{Layer: "L3", Health: "ok"}
	factRows, _, factActive, _, err := uc.admin.ListFactRows(ctx, "agent", agentID, "", "", "", layerOverviewScanLimit, 0)
	if err != nil {
		return nil, err
	}
	l3.ItemCount = factActive
	for _, raw := range factRows {
		m, _ := jsonutil.ParseMap(raw)
		if m == nil {
			continue
		}
		l3.RecallHits += jsonutil.IfaceI32(m, "hit_count")
		if memCreatedToday(jsonutil.IfaceStr(m, "created_at"), todayStart) {
			l3.TodayAdded++
		}
	}
	_, conflictTotal, err := uc.admin.ListConflictingFacts(ctx, "agent", agentID, 1, 0)
	if err != nil {
		return nil, err
	}
	if conflictTotal > 0 {
		l3.Health = "warn"
	}
	l3.HeadlineJSON = marshalHeadline(map[string]any{"conflict_open": conflictTotal}, uc.lg)

	// --- L4: entities + relations ---
	l4s := MemoryLayerStat{Layer: "L4", Health: "ok"}
	entityRows, entityTotal, err := uc.admin.ListEntityRows(ctx, "agent", agentID, "", "", "", "", "", layerOverviewScanLimit, 0)
	if err != nil {
		return nil, err
	}
	l4s.ItemCount = entityTotal
	for _, raw := range entityRows {
		m, _ := jsonutil.ParseMap(raw)
		if m == nil {
			continue
		}
		if memCreatedToday(jsonutil.IfaceStr(m, "created_at"), todayStart) {
			l4s.TodayAdded++
		}
	}
	var relationCount int32
	if uc.l4RelReader != nil {
		relationCount, err = uc.l4RelReader.CountActiveRelations(ctx, "agent", agentID)
		if err != nil {
			return nil, err
		}
	}
	l4s.HeadlineJSON = marshalHeadline(map[string]any{"relation_count": relationCount}, uc.lg)

	ov.Layers = []MemoryLayerStat{l0, l1, l2s, l3, l4s}

	// --- action items ---
	if conflictTotal > 0 {
		ov.ActionItems = append(ov.ActionItems, MemoryActionItem{Kind: "fact_conflict", Count: conflictTotal, TargetTab: "browse"})
	}
	if evoRows, evoErr := uc.admin.EvolutionProposalRows(ctx, agentID, "pending", 100); evoErr != nil {
		uc.lg.Warn("evolution pending count failed (best-effort)",
			loggateway.StepID("memory.center_evolution_fail"),
			loggateway.Err(evoErr))
	} else if len(evoRows) > 0 {
		ov.ActionItems = append(ov.ActionItems, MemoryActionItem{Kind: "evolution_pending", Count: int32(len(evoRows)), TargetTab: "governance"})
	}
	if compressStatus != "normal" {
		ov.ActionItems = append(ov.ActionItems, MemoryActionItem{Kind: "context_risk", Count: 1, TargetTab: "panorama"})
	}

	// --- activity feed: merge latest facts / episodes / entities by created_at ---
	ov.ActivityFeed = buildMemoryActivityFeed(factRows, episodeFeedRows, entityRows, activityFeedLimit)
	return ov, nil
}

func buildMemoryActivityFeed(factRows, episodeRows, entityRows [][]byte, limit int) []MemoryActivityItem {
	type timedItem struct {
		ts   time.Time
		item MemoryActivityItem
	}
	var timed []timedItem
	collect := func(rows [][]byte, kind, from, to string, summaryFn func(map[string]any) string) {
		for _, raw := range rows {
			m, _ := jsonutil.ParseMap(raw)
			if m == nil {
				continue
			}
			tsRaw := jsonutil.IfaceStr(m, "created_at")
			timed = append(timed, timedItem{
				ts: parseMemTime(tsRaw),
				item: MemoryActivityItem{
					Ts: tsRaw, Kind: kind, LayerFrom: from, LayerTo: to,
					Summary: truncateRunes(summaryFn(m), 60),
				},
			})
		}
	}
	collect(factRows, "fact_extracted", "L2", "L3", func(m map[string]any) string { return jsonutil.IfaceStr(m, "statement") })
	collect(episodeRows, "episode_recorded", "L1", "L2", func(m map[string]any) string { return jsonutil.IfaceStr(m, "title") })
	collect(entityRows, "entity_created", "L3", "L4", func(m map[string]any) string { return jsonutil.IfaceStr(m, "name") })

	sort.SliceStable(timed, func(i, j int) bool {
		if timed[i].ts.Equal(timed[j].ts) {
			return timed[i].item.Summary < timed[j].item.Summary
		}
		return timed[i].ts.After(timed[j].ts)
	})
	if limit <= 0 {
		limit = activityFeedLimit
	}
	if len(timed) > limit {
		timed = timed[:limit]
	}
	out := make([]MemoryActivityItem, 0, len(timed))
	for _, t := range timed {
		out = append(out, t.item)
	}
	return out
}

// compressStatusFromWarnings maps L0 warning codes to a compress status enum
// (normal | warning | critical | exceeded).
func compressStatusFromWarnings(warningCodesJSON string) string {
	codes := parseJSONStringSlice(warningCodesJSON)
	status := "normal"
	for _, c := range codes {
		switch strings.TrimSpace(c) {
		case "exceeded":
			return "exceeded"
		case "critical":
			status = "critical"
		case "near_limit":
			if status == "normal" {
				status = "warning"
			}
		}
	}
	return status
}

func marshalHeadline(v map[string]any, lg loggateway.Logger) string {
	return string(safeMarshalJSON(v, lg))
}

// ── Unified cross-layer graph ─────────────────────────────────────────

// GetUnifiedMemoryGraph assembles the cross-layer graph (L4 entities, L3
// facts, L2 episodes) around a focus node. Edge sources:
// memory_relations (classified by endpoint type), facts.links_json,
// facts.source_episode_id. See memory.design.md §10.2 ②.
func (uc *MemoryAdminUsecase) GetUnifiedMemoryGraph(ctx context.Context, agentID, focus string, hops int32, minWeight float64, layers []string) (*UnifiedMemoryGraph, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, err
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, apierror.BadRequest(apierror.DomainMemory, "agent id is required")
	}
	if uc.l2AdminReader == nil || uc.l4RelReader == nil {
		return nil, apierror.Internal(apierror.DomainMemory, "memory center readers not wired")
	}
	if hops <= 0 {
		hops = unifiedGraphDefaultHops
	}
	if hops > unifiedGraphMaxHops {
		hops = unifiedGraphMaxHops
	}
	if minWeight < 0 {
		minWeight = 0
	}
	include := map[string]bool{"L2": true, "L3": true, "L4": true}
	if len(layers) > 0 {
		include = map[string]bool{}
		for _, l := range layers {
			switch strings.ToUpper(strings.TrimSpace(l)) {
			case "L2", "L3", "L4":
				include[strings.ToUpper(strings.TrimSpace(l))] = true
			}
		}
	}

	// --- load node rows ---
	entityByID := map[string]map[string]any{}
	if include["L4"] {
		rows, _, err := uc.admin.ListEntityRows(ctx, "agent", agentID, "", "", "", "", "", unifiedGraphScanLimit, 0)
		if err != nil {
			return nil, err
		}
		for _, raw := range rows {
			m, _ := jsonutil.ParseMap(raw)
			if id := jsonutil.IfaceStr(m, "id"); m != nil && id != "" {
				entityByID[id] = m
			}
		}
	}
	factByID := map[string]map[string]any{}
	if include["L3"] {
		rows, _, _, _, err := uc.admin.ListFactRows(ctx, "agent", agentID, "", "", "", unifiedGraphScanLimit, 0)
		if err != nil {
			return nil, err
		}
		for _, raw := range rows {
			m, _ := jsonutil.ParseMap(raw)
			if id := jsonutil.IfaceStr(m, "id"); m != nil && id != "" {
				factByID[id] = m
			}
		}
	}

	// --- assemble edges ---
	type graphEdge struct {
		UnifiedGraphEdge
		fromRelation bool
	}
	var edges []graphEdge
	seen := map[string]bool{}
	addEdge := func(source, target, typ, label string, weight float64, polarity string, fromRel bool) {
		key := source + "|" + target + "|" + typ
		if seen[key] {
			return
		}
		seen[key] = true
		edges = append(edges, graphEdge{
			UnifiedGraphEdge: UnifiedGraphEdge{Source: source, Target: target, Type: typ, Label: label, Weight: weight, Polarity: polarity},
			fromRelation:     fromRel,
		})
	}

	if include["L4"] || include["L3"] {
		relRows, err := uc.l4RelReader.ListActiveRelationRows(ctx, "agent", agentID)
		if err != nil {
			return nil, err
		}
		for _, raw := range relRows {
			m, _ := jsonutil.ParseMap(raw)
			if m == nil {
				continue
			}
			src := jsonutil.IfaceStr(m, "source_id")
			tgt := jsonutil.IfaceStr(m, "target_id")
			relType := jsonutil.IfaceStr(m, "relation_type")
			weight := jsonutil.IfaceF64(m, "weight")
			if src == "" || tgt == "" {
				continue
			}
			_, srcEnt := entityByID[src]
			_, tgtEnt := entityByID[tgt]
			_, srcFact := factByID[src]
			_, tgtFact := factByID[tgt]
			polarity := ""
			if relType == "INHIBIT" || relType == "SUPPORTS" {
				polarity = relType
			}
			switch {
			case srcEnt && tgtEnt:
				addEdge(src, tgt, "entity_relation", relType, weight, polarity, true)
			case (srcEnt && tgtFact) || (srcFact && tgtEnt):
				addEdge(src, tgt, "entity_fact", relType, weight, polarity, true)
			case srcFact && tgtFact:
				if relType == "INHIBIT" {
					addEdge(src, tgt, "fact_conflict", relType, weight, "INHIBIT", true)
				} else {
					addEdge(src, tgt, "fact_link", relType, weight, polarity, true)
				}
			default:
				// endpoint does not resolve to a loaded node → drop
			}
		}
	}

	// fact_link edges from A-MEM links_json
	if include["L3"] {
		for id, m := range factByID {
			for _, tgt := range parseJSONStringSlice(jsonutil.IfaceStr(m, "links")) {
				if _, ok := factByID[tgt]; ok {
					addEdge(id, tgt, "fact_link", "", factLinkDefaultWeight, "", false)
				}
			}
		}
	}

	// fact_source edges + episode nodes
	episodeByID := map[string]map[string]any{}
	if include["L2"] && include["L3"] {
		var epIDs []string
		for _, m := range factByID {
			if epID := jsonutil.IfaceStr(m, "source_episode_id"); epID != "" {
				epIDs = append(epIDs, epID)
			}
		}
		epRows, err := uc.l2AdminReader.ListEpisodeRowsByIDs(ctx, epIDs)
		if err != nil {
			return nil, err
		}
		for _, raw := range epRows {
			m, _ := jsonutil.ParseMap(raw)
			if id := jsonutil.IfaceStr(m, "id"); m != nil && id != "" {
				episodeByID[id] = m
			}
		}
		for fid, m := range factByID {
			epID := jsonutil.IfaceStr(m, "source_episode_id")
			if _, ok := episodeByID[epID]; ok {
				addEdge(fid, epID, "fact_source", "", factSourceEdgeWeight, "", false)
			}
		}
	}

	// --- build node objects ---
	nodeByID := map[string]*UnifiedGraphNode{}
	for id, m := range entityByID {
		nodeByID[id] = &UnifiedGraphNode{
			ID: id, Layer: "L4", Kind: "entity",
			Label:  jsonutil.IfaceStr(m, "name"),
			Weight: float64(jsonutil.IfaceI32(m, "use_count")),
			MetaJSON: marshalHeadline(map[string]any{
				"entity_type": jsonutil.IfaceStr(m, "entity_type"),
				"confidence":  jsonutil.IfaceF64(m, "confidence"),
			}, uc.lg),
		}
	}
	for id, m := range factByID {
		hits := jsonutil.IfaceI32(m, "hit_count")
		nodeByID[id] = &UnifiedGraphNode{
			ID: id, Layer: "L3", Kind: "fact",
			Label:  truncateRunes(jsonutil.IfaceStr(m, "statement"), 40),
			Weight: float64(hits),
			MetaJSON: marshalHeadline(map[string]any{
				"statement":  jsonutil.IfaceStr(m, "statement"),
				"confidence": jsonutil.IfaceF64(m, "confidence"),
				"hit_count":  hits,
			}, uc.lg),
		}
	}
	for id, m := range episodeByID {
		nodeByID[id] = &UnifiedGraphNode{
			ID: id, Layer: "L2", Kind: "episode",
			Label:  jsonutil.IfaceStr(m, "title"),
			Weight: 1,
			MetaJSON: marshalHeadline(map[string]any{
				"happened_at": jsonutil.IfaceStr(m, "created_at"),
				"summary":     jsonutil.IfaceStr(m, "outcome_summary"),
			}, uc.lg),
		}
	}

	// --- focus resolution ---
	focus = strings.TrimSpace(focus)
	if focus != "" {
		if _, ok := nodeByID[focus]; !ok {
			return &UnifiedMemoryGraph{EmptyReason: "focus_not_found"}, nil
		}
	} else {
		if top, topErr := uc.l4RelReader.TopConnectedEntityID(ctx, "agent", agentID); topErr != nil {
			uc.lg.Warn("TopConnectedEntityID failed (falling back to heaviest node)",
				loggateway.StepID("memory.center_top_focus_fail"),
				loggateway.Err(topErr))
		} else if _, ok := nodeByID[top]; ok && top != "" {
			focus = top
		}
		if focus == "" {
			focus = heaviestGraphNode(nodeByID)
		}
	}
	if focus == "" {
		return &UnifiedMemoryGraph{EmptyReason: "no_memory_data"}, nil
	}

	// --- BFS expansion (undirected) ---
	adj := map[string][]string{}
	for _, e := range edges {
		if _, ok := nodeByID[e.Source]; !ok {
			continue
		}
		if _, ok := nodeByID[e.Target]; !ok {
			continue
		}
		adj[e.Source] = append(adj[e.Source], e.Target)
		adj[e.Target] = append(adj[e.Target], e.Source)
	}
	visited := map[string]bool{focus: true}
	queue := []string{focus}
	for depth := int32(0); depth < hops && len(queue) > 0 && len(visited) < unifiedGraphMaxNodes; depth++ {
		var next []string
		for _, cur := range queue {
			for _, nb := range adj[cur] {
				if visited[nb] {
					continue
				}
				visited[nb] = true
				next = append(next, nb)
				if len(visited) >= unifiedGraphMaxNodes {
					break
				}
			}
			if len(visited) >= unifiedGraphMaxNodes {
				break
			}
		}
		queue = next
	}

	// --- collect + min_weight filter (relation edges only) ---
	out := &UnifiedMemoryGraph{Focus: focus}
	for id := range visited {
		out.Nodes = append(out.Nodes, *nodeByID[id])
	}
	for _, e := range edges {
		if !visited[e.Source] || !visited[e.Target] {
			continue
		}
		if e.fromRelation && e.Weight < minWeight {
			out.FilteredEdgeCount++
			continue
		}
		out.Edges = append(out.Edges, e.UnifiedGraphEdge)
	}
	sort.Slice(out.Nodes, func(i, j int) bool { return out.Nodes[i].ID < out.Nodes[j].ID })
	sort.Slice(out.Edges, func(i, j int) bool {
		if out.Edges[i].Source != out.Edges[j].Source {
			return out.Edges[i].Source < out.Edges[j].Source
		}
		if out.Edges[i].Target != out.Edges[j].Target {
			return out.Edges[i].Target < out.Edges[j].Target
		}
		return out.Edges[i].Type < out.Edges[j].Type
	})
	return out, nil
}

// ── P3: L2 episode browsing（design §10.6.1）──

const (
	// episodesAdminDefaultLimit is the default page size for episode browsing.
	episodesAdminDefaultLimit = 20
	// episodesAdminMaxLimit caps user-supplied page size.
	episodesAdminMaxLimit = 100
)

// MemoryEpisodeItem is one L2 episode row for the memory-center browse tab.
type MemoryEpisodeItem struct {
	ID                  string
	SessionID           string
	AgentID             string
	Kind                string
	Title               string
	OutcomeSummary      string
	Importance          float64
	ConsolidationStatus string // pending | consolidated
	ConsolidatedL3Count int32
	EndedAt             string
	CreatedAt           string
}

// ListEpisodesAdmin returns paginated L2 episodes (created_at DESC) for one
// agent, optionally filtered by session. Reader-backed; no new SQL here.
func (uc *MemoryAdminUsecase) ListEpisodesAdmin(ctx context.Context, agentID, sessionID string, limit, offset int32) ([]MemoryEpisodeItem, int32, error) {
	if err := uc.requireAdmin(); err != nil {
		return nil, 0, err
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, 0, apierror.BadRequest(apierror.DomainMemory, "agent id is required")
	}
	if uc.l2AdminReader == nil {
		return nil, 0, apierror.Internal(apierror.DomainMemory, "memory center readers not wired")
	}
	if limit <= 0 {
		limit = episodesAdminDefaultLimit
	}
	if limit > episodesAdminMaxLimit {
		limit = episodesAdminMaxLimit
	}
	if offset < 0 {
		offset = 0
	}
	rows, total, _, err := uc.l2AdminReader.ListEpisodeRowsAdmin(ctx, agentID, strings.TrimSpace(sessionID), limit, offset)
	if err != nil {
		return nil, 0, err
	}
	items := make([]MemoryEpisodeItem, 0, len(rows))
	for _, raw := range rows {
		var m map[string]any
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		items = append(items, MemoryEpisodeItem{
			ID:                  jsonutil.IfaceStr(m, "id"),
			SessionID:           jsonutil.IfaceStr(m, "session_id"),
			AgentID:             jsonutil.IfaceStr(m, "agent_id"),
			Kind:                jsonutil.IfaceStr(m, "episode_kind"),
			Title:               jsonutil.IfaceStr(m, "title"),
			OutcomeSummary:      jsonutil.IfaceStr(m, "outcome_summary"),
			Importance:          jsonutil.IfaceF64(m, "importance"),
			ConsolidationStatus: jsonutil.IfaceStr(m, "consolidation_status"),
			ConsolidatedL3Count: jsonutil.IfaceI32(m, "consolidated_l3_count"),
			EndedAt:             jsonutil.IfaceStr(m, "ended_at"),
			CreatedAt:           jsonutil.IfaceStr(m, "created_at"),
		})
	}
	return items, total, nil
}

// heaviestGraphNode returns the node ID with the max weight (ties broken by ID
// for determinism). Returns "" for an empty graph.
func heaviestGraphNode(nodes map[string]*UnifiedGraphNode) string {
	best := ""
	var bestW float64
	for id, n := range nodes {
		if best == "" || n.Weight > bestW || (n.Weight == bestW && id < best) {
			best, bestW = id, n.Weight
		}
	}
	return best
}

func parseJSONStringSlice(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

// memCreatedToday reports whether a created_at timestamp (RFC3339 variants)
// falls on/after today 00:00 UTC.
func memCreatedToday(createdAt string, todayStart time.Time) bool {
	t := parseMemTime(createdAt)
	return !t.IsZero() && !t.Before(todayStart)
}

func parseMemTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
