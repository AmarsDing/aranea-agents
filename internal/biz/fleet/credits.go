package fleet

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// 平台积分（大屏显示「积分与额度」）。只用合规来源：
//   - deepseek：官方余额接口 GET /user/balance，密钥取后端环境变量 DEEPSEEK_API_KEY（DeepSeek 终端用的同一个）；
//   - codex：本机日志 ~/.codex/sessions 里 token_count 事件的 rate_limits.credits 与 plan_type；
//   - claude / codebuddy / qoder / trae / cursor：没有公开接口。复用 claude.ai 登录是本项目合规红线，
//     WorkBuddy 逆向接口被 ADR-88 D3 禁止，其余只有未公开的登录态接口——一律由用户手填。

// PlatformCredit 是一个平台的积分 / 余额。Total、Remaining 为 -1 表示未知。
type PlatformCredit struct {
	Platform  string  `json:"platform"`
	Source    string  `json:"source"` // auto | manual
	Plan      string  `json:"plan"`
	Total     float64 `json:"total"`
	Remaining float64 `json:"remaining"`
	Unit      string  `json:"unit"`
	Unlimited bool    `json:"unlimited"`
	UpdatedAt string  `json:"updated_at"`
	Note      string  `json:"note"`
	// 近 30 天每个 UTC 日最后一次的剩余（升序）；由快照拼出，不随手填记录存
	History []CreditPoint `json:"-"`
}

// CreditPoint 是某个 UTC 日最后一次记下的剩余。
type CreditPoint struct {
	Date      string  `json:"date"`
	Remaining float64 `json:"remaining"`
}

// creditSnap 是 kindCreditSnap 记录：每个平台每个 UTC 日一条，同一天后写的覆盖先写的。
type creditSnap struct {
	Platform string `json:"platform"`
	CreditPoint
}

const (
	kindCredit     = "credit"
	kindCreditSnap = "credit_snap"
	// creditHistoryDays 是保留与返回的天数；更早的快照在读取时顺手删掉
	creditHistoryDays = 30
)

// ManualCreditPlatforms 是只能手填的平台，顺序即界面顺序。
var ManualCreditPlatforms = []string{"claude", "codebuddy", "qoder", "trae", "cursor"}

// deepseekBalanceURL 测试里换成 httptest 地址。
var deepseekBalanceURL = "https://api.deepseek.com/user/balance"

var (
	creditMu     sync.Mutex
	creditAuto   []PlatformCredit
	creditAutoAt time.Time
)

// ListCredits 返回自动取到的 + 手填的积分；自动部分缓存 60 秒（与 Usage 一致）。
func (u *Usecase) ListCredits(ctx context.Context) ([]PlatformCredit, error) {
	creditMu.Lock()
	out, fresh := append([]PlatformCredit(nil), creditAuto...), creditAuto != nil && time.Since(creditAutoAt) < usageTTL
	creditMu.Unlock()
	if !fresh {
		// 锁外取数（DeepSeek 最慢 10 秒，不能把并发请求都卡住）；用脱离请求的上下文，
		// 否则浏览器取消请求会把「连不上」缓存 60 秒。并发未命中最多多取一次，可以接受。
		out = []PlatformCredit{deepseekCredit(context.WithoutCancel(ctx), os.Getenv("DEEPSEEK_API_KEY")), u.codexCredit()}
		creditMu.Lock()
		creditAuto, creditAutoAt = out, time.Now()
		creditMu.Unlock()
		out = append([]PlatformCredit(nil), out...)
		for _, c := range out {
			u.recordCreditSnap(context.WithoutCancel(ctx), c)
		}
	}

	for _, p := range ManualCreditPlatforms {
		c, ok, err := getDoc[PlatformCredit](ctx, u.store, kindCredit, p)
		if err != nil {
			return nil, err
		}
		if !ok {
			c = PlatformCredit{Platform: p, Total: -1, Remaining: -1}
		}
		c.Platform, c.Source = p, "manual"
		out = append(out, c)
	}

	history := u.creditHistory(ctx, time.Now())
	for i := range out {
		out[i].History = history[out[i].Platform]
	}
	return out, nil
}

// recordCreditSnap 记下当天（UTC）的剩余，同一天覆盖。剩余未知或不限量不记。记失败只打日志：积分照常返回。
func (u *Usecase) recordCreditSnap(ctx context.Context, c PlatformCredit) {
	if c.Remaining < 0 || c.Unlimited || c.UpdatedAt == "" {
		return
	}
	date := time.Now().UTC().Format("2006-01-02")
	snap := creditSnap{Platform: c.Platform, CreditPoint: CreditPoint{Date: date, Remaining: c.Remaining}}
	if err := putDoc(ctx, u.store, kindCreditSnap, c.Platform+":"+date, snap); err != nil {
		u.lg.Warn("fleet credit snapshot failed", loggateway.Str("platform", c.Platform), loggateway.Err(err))
	}
}

// creditHistory 按平台返回近 creditHistoryDays 天的快照（日期升序），并删掉更早的。读失败返回空：没有历史不影响当前值。
func (u *Usecase) creditHistory(ctx context.Context, now time.Time) map[string][]CreditPoint {
	out := map[string][]CreditPoint{}
	docs, err := u.store.List(ctx, kindCreditSnap)
	if err != nil {
		u.lg.Warn("fleet credit history failed", loggateway.Err(err))
		return out
	}
	cutoff := now.UTC().AddDate(0, 0, -(creditHistoryDays - 1)).Format("2006-01-02")
	for _, raw := range docs {
		var s creditSnap
		if json.Unmarshal(raw, &s) != nil || s.Platform == "" {
			continue
		}
		if s.Date < cutoff {
			_ = u.store.Delete(ctx, kindCreditSnap, s.Platform+":"+s.Date)
			continue
		}
		out[s.Platform] = append(out[s.Platform], s.CreditPoint)
	}
	for p := range out {
		sort.Slice(out[p], func(i, j int) bool { return out[p][i].Date < out[p][j].Date })
	}
	return out
}

// SetCredit 登记一个手填平台的积分。total / remaining 填 -1 表示不知道。
func (u *Usecase) SetCredit(ctx context.Context, c PlatformCredit) (PlatformCredit, error) {
	if !isManualCreditPlatform(c.Platform) {
		return PlatformCredit{}, apierror.BadRequest(domain, "该平台不支持手填积分："+c.Platform)
	}
	if c.Total < -1 || c.Remaining < -1 || (c.Total >= 0 && c.Remaining > c.Total) {
		return PlatformCredit{}, apierror.BadRequest(domain, "剩余不能大于总额，且都不能为负数")
	}
	c.Source, c.UpdatedAt = "manual", nowString()
	c.Plan, c.Unit, c.Note = strings.TrimSpace(c.Plan), strings.TrimSpace(c.Unit), strings.TrimSpace(c.Note)
	// 与表单上限一致；接口不能只靠前端拦
	if utf8.RuneCountInString(c.Plan) > 40 || utf8.RuneCountInString(c.Note) > 80 {
		return PlatformCredit{}, apierror.BadRequest(domain, "套餐最多 40 个字、备注最多 80 个字")
	}
	if c.Unit == "" {
		c.Unit = "credits"
	}
	if c.Unit != "credits" && c.Unit != "CNY" && c.Unit != "USD" {
		return PlatformCredit{}, apierror.BadRequest(domain, "单位只能是 credits、CNY 或 USD")
	}
	if err := putDoc(ctx, u.store, kindCredit, c.Platform, c); err != nil {
		return PlatformCredit{}, err
	}
	u.recordCreditSnap(ctx, c)
	c.History = u.creditHistory(ctx, time.Now())[c.Platform]
	return c, nil
}

func isManualCreditPlatform(p string) bool {
	for _, m := range ManualCreditPlatforms {
		if m == p {
			return true
		}
	}
	return false
}

// deepseekCredit 调官方余额接口。取不到时 Note 写原因，不报错（大屏上如实显示）。
func deepseekCredit(ctx context.Context, key string) PlatformCredit {
	c := PlatformCredit{Platform: "deepseek", Source: "auto", Total: -1, Remaining: -1, Unit: "CNY"}
	if key == "" {
		c.Note = "后端没有配置 DEEPSEEK_API_KEY"
		return c
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, deepseekBalanceURL, nil)
	if err != nil {
		c.Note = err.Error()
		return c
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.Note = "余额接口连不上：" + err.Error()
		return c
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode != http.StatusOK {
		c.Note = fmt.Sprintf("余额接口返回 %d", resp.StatusCode)
		return c
	}
	var v struct {
		IsAvailable  bool `json:"is_available"`
		BalanceInfos []struct {
			Currency     string `json:"currency"`
			TotalBalance string `json:"total_balance"`
		} `json:"balance_infos"`
	}
	if err := json.Unmarshal(body, &v); err != nil || len(v.BalanceInfos) == 0 {
		c.Note = "余额接口返回的格式看不懂"
		return c
	}
	// 多币种时取第一项（DeepSeek 账户只有一种币种）
	b := v.BalanceInfos[0]
	if f, err := strconv.ParseFloat(b.TotalBalance, 64); err == nil {
		c.Remaining = f
	}
	if b.Currency != "" {
		c.Unit = b.Currency
	}
	if !v.IsAvailable {
		c.Note = "余额不足，暂不可用"
	}
	c.UpdatedAt = nowString()
	return c
}

// codexCredit 取运行台用量扫描（与 /v1/fleet/usage 同一次扫描、同一份缓存）里最近一次 Codex 额度快照，
// 跨近 7 天的所有会话日志取最新的那次，不会因为刚开的新会话还没有快照就读不到。
func (u *Usecase) codexCredit() PlatformCredit {
	c := PlatformCredit{Platform: "codex", Source: "auto", Total: -1, Remaining: -1, Unit: "credits"}
	for _, a := range u.Usage() {
		if a.Agent != "codex" {
			continue
		}
		if a.codexLimits == nil {
			c.Note = "Codex 日志里还没有额度快照"
			return c
		}
		return codexCreditFrom(*a.codexLimits, time.Unix(a.codexAt, 0))
	}
	c.Note = "本机没有 Codex 会话日志"
	return c
}

type codexRateLimits struct {
	PlanType string `json:"plan_type"`
	Credits  *struct {
		HasCredits bool            `json:"has_credits"`
		Unlimited  bool            `json:"unlimited"`
		Balance    json.RawMessage `json:"balance"`
	} `json:"credits"`
}

func codexCreditFrom(rl codexRateLimits, at time.Time) PlatformCredit {
	c := PlatformCredit{Platform: "codex", Source: "auto", Total: -1, Remaining: -1, Unit: "credits",
		Plan: rl.PlanType, UpdatedAt: at.UTC().Format(time.RFC3339)}
	switch {
	case rl.Credits == nil:
		c.Note = "这个 Codex 版本的日志不带积分信息"
	case rl.Credits.Unlimited:
		c.Unlimited = true
	case !rl.Credits.HasCredits:
		c.Remaining, c.Note = 0, "账户没有积分"
	default:
		// balance 可能是数字或字符串
		s := strings.Trim(string(rl.Credits.Balance), `"`)
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			c.Remaining = f
		}
	}
	return c
}

// codexRateLimitsOf 从一行 token_count 事件里取 rate_limits（plan_type 与 credits）。
func codexRateLimitsOf(line []byte) (codexRateLimits, bool) {
	var v struct {
		Payload struct {
			RateLimits *codexRateLimits `json:"rate_limits"`
		} `json:"payload"`
	}
	if json.Unmarshal(line, &v) != nil || v.Payload.RateLimits == nil {
		return codexRateLimits{}, false
	}
	return *v.Payload.RateLimits, true
}
