// Package evolution 装配框架 v1.11 evolution service（技能演化学习）。
//
// 首期运行模式为「全挂起」（hold-all）：
//   - HumanGate = AlwaysHold：所有 reviewer 产出的技能修订一律挂起待人工审批，
//     零自动发布（不配 Publisher、不开 shadow——shadow 语义是"闸门仅记录不拦截，
//     照样发布"，与观察模式相反，见框架 worker.go shouldPublish）。
//   - CandidateStore = 文件存储：挂起/拒绝的修订全量落盘，形成可审查的审计轨迹。
//   - Spec/Safety 用框架默认闸门（密钥/危险 shell/路径穿越扫描）。
//
// 消费侧零改动：aranea DB skill repo 已接 llmagent.WithSkills，将来审批发布
// 通路（Publisher → SkillUsecase，需过 importer 校验）落地后技能即时生效。
//
// 已知限制（框架 v1.11.1，待上游修复）：演化 worker 的增量水位线
//（writeLastReviewAt → sess.SetState）只写 session 内存态，异步 worker
// 处理 job 时 runner 已完成本 turn 的持久化（AppendEvent），水位线永不落库；
// aranea 的 PG session service 无缓存、每 turn 从 DB 重建 State，故跨 turn
// 水位线必丢，scanDelta 退化为全量评审。hold-all 模式下影响限于评审 LLM
// 成本随 session 长度增长与候选重复累积（reconcile 批内去重兜底），功能
// 正确性无损。评审触发本身有 DefaultReviewPolicy 门槛（≥4 次工具调用/用户
// 纠正/工具错误恢复），纯聊天 turn 不产生评审开销。
package evolution

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/provider"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	trpcevolution "trpc.group/trpc-go/trpc-agent-go/evolution"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
)

// Config 是装配 evolution service 的依赖集。
type Config struct {
	// Catalog 用于延迟解析评审模型（与 session 摘要同策略：PickTitleModel）。
	// 启动时目录可能尚未配置模型，故首次 EnqueueLearningJob 时才解析构建。
	Catalog *biz.LlmProviderModelUsecase
	RT      *provider.RoundTrip
	// Repo 向 reviewer 提供既有技能上下文（框架 skill.Repository 只读接口）。
	Repo trpcskill.Repository
	// UsageRef 迟绑定解析 usage usecase，记录评审 LLM 调用的
	// aux_evolution 用量（P1-2，2026-08-19）；nil/未就绪时跳过记录。
	// 用 ref 而非直接注入 *UsageUsecase：evolution service 经
	// PersistenceSet 位于 wire DI 图上游，直接注入成环。
	UsageRef *biz.UsageUsecaseRef
	// CandidatesDir 为挂起修订的落盘目录；空时走默认（UserConfigDir/Aranea/
	// evolution/candidates，可用 EVOLUTION_CANDIDATES_DIR 覆盖）。
	CandidatesDir string
	Lg            loggateway.Logger
}

// Service 延迟构建框架 evolution service，实现 trpcevolution.Service。
// 目录无可用模型时 EnqueueLearningJob 静默跳过（与摘要 resolver 同策略）。
type Service struct {
	cfg Config

	mu    sync.Mutex
	inner trpcevolution.Service
	tried bool // 构建只尝试一次，失败不再重试（模型配置变化需重启生效）
}

// NewService 创建延迟装配的 evolution service；cfg 缺关键依赖时返回 nil
// （nil service 不会被接线到 runner，语义与框架 WithEvolutionService(nil) 一致）。
func NewService(cfg Config) *Service {
	if cfg.Catalog == nil || cfg.RT == nil || cfg.Repo == nil {
		return nil
	}
	if cfg.Lg == nil {
		cfg.Lg = loggateway.NewNoop()
	}
	return &Service{cfg: cfg}
}

// EnqueueLearningJob 实现 trpcevolution.Service。首次调用时构建底层 service。
func (s *Service) EnqueueLearningJob(ctx context.Context, job trpcevolution.LearningJob) error {
	inner := s.resolve(ctx)
	if inner == nil {
		return nil
	}
	return inner.EnqueueLearningJob(ctx, job)
}

// Close 实现 trpcevolution.Service；仅在底层已构建时传播。
func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inner == nil {
		return nil
	}
	return s.inner.Close()
}

func (s *Service) resolve(ctx context.Context) trpcevolution.Service {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inner != nil || s.tried {
		return s.inner
	}
	s.tried = true

	m, prov, mod, err := s.resolveReviewModel(ctx)
	if err != nil || m == nil {
		s.cfg.Lg.Warn("evolution: 评审模型解析失败，技能演化保持停用",
			loggateway.StepID("skill.evolution_resolve"),
			loggateway.Err(err))
		return nil
	}
	// P1-2 (2026-08-19): 计量包装——框架 reviewer 在内部调 LLM，用量不经
	// aranea 任何记录点；包一层 model 把每次评审调用记为 aux_evolution。
	if s.cfg.UsageRef != nil {
		m = newMeteringModel(m, s.cfg.UsageRef, prov, mod, s.cfg.Lg)
	}

	dir := resolveCandidatesDir(s.cfg.CandidatesDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		s.cfg.Lg.Warn("evolution: 候选存储目录创建失败，技能演化保持停用",
			loggateway.StepID("skill.evolution_mkdir"),
			loggateway.Str("dir", dir),
			loggateway.Err(err))
		return nil
	}

	s.inner = trpcevolution.NewService(m,
		trpcevolution.WithSkillRepository(s.cfg.Repo),
		trpcevolution.WithSpecGate(trpcevolution.NewDefaultSpecGate()),
		trpcevolution.WithSafetyGate(trpcevolution.NewDefaultSafetyGate()),
		// 全挂起：一切修订待人工审批，零自动发布。
		trpcevolution.WithHumanGate(trpcevolution.NewAlwaysHoldGate()),
		trpcevolution.WithCandidateStore(trpcevolution.NewFileCandidateStore(dir)),
	)
	s.cfg.Lg.Info("evolution: 技能演化 service 已构建（hold-all 模式，零自动发布）",
		loggateway.StepID("skill.evolution_ready"),
		loggateway.Str("candidates_dir", dir))
	return s.inner
}

// resolveReviewModel 复用 session 摘要的模型选取策略：catalog 的标题模型，
// 回退到第一个启用模型；目录为空时返回 (nil, "", "", nil)。
// 同时返回 provider/model 标识供用量计价归因（P1-2）。
func (s *Service) resolveReviewModel(ctx context.Context) (trpcmodel.Model, string, string, error) {
	models, err := s.cfg.Catalog.List(ctx)
	if err != nil {
		return nil, "", "", err
	}
	if len(models) == 0 {
		return nil, "", "", nil
	}
	pm, ok := biz.PickTitleModel(models)
	if !ok {
		return nil, "", "", nil
	}
	m, err := provider.TRPCModelForProviderModel(ctx, s.cfg.Catalog, s.cfg.RT, pm.Provider, pm.Model, s.cfg.Lg)
	if err != nil {
		return nil, "", "", err
	}
	return m, pm.Provider, pm.Model, nil
}

func resolveCandidatesDir(configured string) string {
	if dir := strings.TrimSpace(configured); dir != "" {
		return dir
	}
	if dir := strings.TrimSpace(os.Getenv("EVOLUTION_CANDIDATES_DIR")); dir != "" {
		return dir
	}
	if base, err := os.UserConfigDir(); err == nil && strings.TrimSpace(base) != "" {
		return filepath.Join(base, "Aranea", "evolution", "candidates")
	}
	return filepath.Join("data", "evolution", "candidates")
}

var _ trpcevolution.Service = (*Service)(nil)

// meteringModel 包装评审模型，把每次 GenerateContent 调用的 token 用量记录为
// aux_evolution 事件（P1-2，2026-08-19）。框架 reviewer 在内部 worker 里调
// LLM，不走 aranea 的任何计量点，只能用模型层装饰器截获。
type meteringModel struct {
	inner trpcmodel.Model
	// record 落一条 aux 用量；由 newMeteringModel 从 UsageRef 迟解析填充。
	// 测试可直注入捕获闭包。
	record func(ctx context.Context, in biz.AuxLLMUsageInput) error
	prov   string
	mod    string
	lg     loggateway.Logger
}

func newMeteringModel(inner trpcmodel.Model, usageRef *biz.UsageUsecaseRef, prov, mod string, lg loggateway.Logger) trpcmodel.Model {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &meteringModel{
		inner: inner,
		record: func(ctx context.Context, in biz.AuxLLMUsageInput) error {
			if u := usageRef.Get(); u != nil {
				return u.RecordAuxLLMUsage(ctx, in)
			}
			return nil
		},
		prov: prov,
		mod:  mod,
		lg:   lg,
	}
}

func (m *meteringModel) Info() trpcmodel.Info { return m.inner.Info() }

func (m *meteringModel) GenerateContent(ctx context.Context, req *trpcmodel.Request) (<-chan *trpcmodel.Response, error) {
	ch, err := m.inner.GenerateContent(ctx, req)
	if err != nil {
		return nil, err
	}
	out := make(chan *trpcmodel.Response, 64)
	safego.Go(ctx, "skill.evolution.metering_forward", func() { m.forward(ctx, ch, out) })
	return out, nil
}

// forward 透传响应流并累计计费口径用量（每轮独立计费 → 跨轮求和；轮内
// 流式累计 → 取最大，与 agent.accumulateStreamUsage 同语义）。流关闭后
// 落一条 aux_evolution 用量；零 token（provider 未回报）跳过。
func (m *meteringModel) forward(ctx context.Context, in <-chan *trpcmodel.Response, out chan<- *trpcmodel.Response) {
	defer close(out)
	start := time.Now()
	var prevPrompt, prevCompletion, prevCached int
	var curPrompt, curCompletion, curCached int
	var apiErr string
	for resp := range in {
		if resp != nil {
			if resp.Error != nil {
				apiErr = resp.Error.Message
			}
			if u := resp.Usage; u != nil {
				if u.PromptTokens != curPrompt {
					prevPrompt += curPrompt
					prevCompletion += curCompletion
					prevCached += curCached
					curPrompt = u.PromptTokens
					curCompletion = u.CompletionTokens
					curCached = u.PromptTokensDetails.CachedTokens
				} else {
					if u.CompletionTokens > curCompletion {
						curCompletion = u.CompletionTokens
					}
					if u.PromptTokensDetails.CachedTokens > curCached {
						curCached = u.PromptTokensDetails.CachedTokens
					}
				}
			}
		}
		select {
		case out <- resp:
		case <-ctx.Done():
			// 收方随 ctx 取消弃读时必须有逃生门，否则本 goroutine 永久阻塞泄漏。
			return
		}
	}
	promptTok := prevPrompt + curPrompt
	completionTok := prevCompletion + curCompletion
	if promptTok <= 0 && completionTok <= 0 {
		return
	}
	if m.record == nil {
		return
	}
	status := "success"
	if apiErr != "" {
		status = "failed"
	}
	// 计费落库脱离已取消的请求 ctx（WithoutCancel 保留 trace 值），但必须有界。
	rctx, rcancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer rcancel()
	if err := m.record(rctx, biz.AuxLLMUsageInput{
		Kind:          biz.UsageKindAuxEvolution,
		Provider:      m.prov,
		Model:         m.mod,
		Status:        status,
		PromptTok:     promptTok,
		CompletionTok: completionTok,
		CachedTok:     prevCached + curCached,
		UsageSource:   biz.UsageSourceResponse,
		Latency:       time.Since(start),
		ErrMsg:        apiErr,
	}); err != nil {
		m.lg.Warn("evolution: 评审用量落库失败",
			loggateway.StepID("skill.evolution_usage_record"),
			loggateway.Err(err))
	}
}
