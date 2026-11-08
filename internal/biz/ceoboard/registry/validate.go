package registry

import (
	"fmt"
	"regexp"
	"strings"
)

// 合法适配器类型(§14.4)。db-query 为预留;hybrid 为 nats+rest 复合。
var adapterKinds = map[string]bool{
	"nats-jetstream": true, "rest-poll": true, "webhook-recv": true,
	"sse": true, "ws": true, "hybrid": true, "db-query": true,
}

var probeTypes = map[string]bool{"rest": true, "nats": true, "ws": true}
var drillTypes = map[string]bool{"rest": true, "sse": true, "console_url": true}

// 条件表达式形态:标识符(任意内容),如 event(subject='x') / job(state='queued')。
var condExpr = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*\([\s\S]*\)$`)

// 阈值表达式:开头必须为比较符+数值(允许 % 后缀),后续允许自由修饰文本(如 "持续30m")。
var thresholdExpr = regexp.MustCompile(`^\s*(>=|<=|>|<|=)\s*\d+(\.\d+)?%?`)

// 指标算子白名单(R3-Q3:四算子;算不出的必须声明 precompute)。
var metricOps = []string{"count(", "ratio(", "latency(", "gauge("}

// Validate 对一条工作流做 schema 校验,返回全部错误(空切片=通过)。
// 校验失败的工作流整组不加载(F17 原子加载;§16.2 失败隔离)。
func Validate(wf *Workflow) []string {
	var errs []string
	fail := func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) }

	if wf.APIVersion != "ceoboard/v1" {
		fail("api_version 必须为 ceoboard/v1,实际 %q", wf.APIVersion)
	}
	if wf.Kind != "Workflow" {
		fail("kind 必须为 Workflow,实际 %q", wf.Kind)
	}
	if wf.ID == "" {
		fail("id 必填")
	}
	if wf.Name == "" {
		fail("name 必填")
	}
	if wf.BizLine == "" {
		fail("biz_line 必填")
	}
	if wf.Owner == "" {
		fail("owner 必填")
	}
	if wf.Version < 1 {
		fail("version 必须 >=1")
	}
	// F16:权限标签漏配即拒绝(CI 强制非空;缺省=仅 ceo 的语义由"非空且不含其他角色"表达)。
	if len(wf.Permissions) == 0 {
		fail("permissions 必须显式非空(F16 默认拒绝)")
	}

	if wf.Probe != nil {
		p := wf.Probe
		if !probeTypes[p.Type] {
			fail("probe.type %q 非法(可选 rest|nats|ws)", p.Type)
		}
		if p.Type == "rest" && !strings.HasPrefix(strings.ToUpper(p.Path), "GET ") &&
			!strings.HasPrefix(strings.ToUpper(p.Path), "HEAD ") {
			fail("probe.path(rest) 须为 'GET <url>' 形态,实际 %q", p.Path)
		}
		if p.Interval.D() <= 0 {
			fail("probe.interval 必须为正时长")
		}
		if p.StaleAfter.D() <= 0 {
			fail("probe.stale_after 必须为正时长")
		}
	}

	if len(wf.Nodes) == 0 {
		fail("nodes 至少一条")
	}
	seenNode, seenOrder := map[string]bool{}, map[int]bool{}
	for i := range wf.Nodes {
		n := &wf.Nodes[i]
		if n.ID == "" {
			fail("nodes[%d].id 必填", i)
		} else if seenNode[n.ID] {
			fail("node id %q 重复", n.ID)
		}
		seenNode[n.ID] = true
		if seenOrder[n.Order] {
			fail("node %q order %d 重复", n.ID, n.Order)
		}
		seenOrder[n.Order] = true
		if !condExpr.MatchString(n.Enter) {
			fail("node %q enter 表达式形态非法(须为 ident(...)): %q", n.ID, n.Enter)
		}
		if !condExpr.MatchString(n.Exit) {
			fail("node %q exit 表达式形态非法: %q", n.ID, n.Exit)
		}
		for _, b := range n.BranchOut {
			if !condExpr.MatchString(b.When) || b.To == "" {
				fail("node %q branch_out 形态非法", n.ID)
			}
		}
		errs = append(errs, validateSource(n)...)
		errs = append(errs, validateMetrics(n)...)
		for j, d := range n.Drill {
			if !drillTypes[d.Type] {
				fail("node %q drill[%d].type %q 非法(可选 rest|sse|console_url)", n.ID, j, d.Type)
			}
		}
	}
	return errs
}

func validateSource(n *Node) []string {
	var errs []string
	s := n.Source
	if s == nil {
		return errs
	}
	if !adapterKinds[s.Adapter] {
		errs = append(errs, fmt.Sprintf("node %q source.adapter %q 非法", n.ID, s.Adapter))
	}
	pending := strings.Contains(s.RestPath, PendingMarker)
	switch s.Adapter {
	case "rest-poll":
		if s.RestPath == "" {
			errs = append(errs, fmt.Sprintf("node %q rest-poll 必须给 rest_path", n.ID))
		}
	case "nats-jetstream":
		if len(s.Subjects) == 0 {
			errs = append(errs, fmt.Sprintf("node %q nats-jetstream 必须给 subjects", n.ID))
		}
	case "webhook-recv":
		if s.DedupeKey == "" {
			errs = append(errs, fmt.Sprintf("node %q webhook-recv 必须给 dedupe_key", n.ID))
		}
	}
	if s.RestPath != "" && !pending && !looksLikeRESTPath(s.RestPath) {
		errs = append(errs, fmt.Sprintf("node %q rest_path 形态非法(须为 'GET <url|path>'): %q", n.ID, s.RestPath))
	}
	return errs
}

func looksLikeRESTPath(p string) bool {
	up := strings.ToUpper(p)
	return strings.HasPrefix(up, "GET ") || strings.HasPrefix(up, "HEAD ") || strings.HasPrefix(up, "POST ")
}

func validateMetrics(n *Node) []string {
	var errs []string
	seen := map[string]bool{}
	for i := range n.Metrics {
		m := &n.Metrics[i]
		if m.ID == "" {
			errs = append(errs, fmt.Sprintf("node %q metrics[%d].id 必填", n.ID, i))
		} else if seen[m.ID] {
			errs = append(errs, fmt.Sprintf("node %q 指标 id %q 重复", n.ID, m.ID))
		}
		seen[m.ID] = true
		ok := false
		for _, op := range metricOps {
			if strings.HasPrefix(strings.TrimSpace(m.Expr), op) {
				ok = true
				break
			}
		}
		if !ok && m.Precompute == "" {
			errs = append(errs, fmt.Sprintf("指标 %q expr 算子非法(仅 count/ratio/latency/gauge;算不出须声明 precompute): %q", m.ID, m.Expr))
		}
		for level, th := range m.Thresholds {
			if level != "yellow" && level != "red" {
				errs = append(errs, fmt.Sprintf("指标 %q thresholds 仅支持 yellow/red,实际 %q", m.ID, level))
			}
			if !thresholdExpr.MatchString(th) {
				errs = append(errs, fmt.Sprintf("指标 %q 阈值 %q 不可解析(须以比较符+数值开头)", m.ID, th))
			}
		}
	}
	return errs
}
