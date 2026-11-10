package biz

// D6 L-2 脱敏引擎（E 阶段 W2b，需求 §7.2/§7.3 验收 #3/#4 的引擎半边）。
//
// 语义自 workbench `server/mask.go` 逐字平移：
//   - 一级脱敏：规则驱动（正则 / 词表 × operator=placeholder|mask）。占位符
//     在作用域（会话）内保持一致映射：同一原文恒得同一 <ENTITY_n>，per-scope
//     per-entity 递增；operator=mask 直接替换 "***"。
//   - 灰度：rollout_pct 按作用域 FNV32a(scope)%100 < pct 命中才应用（同 scope
//     结果稳定；>=100 恒开、<=0 恒关）。
//   - 影子：shadow 规则只统计命中次数、不做替换（观察期）。
//   - 二级检测 secondCheck：独立于规则配置的内置高精确度识别器（防「规则过
//     宽/漏配」），findings 前缀 idcard:/phone:/bankcard:/email:；返回非空即
//     禁止上云（v2.0 §脱敏流水线结论：二次检测 + 失败即拒绝上云）。
//
// aranea 适配差异（W4 日志登记）：
//  1. 占位符实体名清洗改为 Unicode 感知（保留字母/数字）——wb 的
//     `[^A-Za-z0-9]` 会把中文实体名整体剥空坍缩成 <ENTITY_n>，同作用域内
//     不同实体的首个占位符互撞；保留 CJK 字符后 <手机号_1>/<身份证号_1> 语义
//     清晰且与 wb 的英文实体行为一致。
//  2. 作用域映射有上限（maxMaskEngineScopes），超限任意淘汰——wb 进程短
//     生命周期未做清理，aranea 常驻进程必须防无界增长（淘汰只影响占位符
//     编号续接，不影响脱敏正确性）。

import (
	"fmt"
	"hash/fnv"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

// 脱敏算子（与 mask_rules.operator 列取值一致）。
const (
	MaskOperatorPlaceholder = "placeholder" // <ENTITY_n>，作用域内一致
	MaskOperatorMask        = "mask"        // ***
)

// MaskRule 脱敏规则（mask_rules 表一行；DDL 20261327）。
type MaskRule struct {
	ID         string
	Name       string
	Entity     string
	Pattern    string   // 正则（与 Wordlist 二选一；均空 = 规则无效）
	Wordlist   []string // 词表（wordlist_json 反序列化）
	Operator   string   // placeholder / mask
	Enabled    bool
	Version    int
	Shadow     bool // 影子模式：只记录命中不替换
	RolloutPct int  // 按作用域灰度百分比（0..100）
}

type compiledMaskRule struct {
	MaskRule
	re *regexp.Regexp
}

// maxMaskEngineScopes 是占位符映射的作用域上限（防常驻进程无界增长）。
const maxMaskEngineScopes = 4096

// MaskEngine 是无状态规则集 + 有状态占位符映射的脱敏引擎。并发安全。
type MaskEngine struct {
	rules []compiledMaskRule

	mu       sync.Mutex
	maps     map[string]map[string]string // scope -> 原文 -> 占位符（会话内一致映射）
	counters map[string]map[string]int    // scope -> entity -> 计数
}

// NewMaskEngine 编译规则集。禁用规则跳过；正则编译失败返回 error
// （wb 同语义：整组规则不可信，不做部分装配）。
func NewMaskEngine(rules []MaskRule) (*MaskEngine, error) {
	e := &MaskEngine{
		maps:     make(map[string]map[string]string),
		counters: make(map[string]map[string]int),
	}
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		cr := compiledMaskRule{MaskRule: r}
		if strings.TrimSpace(r.Pattern) != "" {
			re, err := regexp.Compile(r.Pattern)
			if err != nil {
				return nil, fmt.Errorf("脱敏规则「%s」正则编译失败: %w", r.Name, err)
			}
			cr.re = re
		}
		if cr.re == nil && len(r.Wordlist) == 0 {
			continue // 既无正则又无词表的规则无效果，跳过
		}
		e.rules = append(e.rules, cr)
	}
	return e, nil
}

// RuleCount 返回编译后生效的规则条数（可观测/测试用）。
func (e *MaskEngine) RuleCount() int {
	if e == nil {
		return 0
	}
	return len(e.rules)
}

// sanitizeEntity 生成占位符实体名：保留 Unicode 字母/数字，其余剥离；
// 全空回落 ENTITY。见文件头差异说明 1。
func sanitizeEntity(entity string) string {
	var b strings.Builder
	for _, r := range entity {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	out := strings.ToUpper(b.String())
	if out == "" {
		return "ENTITY"
	}
	return out
}

// evictOneScopeLocked 在超出作用域上限时任意淘汰一个作用域（调用方持锁）。
func (e *MaskEngine) evictOneScopeLocked() {
	for scope := range e.maps {
		delete(e.maps, scope)
		delete(e.counters, scope)
		return
	}
}

// placeholder 返回原文在作用域内的稳定占位符（wb 逐字语义 + 上限淘汰）。
func (e *MaskEngine) placeholder(scope, entity, original, operator string) string {
	if operator == MaskOperatorMask {
		return "***"
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	m := e.maps[scope]
	if m == nil {
		if len(e.maps) >= maxMaskEngineScopes {
			e.evictOneScopeLocked()
		}
		m = map[string]string{}
		e.maps[scope] = m
	}
	if ph, ok := m[original]; ok {
		return ph
	}
	c := e.counters[scope]
	if c == nil {
		c = map[string]int{}
		e.counters[scope] = c
	}
	c[entity]++
	ph := "<" + sanitizeEntity(entity) + "_" + strconv.Itoa(c[entity]) + ">"
	m[original] = ph
	return ph
}

// MaskRolloutHit 作用域灰度判定：同一 scope 对同一规则结果稳定
// （FNV32a(scope)%100 < pct；>=100 恒开、<=0 恒关）。wb rolloutHit 逐字。
func MaskRolloutHit(scope string, pct int) bool {
	if pct >= 100 {
		return true
	}
	if pct <= 0 {
		return false
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(scope))
	return h.Sum32()%100 < uint32(pct)
}

// Mask 执行一级替换，返回脱敏文本与命中次数。scope 一般为会话 ID
// （回落 run/invocation ID）。shadow 规则只计命中不替换；rollout_pct<100
// 的规则按 scope 灰度跳过。wb mask 逐字语义。
func (e *MaskEngine) Mask(scope, text string) (string, int) {
	if e == nil || text == "" {
		return text, 0
	}
	hits := 0
	for _, r := range e.rules {
		if !MaskRolloutHit(scope, r.RolloutPct) {
			continue
		}
		if r.re != nil {
			cr := r
			if cr.Shadow {
				hits += len(cr.re.FindAllString(text, -1))
				continue
			}
			text = cr.re.ReplaceAllStringFunc(text, func(s string) string {
				hits++
				return e.placeholder(scope, cr.Entity, s, cr.Operator)
			})
			continue
		}
		for _, word := range r.Wordlist {
			if word == "" {
				continue
			}
			if r.Shadow {
				hits += strings.Count(text, word)
				continue
			}
			for {
				i := strings.Index(text, word)
				if i < 0 {
					break
				}
				hits++
				ph := e.placeholder(scope, r.Entity, word, r.Operator)
				text = text[:i] + ph + text[i+len(word):]
			}
		}
	}
	return text, hits
}

var (
	reIDCard = regexp.MustCompile(`\b\d{17}[\dXx]\b`)
	rePhone  = regexp.MustCompile(`\b1[3-9]\d{9}\b`)
	reCard   = regexp.MustCompile(`\b\d{16,19}\b`)
	reEmail  = regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`)
)

// SecondCheck 二级检测（漏脱兜底）：对「即将上云」的文本独立复扫。
// 返回非空即禁止上云。findings 形如 "idcard:<原文>"——**含 PII 原文，绝不
// 可写日志/审计/decision metadata**，调用侧只允许取数量与前缀类别。
func (e *MaskEngine) SecondCheck(text string) []string {
	if text == "" {
		return nil
	}
	var findings []string
	for _, m := range reIDCard.FindAllString(text, -1) {
		if validIDCard(m) {
			findings = append(findings, "idcard:"+m)
		}
	}
	for _, m := range rePhone.FindAllString(text, -1) {
		findings = append(findings, "phone:"+m)
	}
	for _, m := range reCard.FindAllString(text, -1) {
		if luhnValid(m) && !validIDCard(m) {
			findings = append(findings, "bankcard:"+m)
		}
	}
	for _, m := range reEmail.FindAllString(text, -1) {
		findings = append(findings, "email:"+m)
	}
	return findings
}

// SecondCheckKinds 把 findings 收敛为「类别集合」（不含原文，审计安全）。
func SecondCheckKinds(findings []string) []string {
	if len(findings) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, 4)
	out := make([]string, 0, 4)
	for _, f := range findings {
		kind := f
		if i := strings.IndexByte(f, ':'); i >= 0 {
			kind = f[:i]
		}
		if _, ok := seen[kind]; ok {
			continue
		}
		seen[kind] = struct{}{}
		out = append(out, kind)
	}
	return out
}

var idWeights = []int{7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2}

const idCodes = "10X98765432"

// validIDCard 校验 GB 11643 身份证校验位（wb 逐字）。
func validIDCard(s string) bool {
	if len(s) != 18 {
		return false
	}
	sum := 0
	for i := 0; i < 17; i++ {
		d := int(s[i] - '0')
		if d < 0 || d > 9 {
			return false
		}
		sum += d * idWeights[i]
	}
	c := byte(idCodes[sum%11])
	last := s[17]
	if last >= 'a' && last <= 'z' {
		last -= 32
	}
	return last == c
}

// luhnValid 银行卡 Luhn 校验（wb 逐字）。
func luhnValid(s string) bool {
	sum := 0
	double := false
	for i := len(s) - 1; i >= 0; i-- {
		d := int(s[i] - '0')
		if d < 0 || d > 9 {
			return false
		}
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	return sum%10 == 0
}
