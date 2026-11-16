// Package inbox 实现统一收件箱聚合器：所有「需要人处理」的事项在服务端投影
// 为同一种条目（Item），网页/移动端/Agent 读同一份口径。
//
// 设计约束：
//   - 本包是纯聚合与分发骨架，不依赖任何领域包；各来源通过 Source 接口接入
//     （service 层适配，复用各领域既有读/写路径与访问校验）。
//   - 逐源可见性：来源自行按调用方权限过滤，无权时返回空集 + 0，不返回错误。
//   - 单源故障隔离：某来源读取失败只进入 Degraded 列表，其余来源照常返回。
//   - 决议不信任客户端：Decide 按 id 回到来源重新解析条目（确认仍待处理且对
//     调用方可见），再交来源自己的写路径执行。
package inbox

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// 条目类别（Item.Kind）。
const (
	KindDecide    = "decide"    // 审批：批准 / 驳回
	KindConfirm   = "confirm"   // 确认执行（工具确认）
	KindReview    = "review"    // 验收
	KindException = "exception" // 异常待处理
)

// 风险级（与 approval 包、proto RiskLevel 同名小写）。
const (
	RiskLow    = "low"
	RiskMedium = "medium"
	RiskHigh   = "high"
)

// 动作（Item.Actions / Decide 的 action 参数）。
const (
	ActionApprove = "approve"
	ActionReject  = "reject"
	ActionAck     = "ack"
	ActionResolve = "resolve"
)

// Item 是统一收件箱条目（proto InboxItem 的领域形态）。
type Item struct {
	ID        string // `<source>:<NativeID>`，由聚合器统一拼装
	Source    string // 来源名（= Source.Name()）
	NativeID  string // 领域内条目 id
	Kind      string
	Title     string
	Summary   string
	Risk      string
	Requester string
	AgentKey  string // 归因 agent；空 = 无法归因
	CreatedAt time.Time
	DueAt     time.Time // 零值 = 无截止
	Actions   []string
	// RejectRequiresNote 驳回是否必须带理由（由来源声明，Decide 前置校验）。
	RejectRequiresNote bool
	Link               string // 前端深链
	ContextJSON        string // 来源特定上下文（仅展示用）
	// Ref 是来源私有的决议定位信息（如工具确认的 session_id / activity_id）。
	// 只在服务端流转，不出 proto。
	Ref map[string]string
}

// Source 是一类待办来源的统一读写口。
//
// Stability:evolving
type Source interface {
	// Name 返回来源名（全局唯一，作为 Item.ID 前缀；不得含冒号）。
	Name() string
	// List 返回对调用方可见的待处理条目（最多 limit 条）与可见总数。
	// 调用方无权时返回 (nil, 0, nil)。
	List(ctx context.Context, limit int) (items []Item, total int64, err error)
	// Decide 对一条已由聚合器重新解析过的条目执行动作。
	Decide(ctx context.Context, item Item, action, note string) error
}

// 哨兵错误（service 层映射为 400/404/409）。
var (
	// ErrUnknownSource 请求了未注册的来源。
	ErrUnknownSource = errors.New("inbox: unknown source")
	// ErrInvalidID 条目 id 形态非法。
	ErrInvalidID = errors.New("inbox: invalid item id")
	// ErrGone 条目已不在待处理集合（已被处理、已失效或对调用方不可见）。
	ErrGone = errors.New("inbox: item is no longer pending")
	// ErrActionNotAllowed 条目不支持该动作。
	ErrActionNotAllowed = errors.New("inbox: action not allowed for this item")
	// ErrNoteRequired 驳回缺少理由。
	ErrNoteRequired = errors.New("inbox: reject requires a note")
)

// maxSourceFetch 单源单次拉取上限（跨源合并分页在内存进行）。
const maxSourceFetch = 500

// Aggregator 跨源聚合收件箱条目（注册即接入）。
type Aggregator struct {
	sources map[string]Source
	order   []string
}

// NewAggregator 装配聚合器；nil 来源跳过，同名后者覆盖前者（顺序不变）。
func NewAggregator(sources ...Source) *Aggregator {
	a := &Aggregator{sources: make(map[string]Source, len(sources))}
	for _, s := range sources {
		if s == nil {
			continue
		}
		name := s.Name()
		if name == "" || strings.Contains(name, ":") {
			continue
		}
		if _, dup := a.sources[name]; !dup {
			a.order = append(a.order, name)
		}
		a.sources[name] = s
	}
	return a
}

// Sources 返回已注册来源名（注册顺序）。
func (a *Aggregator) Sources() []string {
	out := make([]string, len(a.order))
	copy(out, a.order)
	return out
}

// Result 是一次聚合读取的结果。
type Result struct {
	Items    []Item
	Total    int64
	HighRisk int64            // 已拉取条目中的高风险数
	Counts   map[string]int64 // 来源 → 可见待办数（已选来源全量，含 0）
	Order    []string         // Counts 的稳定输出序
	Degraded []string         // 读取失败的来源
}

// List 聚合待办：sources 为空=全部来源；分页在合并排序后进行。
func (a *Aggregator) List(ctx context.Context, sources []string, page, pageSize int) (Result, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}
	names, err := a.selectSources(sources)
	if err != nil {
		return Result{}, err
	}
	fetch := page * pageSize
	if fetch > maxSourceFetch {
		fetch = maxSourceFetch
	}
	res := a.collect(ctx, names, fetch)
	start := (page - 1) * pageSize
	if start >= len(res.Items) {
		res.Items = nil
		return res, nil
	}
	end := start + pageSize
	if end > len(res.Items) {
		end = len(res.Items)
	}
	res.Items = res.Items[start:end]
	return res, nil
}

// Summary 返回全来源计数（全站唯一待办数口径）。Items 不返回。
func (a *Aggregator) Summary(ctx context.Context) Result {
	res := a.collect(ctx, a.order, maxSourceFetch)
	res.Items = nil
	return res
}

// Decide 对条目执行动作：按 id 回源重新解析 → 校验动作与理由 → 交来源执行。
func (a *Aggregator) Decide(ctx context.Context, id, action, note string) (Item, error) {
	source, nativeID, ok := SplitID(id)
	if !ok {
		return Item{}, ErrInvalidID
	}
	src, found := a.sources[source]
	if !found {
		return Item{}, fmt.Errorf("%w: %s", ErrUnknownSource, source)
	}
	action = strings.ToLower(strings.TrimSpace(action))
	note = strings.TrimSpace(note)

	items, _, err := src.List(ctx, maxSourceFetch)
	if err != nil {
		return Item{}, err
	}
	var item *Item
	for i := range items {
		if items[i].NativeID == nativeID {
			item = &items[i]
			break
		}
	}
	if item == nil {
		return Item{}, ErrGone
	}
	if !containsAction(item.Actions, action) {
		return Item{}, fmt.Errorf("%w: %s", ErrActionNotAllowed, action)
	}
	if action == ActionReject && item.RejectRequiresNote && note == "" {
		return Item{}, ErrNoteRequired
	}
	normalize(item, source)
	if err := src.Decide(ctx, *item, action, note); err != nil {
		return Item{}, err
	}
	return *item, nil
}

// SplitID 拆分 `<source>:<nativeID>`（nativeID 内可含冒号）。
func SplitID(id string) (source, nativeID string, ok bool) {
	id = strings.TrimSpace(id)
	i := strings.Index(id, ":")
	if i <= 0 || i == len(id)-1 {
		return "", "", false
	}
	return id[:i], id[i+1:], true
}

func (a *Aggregator) selectSources(requested []string) ([]string, error) {
	var picked []string
	seen := map[string]bool{}
	for _, r := range requested {
		r = strings.TrimSpace(r)
		if r == "" || seen[r] {
			continue
		}
		if _, ok := a.sources[r]; !ok {
			return nil, fmt.Errorf("%w: %s", ErrUnknownSource, r)
		}
		seen[r] = true
		picked = append(picked, r)
	}
	if len(picked) == 0 {
		return a.order, nil
	}
	// 输出序跟随注册序，与请求顺序无关（稳定）。
	out := make([]string, 0, len(picked))
	for _, name := range a.order {
		if seen[name] {
			out = append(out, name)
		}
	}
	return out, nil
}

func (a *Aggregator) collect(ctx context.Context, names []string, fetch int) Result {
	res := Result{Counts: make(map[string]int64, len(names)), Order: append([]string(nil), names...)}
	for _, name := range names {
		items, total, err := a.sources[name].List(ctx, fetch)
		if err != nil {
			// 调用方取消不是来源故障：不标降级，直接带着已有结果返回。
			if ctx.Err() != nil {
				break
			}
			res.Counts[name] = 0
			res.Degraded = append(res.Degraded, name)
			continue
		}
		if total < int64(len(items)) {
			total = int64(len(items))
		}
		res.Counts[name] = total
		res.Total += total
		for i := range items {
			normalize(&items[i], name)
			if items[i].Risk == RiskHigh {
				res.HighRisk++
			}
		}
		res.Items = append(res.Items, items...)
	}
	sortItems(res.Items)
	return res
}

func normalize(it *Item, source string) {
	it.Source = source
	it.ID = source + ":" + it.NativeID
	switch it.Risk {
	case RiskLow, RiskMedium, RiskHigh:
	default:
		it.Risk = RiskMedium
	}
}

func riskRank(r string) int {
	switch r {
	case RiskHigh:
		return 3
	case RiskMedium:
		return 2
	default:
		return 1
	}
}

// sortItems 排序口径：风险高→低；同级有截止的在前且截止近→远；其余按创建
// 新→旧；最后按 ID 字典序兜底（稳定可复现）。
func sortItems(items []Item) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if ra, rb := riskRank(a.Risk), riskRank(b.Risk); ra != rb {
			return ra > rb
		}
		aDue, bDue := !a.DueAt.IsZero(), !b.DueAt.IsZero()
		if aDue != bDue {
			return aDue
		}
		if aDue && !a.DueAt.Equal(b.DueAt) {
			return a.DueAt.Before(b.DueAt)
		}
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.After(b.CreatedAt)
		}
		return a.ID < b.ID
	})
}

func containsAction(actions []string, action string) bool {
	for _, a := range actions {
		if a == action {
			return true
		}
	}
	return false
}
