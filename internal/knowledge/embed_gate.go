package knowledge

// D6 L-3 embedding 密级闸（E 阶段 W2b，需求 §7.3 验收 #7：L3+ 内容走
// cloud embed → 拒+审计；bge 本地 embedding 移植为独立拍板项，过渡期
// L3+ 内容不 embed 直接拒）。
//
// 闸口坐标：embedBatchInternal 是全量 embedding 流量的唯一网络漏斗
//（memory 适配器/ingest/reembed/promote/sourcesync/graph_card 全经此），
// 闸挂漏斗处一次覆盖全部路径。密级经 ctx 载体随行（摄取管线按文档
// classification 列注入，见 IngestParams.Classification）。
//
// 分层：闸判定逻辑（阈值/审计/指标）实现于 biz.SecopsEmbeddingGate
//（biz 不 import 本包，无环）；本包只定义载体/错误类型/本地性判定与
// 默认门持有器。

import (
	"context"
	"net"
	"net/url"
	"strings"
	"sync/atomic"

	"aranea-agents/internal/biz"
)

// EmbedClassificationBlockedError 包装闸拒绝：embedding 调用未发出网络请求。
// 摄取管线（BuildIndexedChunks）按 errors.As 识别并走词法降级（与
// ErrEmbedderNotConfigured 同款的 F5 降级口径：chunk 留空向量、BM25 可检索、
// 文档不卡 error 尸体）——「拒」的语义是拒 cloud embed 调用本身，不是拒
// 整文档入库。
type EmbedClassificationBlockedError struct {
	Cause error
}

func (e *EmbedClassificationBlockedError) Error() string {
	if e == nil || e.Cause == nil {
		return "embedding blocked by classification gate"
	}
	return "embedding blocked by classification gate: " + e.Cause.Error()
}

func (e *EmbedClassificationBlockedError) Unwrap() error { return e.Cause }

// embedClassificationCtx 是 ctx 载体值（level + subject 单键；subject 供审计
// 定位文档，绝不携带内容）。
type embedClassificationCtx struct {
	level   int
	subject string
}

// embedClassificationCtxKey 是 WithEmbedClassification 的 ctx 键（未导出，
// 强制经函数读写；与 decision.WithGateRunID 同型先例）。
type embedClassificationCtxKey struct{}

// WithEmbedClassification 把内容密级注入 embedding ctx。level 合法域 0..5
//（0=未标=放行云通道，与既有行为一致）；subject 为审计定位符（文档 id 等，
// 可空）。level<=0 且无 subject 时不写 ctx（零开销直通）。
func WithEmbedClassification(ctx context.Context, level int, subject string) context.Context {
	if ctx == nil || (level <= 0 && strings.TrimSpace(subject) == "") {
		return ctx
	}
	return context.WithValue(ctx, embedClassificationCtxKey{}, embedClassificationCtx{
		level:   level,
		subject: strings.TrimSpace(subject),
	})
}

// embedClassificationFromContext 取回载体；未注入返回零值（level 0）。
func embedClassificationFromContext(ctx context.Context) embedClassificationCtx {
	if ctx == nil {
		return embedClassificationCtx{}
	}
	if v, ok := ctx.Value(embedClassificationCtxKey{}).(embedClassificationCtx); ok {
		return v
	}
	return embedClassificationCtx{}
}

// IsLocalEmbedTarget 判定 embedding 提供方是否为本地通道（内容不出机）：
// host 为 loopback/localhost/RFC1918 私网/IPv6 ULA 即本地；ollama 与
// huggingface 的默认端点均为本机（空 baseURL=本地默认），openai 兼容网关
// 指向本机/私网（本地 vllm 等）同样算本地。host 解析失败保守按 cloud
//（密级判定宁严勿宽）。
func IsLocalEmbedTarget(provider, baseURL string) bool {
	host := ""
	if raw := strings.TrimSpace(baseURL); raw != "" {
		u, err := url.Parse(raw)
		if err != nil {
			return false
		}
		host = u.Hostname()
	}
	if host == "" {
		// 空 baseURL：ollama 回退 localhost:11434、huggingface 回退
		// localhost:8080（embedder.go 构造默认），其余按 cloud。
		switch provider {
		case ProviderOllama, ProviderHuggingFace:
			return true
		}
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate()
}

// defaultEmbedClassificationGate 进程级默认门（装配副作用注册，V2/V3
// SetDefaultXxx 先例；holder 包装避免 atomic.Value 存 nil 接口 panic；
// holder.gate 为 nil=门停用=全放行，行为同闸引入前）。
type embedGateHolder struct{ gate biz.EmbedClassificationGate }

var defaultEmbedClassificationGate atomic.Value // embedGateHolder

// SetDefaultEmbedClassificationGate 注册/清空进程级默认门（装配层调用）。
func SetDefaultEmbedClassificationGate(g biz.EmbedClassificationGate) {
	defaultEmbedClassificationGate.Store(embedGateHolder{gate: g})
}

// classificationGate 取生效门：实例级优先（SetClassificationGate，测试注入），
// 否则进程级默认。
func (e *MultiProviderEmbedder) classificationGate() biz.EmbedClassificationGate {
	if e == nil {
		return nil
	}
	e.mu.RLock()
	g := e.embedGate
	e.mu.RUnlock()
	if g != nil {
		return g
	}
	if v := defaultEmbedClassificationGate.Load(); v != nil {
		return v.(embedGateHolder).gate
	}
	return nil
}

// SetClassificationGate 注入实例级密级门（nil 回落进程级默认）。
func (e *MultiProviderEmbedder) SetClassificationGate(g biz.EmbedClassificationGate) {
	if e == nil {
		return
	}
	e.mu.Lock()
	e.embedGate = g
	e.mu.Unlock()
}

// checkEmbedGate 网络调用前的密级闸：构造 EmbedGateInfo（provider/baseURL
// 快照 + ctx 密级载体），门拒绝时包装 EmbedClassificationBlockedError 返回
//（零网络流量）。门为 nil 时直通。
func (e *MultiProviderEmbedder) checkEmbedGate(ctx context.Context, batchSize int) error {
	g := e.classificationGate()
	if g == nil {
		return nil
	}
	provider, baseURL, _, _, _ := e.snapshot()
	carrier := embedClassificationFromContext(ctx)
	info := biz.EmbedGateInfo{
		Provider:       provider,
		BaseURL:        baseURL,
		Local:          IsLocalEmbedTarget(provider, baseURL),
		Classification: carrier.level,
		Subject:        carrier.subject,
		BatchSize:      batchSize,
	}
	if err := g.CheckEmbed(ctx, info); err != nil {
		return &EmbedClassificationBlockedError{Cause: err}
	}
	return nil
}
