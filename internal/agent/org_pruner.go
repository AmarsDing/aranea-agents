package agent

import (
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// OrgPruneReason explains why OrgPruner returned a particular candidate set.
const (
	OrgPruneReasonMatched     = "matched"
	OrgPruneReasonEmptyDomain = "empty_domain"
	OrgPruneReasonOtherDomain = "other_domain"
	OrgPruneReasonNoOrg       = "no_org"
	OrgPruneReasonNoMatch     = "no_match"
	OrgPruneReasonEmptyPool   = "empty_pool"
)

// maxLLMColdStartCandidates caps Layer-3 LLM matching (M78 NFR-78-01).
const maxLLMColdStartCandidates = 15

// OrgPruneResult is the deterministic department prune for one matching call.
type OrgPruneResult struct {
	// CandidateKeys are heuristic-assignable agent keys in matched departments.
	// Empty with FallbackAll=true means "use the full assignable catalog".
	CandidateKeys  []string
	DepartmentIDs  []string
	DepartmentName string
	FallbackAll    bool
	Reason         string
}

// OrgPruner deterministically maps a domain path onto departments present in
// the capability list. It does not call an LLM and does not create org nodes.
type OrgPruner struct{}

// Prune returns the assignable agents whose department matches the task
// domain. Empty domain, "其他", or missing org placement → FallbackAll.
// A known specialty with no department hit fails closed (FallbackAll=false,
// empty CandidateKeys) so Allocate can surface roster miss instead of
// company-wide wrong-person assignment.
func (OrgPruner) Prune(domainPath string, capabilities []biz.AgentCapability) OrgPruneResult {
	assignable := filterHeuristicAssignable(capabilities)
	if len(assignable) == 0 {
		return OrgPruneResult{FallbackAll: true, Reason: OrgPruneReasonEmptyPool}
	}

	placed := 0
	for _, cap := range assignable {
		if cap.DepartmentID != "" {
			placed++
		}
	}
	if placed == 0 {
		return OrgPruneResult{FallbackAll: true, Reason: OrgPruneReasonNoOrg}
	}

	norm := NormalizeDomainPath(domainPath)
	if strings.TrimSpace(norm) == "" {
		return OrgPruneResult{FallbackAll: true, Reason: OrgPruneReasonEmptyDomain}
	}
	if TopLevelDomain(norm) == domainLexiconOther {
		return OrgPruneResult{FallbackAll: true, Reason: OrgPruneReasonOtherDomain}
	}

	aliases := DomainDepartmentAliases(norm)
	if len(aliases) == 0 {
		// Known specialty, no department alias — fail-closed (roster miss),
		// not FallbackAll. Empty / 其他 / no-org still fail-open above.
		return OrgPruneResult{Reason: OrgPruneReasonNoMatch}
	}

	matchedDepts := make(map[string]string) // id → name
	for _, cap := range assignable {
		if cap.DepartmentID == "" {
			continue
		}
		if _, seen := matchedDepts[cap.DepartmentID]; seen {
			continue
		}
		if departmentMatchesDomain(cap, norm, aliases) {
			matchedDepts[cap.DepartmentID] = cap.DepartmentName
		}
	}
	if len(matchedDepts) == 0 {
		return OrgPruneResult{Reason: OrgPruneReasonNoMatch}
	}

	deptIDs := make([]string, 0, len(matchedDepts))
	primaryName := ""
	for id, name := range matchedDepts {
		deptIDs = append(deptIDs, id)
		if primaryName == "" {
			primaryName = name
		}
	}

	keys := make([]string, 0, len(assignable))
	seen := make(map[string]struct{}, len(assignable))
	for _, cap := range assignable {
		if _, ok := matchedDepts[cap.DepartmentID]; !ok {
			continue
		}
		if _, dup := seen[cap.AgentKey]; dup {
			continue
		}
		seen[cap.AgentKey] = struct{}{}
		keys = append(keys, cap.AgentKey)
	}
	if len(keys) == 0 {
		return OrgPruneResult{Reason: OrgPruneReasonEmptyPool}
	}
	return OrgPruneResult{
		CandidateKeys:  keys,
		DepartmentIDs:  deptIDs,
		DepartmentName: primaryName,
		Reason:         OrgPruneReasonMatched,
	}
}

func departmentMatchesDomain(cap biz.AgentCapability, domainPath string, aliases []string) bool {
	capDomain := NormalizeDomainPath(cap.DomainPath)
	if capDomain != "" && (specialtyPathCompatible(domainPath, capDomain) || domainPath == capDomain) {
		return true
	}
	if paths := biz.DepartmentDomainPaths(cap.DepartmentKey, cap.DepartmentName); len(paths) > 0 {
		for _, dp := range paths {
			norm := NormalizeDomainPath(dp)
			if specialtyPathCompatible(domainPath, norm) || domainPath == norm {
				return true
			}
		}
	}
	return matchDepartmentAlias(cap.DepartmentName, cap.DepartmentKey, aliases)
}

func filterHeuristicAssignable(caps []biz.AgentCapability) []biz.AgentCapability {
	out := make([]biz.AgentCapability, 0, len(caps))
	for _, cap := range caps {
		if cap.IsHeuristicAssignable() {
			out = append(out, cap)
		}
	}
	return out
}

func restrictCapabilities(caps []biz.AgentCapability, keys []string) []biz.AgentCapability {
	if len(keys) == 0 {
		return nil
	}
	want := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		want[k] = struct{}{}
	}
	out := make([]biz.AgentCapability, 0, len(keys))
	for _, cap := range caps {
		if _, ok := want[cap.AgentKey]; ok {
			out = append(out, cap)
		}
	}
	return out
}

func capLLMColdStart(caps []biz.AgentCapability) []biz.AgentCapability {
	if len(caps) <= maxLLMColdStartCandidates {
		return caps
	}
	return caps[:maxLLMColdStartCandidates]
}

// matchingPool returns the heuristic-assignable catalog pruned to the
// departments that match domainPath. Empty domain / 其他 / missing org →
// full assignable pool (NFR-78-06). Known specialty with no department hit
// returns an empty pool (fail-closed roster miss). Allocate never creates
// company/department nodes (ORGFAST-05).
func (impl *agentAllocatorImpl) matchingPool(domainPath string, capabilities []biz.AgentCapability, traceID string) ([]biz.AgentCapability, OrgPruneResult) {
	assignable := filterHeuristicAssignable(capabilities)
	prune := OrgPruner{}.Prune(domainPath, capabilities)
	if prune.FallbackAll {
		return assignable, prune
	}
	if len(prune.CandidateKeys) == 0 {
		impl.lg.Warn("组织剪枝未命中，不回退全量可分配池",
			loggateway.StepID(biz.SpiritStepAllocatorMatch),
			loggateway.Str("trace_id", traceID),
			loggateway.Str("domain_path", domainPath),
			loggateway.Str("reason", prune.Reason),
		)
		return nil, prune
	}
	pool := restrictCapabilities(assignable, prune.CandidateKeys)
	if len(pool) == 0 {
		impl.lg.Warn("组织剪枝结果为空，不回退全量可分配池",
			loggateway.StepID(biz.SpiritStepAllocatorMatch),
			loggateway.Str("trace_id", traceID),
			loggateway.Str("domain_path", domainPath),
			loggateway.Str("reason", OrgPruneReasonEmptyPool),
		)
		prune.Reason = OrgPruneReasonEmptyPool
		return nil, prune
	}
	deptID := ""
	if len(prune.DepartmentIDs) > 0 {
		deptID = prune.DepartmentIDs[0]
	}
	impl.lg.Info("组织剪枝命中",
		loggateway.StepID(biz.SpiritStepAllocatorMatch),
		loggateway.Str("trace_id", traceID),
		loggateway.Str("domain_path", domainPath),
		loggateway.Str("department_id", deptID),
		loggateway.Int("candidates", len(pool)),
	)
	return pool, prune
}

func stampOrgOnAlloc(alloc *biz.TaskAllocation, pool []biz.AgentCapability, prune OrgPruneResult) {
	if alloc == nil {
		return
	}
	if cap, ok := findCapabilityByKey(pool, alloc.AssignedKey); ok {
		if alloc.DepartmentID == "" {
			alloc.DepartmentID = cap.DepartmentID
		}
		if alloc.CompanyID == "" {
			alloc.CompanyID = cap.CompanyID
		}
		if cap.DepartmentName != "" && alloc.MatchReason != "" &&
			!strings.Contains(alloc.MatchReason, cap.DepartmentName) {
			alloc.MatchReason = fmt.Sprintf("%s (dept: %s)", alloc.MatchReason, cap.DepartmentName)
		}
	}
	if alloc.DepartmentID == "" && len(prune.DepartmentIDs) == 1 {
		alloc.DepartmentID = prune.DepartmentIDs[0]
	}
}

func departmentIDFromPool(domainPath string, pool []biz.AgentCapability) string {
	prune := OrgPruner{}.Prune(domainPath, pool)
	if len(prune.DepartmentIDs) == 1 {
		return prune.DepartmentIDs[0]
	}
	counts := map[string]int{}
	best, bestN := "", 0
	for _, cap := range pool {
		if cap.DepartmentID == "" {
			continue
		}
		counts[cap.DepartmentID]++
		if counts[cap.DepartmentID] > bestN {
			best = cap.DepartmentID
			bestN = counts[cap.DepartmentID]
		}
	}
	return best
}
