package biz

import (
	"context"
	"math"
)

// memory_eval_intent.go — AMC P3 VIR（Vector Intent Router）。
//
// 用「锚点向量 + 余弦匹配」替代全部意图书面词表（chronological / aggregate /
// enumerative / relational 查询侧模式，correction / forget 写入侧治理门）。
// 设计要点：
//   - 锚点与业务语句共用同一个 biz.EmbeddingService——向量空间天然自洽，
//     换嵌入模型（如 Jina/JEV 多语系模型）只需换 provider，本文件零改动；
//   - 每个意图挂中英双语正向锚点 + 少量反向锚点，多语言泛化由嵌入模型继承，
//     不维护任何语言词表；
//   - Match = maxCos(pos) >= threshold && maxCos(pos) > maxCos(neg)。反向锚点
//     专门压制「字面像意图、语义相反」的句子（如 难忘/don't forget 之于
//     forget），替代原词表的 reinforcement guard；
//   - 写侧阈值（0.72）高于读侧（0.60）：误触发失效是破坏性的，写侧宁缺；
//     且 forget/correction 通道仍受 cosine>=0.82 向量近邻门与时序守卫兜底。
//
// Lexical 保留项（经评估不按意图路由、与 cue 表无关）：IsHypotheticalStatement
// （假设句形守卫）、HasPreferenceUpdateCue / PreferenceSlotKey（同槽通道结构
// 性护栏）。

// EvalIntent is one route slot for query-mode or write-governance intents.
type EvalIntent string

const (
	EvalIntentChronological EvalIntent = "chronological"
	EvalIntentAggregate     EvalIntent = "aggregate"
	EvalIntentEnumerative   EvalIntent = "enumerative"
	EvalIntentRelational    EvalIntent = "relational"
	EvalIntentCorrection    EvalIntent = "correction"
	EvalIntentForget        EvalIntent = "forget"
	// EvalIntentTrend routes 模拟/规律/预测 queries onto the P3-c numeric
	// series digest rows (同键滚动 digest 置顶 + 外推值引用).
	EvalIntentTrend EvalIntent = "trend"
)

// evalIntentSpec declares one route: a cosine threshold and pos/neg anchor
// phrases (CN+EN mixed; the embedder provides cross-lingual generalization).
type evalIntentSpec struct {
	threshold float64
	pos, neg  []string
}

var evalIntentSpecs = map[EvalIntent]evalIntentSpec{
	EvalIntentChronological: {
		threshold: 0.60,
		pos: []string{
			"按时间顺序列出我的体检记录", "我搬家和入职的先后顺序",
			"这是我第几次换工作", "最早的那次是什么时候",
			"list my doctor visits in order", "what was the sequence of my moves",
			"what happened before and after the trip", "按先后讲讲这些事",
		},
		neg: []string{
			"我今天过得怎么样", "how is the weather this week", "我喜欢蓝色",
			// 单点事实问句：离 pos 锚点仅一两个词元，必须靠反向锚点压住
			// （评审 #2：词表时代不存在这一风险，向量路由必须自带 margin）。
			"when did I move", "我什么时候搬的家", "我第一次搬家是哪天",
		},
	},
	EvalIntentAggregate: {
		threshold: 0.60,
		pos: []string{
			"总结一下我们聊了什么", "我一共去过几次医院", "总共搬过几次家",
			"概括一下我的饮食偏好", "我对这几件事的总体看法",
			"give me a summary of my trips", "how many times did I go to the hospital",
			"overall, summarize what kinds of things I like", "数一下总数",
		},
		neg: []string{
			"我上次体检是哪一天", "what is my phone number", "我现在住在哪里",
			// 同上：单点偏好/事实问句不是 aggregate。
			"what do I like", "我喜欢什么", "do I like coffee",
		},
	},
	EvalIntentEnumerative: {
		threshold: 0.60,
		pos: []string{
			"我家有哪些成员", "我的三个孩子分别叫什么", "列出我提过的所有书名",
			"我的所有投资有哪些", "我去过哪几个城市", "每一种都在什么时候",
			"list all my hobbies", "what are the names of my kids",
			"how many cars have I owned", "name every place I lived",
		},
		neg: []string{
			"我的手机号是多少", "what is my favorite color", "我最喜欢哪一本书",
		},
	},
	EvalIntentRelational: {
		threshold: 0.60,
		pos: []string{
			"小王和小李是什么关系", "我和房东是怎么认识的", "有哪些证据支持我换过工作",
			"那次搬家的来龙去脉是怎样的", "他们为什么闹掰了",
			"what is the relationship between Tom and me", "how do they know each other",
			"what evidence supports my move", "show me the chain of events",
		},
		neg: []string{
			"我儿子叫什么名字", "do I have a car", "我现在的工作是什么",
		},
	},
	EvalIntentCorrection: {
		threshold: 0.72,
		pos: []string{
			"我改主意了，改成周三", "其实我喜欢红色，不是蓝色", "不对，应该是下周",
			"弄错了，会议是八点", "我刚把城市换成上海了", "不再是以前那个答案了",
			"actually I prefer tea now", "correction: the date is May 5",
			"I am no longer using Gmail", "in fact I moved last week",
		},
		neg: []string{
			"我一直没变过", "我还是喜欢蓝色", "nothing changed for me",
			"everything is the same as before",
		},
	},
	EvalIntentForget: {
		threshold: 0.72,
		pos: []string{
			"请忘掉我之前说过的旧手机号", "把我旧的手机号忘掉吧",
			"忘掉我的旧地址", "删掉我昨天说过的银行卡信息", "请抹掉那条记录",
			"please forget my old address", "forget about the phone number I gave you",
			"delete my record about the meeting", "do not keep my old address",
		},
		neg: []string{
			"我永远也不会忘记这个夏天", "这个夏天真难忘", "我忘不了她的脸",
			"don't forget my birthday", "never forget this day",
			"that day was unforgettable", "请记得我喜欢红色", "一定要记得这件事",
		},
	},
	EvalIntentTrend: {
		threshold: 0.60,
		pos: []string{
			"按这个趋势下周的步数大概是多少", "我的体重走势如何", "预测下个月的血糖",
			"这些读数有什么规律", "模拟一下接下来的变化", "步数是在上升还是下降",
			"based on the trend what will my steps be next week",
			"what is the pattern of my readings", "forecast my weight next month",
			"are my numbers going up or down",
		},
		neg: []string{
			"我上次测的是多少", "昨天走了多少步", "what was my weight last time",
			"how many steps did I walk yesterday",
		},
	},
}

// EvalIntentAnchors is the exported view of one intent's declared anchor set
// (threshold + pos/neg texts), so test doubles can mirror the live registry.
type EvalIntentAnchors struct {
	Threshold float64
	Pos, Neg  []string
}

// EvalIntentAnchorSpecs exposes the declared anchors per intent. Data-layer
// tests build deterministic embedder doubles from this instead of
// duplicating the anchor texts by hand.
func EvalIntentAnchorSpecs() map[EvalIntent]EvalIntentAnchors {
	out := make(map[EvalIntent]EvalIntentAnchors, len(evalIntentSpecs))
	for intent, spec := range evalIntentSpecs {
		out[intent] = EvalIntentAnchors{Threshold: spec.threshold, Pos: spec.pos, Neg: spec.neg}
	}
	return out
}

// evalIntentVec is the embedded form of one evalIntentSpec: all anchor vectors
// are L2-normalized at build time so Match is pure dot-product.
type evalIntentVec struct {
	threshold float32
	pos, neg  [][]float32
}

// EvalIntentRouter classifies a text embedding into eval intents via anchor
// vectors. Anchors are embedded once at construction with the SAME
// biz.EmbeddingService the store uses for facts/queries, so the vector space
// is always self-consistent regardless of provider (Jina/JEV/…).
type EvalIntentRouter struct {
	intents map[EvalIntent]evalIntentVec
}

// NewEvalIntentRouter embeds every anchor once and normalizes them. The batch
// path (biz.EmbeddingBatchService) is tried first; on ANY batch error it falls
// back to the per-text loop, and single-anchor failures there are skipped
// rather than aborting the whole router (评审 #7：一条瞬时抖动锚点不得把全
// 部锚点拖下线). Construction fails only when the caller context dies or
// NO anchor embeds at all (full outage) — the caller degrades accordingly.
func NewEvalIntentRouter(ctx context.Context, embedder EmbeddingService) (*EvalIntentRouter, error) {
	if embedder == nil {
		return nil, ErrMemoryUnavailable
	}
	var texts []string
	var owners []EvalIntent
	var isPos []bool
	for intent, spec := range evalIntentSpecs {
		for _, t := range spec.pos {
			texts = append(texts, t)
			owners = append(owners, intent)
			isPos = append(isPos, true)
		}
		for _, t := range spec.neg {
			texts = append(texts, t)
			owners = append(owners, intent)
			isPos = append(isPos, false)
		}
	}
	vecs := make([][]float32, len(texts))
	embedded := 0
	if eb, ok := embedder.(EmbeddingBatchService); ok {
		if v, err := eb.EmbedBatch(ctx, texts); err == nil && len(v) == len(texts) {
			copy(vecs, v)
			embedded = len(texts)
		}
	}
	if embedded == 0 {
		// Per-text fallback (no batch support, or batch failed): skip single
		// transient failures instead of nil-ing the entire router.
		for i, t := range texts {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			v, err := embedder.Embed(ctx, t)
			if err != nil {
				continue
			}
			vecs[i] = v
			embedded++
		}
	}
	if embedded == 0 {
		return nil, ErrMemoryUnavailable
	}
	r := &EvalIntentRouter{intents: make(map[EvalIntent]evalIntentVec, len(evalIntentSpecs))}
	for i, v := range vecs {
		if v == nil {
			continue
		}
		intent := owners[i]
		iv := r.intents[intent]
		iv.threshold = float32(evalIntentSpecs[intent].threshold)
		nv := normalizeVec32(v)
		if isPos[i] {
			iv.pos = append(iv.pos, nv)
		} else {
			iv.neg = append(iv.neg, nv)
		}
		r.intents[intent] = iv
	}
	return r, nil
}

// Match reports whether emb routes to intent: best positive-anchor cosine must
// clear the intent threshold AND beat every negative-anchor cosine (negatives
// encode the reinforcement/opposite semantics that literal cues could not
// separate). Unknown intents or empty inputs never match.
func (r *EvalIntentRouter) Match(emb []float32, intent EvalIntent) bool {
	if r == nil || len(emb) == 0 {
		return false
	}
	iv, ok := r.intents[intent]
	if !ok || len(iv.pos) == 0 {
		return false
	}
	n := normalizeVec32(emb)
	pos, neg := float32(0), float32(0)
	for _, a := range iv.pos {
		if c := dotVec32(n, a); c > pos {
			pos = c
		}
	}
	for _, a := range iv.neg {
		if c := dotVec32(n, a); c > neg {
			neg = c
		}
	}
	return pos >= iv.threshold && pos > neg
}

func normalizeVec32(v []float32) []float32 {
	var norm float64
	for _, x := range v {
		norm += float64(x) * float64(x)
	}
	if norm <= 0 {
		return v
	}
	inv := float32(1 / math.Sqrt(norm))
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = x * inv
	}
	return out
}

func dotVec32(a, b []float32) float32 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var s float32
	for i := 0; i < n; i++ {
		s += a[i] * b[i]
	}
	return s
}
