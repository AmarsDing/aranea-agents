// nats-jetstream 适配器(自 ceo-dashboard/internal/adapter 移植,语义等价)。
// 设计要点:
//   - 双实例物理独立:HCF 与 NovaTrader 各自连接、各自 durable consumer,不桥接;
//   - 每 节点×subject 一个 durable pull consumer(DeliverNew:只看实时流量,不回填历史淹没指标);
//   - 归一化:event_uid = 平台:原去重键(信封 event_id 优先),落库 event_uid 主键幂等;
//   - subject 版本内嵌(.vN)剥离出稳定 subject_key,版本演进不丢事件;
//   - 失败隔离:单 subject 消费失败只影响对应节点状态,不影响其他消费者;
//   - 解码失败进死信(nats-decode),事件仍以摘要键落库(不丢);
//   - 连接级失败带 backoff 自动重接线(修复"启动期 NATS 不可用即永久不接线"缺陷);
//   - 启动期 StreamInfo 探测声明流;缺失→"声明漂移"标红(stream_optional 声明者标灰"未启用");
//   - jetAPI 窄接口 + dial 注入点,断连/重连演练可脚本化(测试 fake);
//   - durable 名稳定(风险点⑩):重启复用同一 durable,不重复消费也不丢。
//
// 移植差异:日志 log.Printf → 注入 loggateway.Logger(可空,禁 log/slog 红线);
// 存储改走 Ent 仓储并全量穿 context.Context;NATSNodeState/ConsumerState/SubjectKey/HostPort
// 在 types.go(agg 依赖面);删除原文件死代码 truncateStr。
package adapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"

	"aranea-agents/internal/biz/ceoboard/registry"
	"aranea-agents/internal/biz/ceoboard/store"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// ---- 可注入窄接口(演练:fake 脚本化失败/恢复) ----

// jetAPI 是 JetStream 的最小依赖面。
type jetAPI interface {
	AddConsumer(stream string, cfg *nats.ConsumerConfig) (*nats.ConsumerInfo, error)
	UpdateConsumer(stream string, cfg *nats.ConsumerConfig) (*nats.ConsumerInfo, error)
	PullSubscribe(subj, durable string, opts ...nats.SubOpt) (pullSub, error)
	StreamInfo(stream string, opts ...nats.JSOpt) (*nats.StreamInfo, error)
	StreamNameBySubject(subj string, opts ...nats.JSOpt) (string, error)
}

// pullSub 是 PullSubscription 的最小依赖面。
type pullSub interface {
	Fetch(batch int, opts ...nats.PullOpt) ([]*nats.Msg, error)
	ConsumerInfo() (*nats.ConsumerInfo, error)
}

// realJS 包装 nats.JetStreamContext 为 jetAPI。
type realJS struct{ js nats.JetStreamContext }

func (r realJS) AddConsumer(s string, c *nats.ConsumerConfig) (*nats.ConsumerInfo, error) {
	return r.js.AddConsumer(s, c)
}
func (r realJS) UpdateConsumer(s string, c *nats.ConsumerConfig) (*nats.ConsumerInfo, error) {
	return r.js.UpdateConsumer(s, c)
}
func (r realJS) PullSubscribe(subj, durable string, opts ...nats.SubOpt) (pullSub, error) {
	return r.js.PullSubscribe(subj, durable, opts...)
}
func (r realJS) StreamInfo(s string, opts ...nats.JSOpt) (*nats.StreamInfo, error) {
	return r.js.StreamInfo(s, opts...)
}
func (r realJS) StreamNameBySubject(subj string, opts ...nats.JSOpt) (string, error) {
	return r.js.StreamNameBySubject(subj, opts...)
}

// NATSManager 管理全部 NATS 实例与消费协程。
type NATSManager struct {
	st         *store.Store
	platformOf map[string]string // endpoint host:port → 平台 id(事件打标)
	log        loggateway.Logger // 可空:空时静默(单测)

	// 注入点(测试):dial 连接工厂;jsOf 由连接构造 jetAPI。默认真实实现。
	dial func(endpoint string, opts ...nats.Option) (*nats.Conn, error)
	jsOf func(nc *nats.Conn) (jetAPI, error)

	// 退避策略(测试可调瞬时):
	// RetryWait 是 WatchNode 连接级重试退避(第 attempt 次,从 1 计);默认 5s 起线性到 30s 封顶。
	RetryWait func(attempt int) time.Duration
	// FetchRetryWait 是消费协程 Fetch 失败(非超时)的退避;默认 2s。
	FetchRetryWait time.Duration
	// ConsumerSampleInterval 是 ConsumerInfo 周期采样间隔;默认 15s。
	ConsumerSampleInterval time.Duration

	// DurablePrefix 是 durable 名前缀;默认 "ceoboard"。
	// 双跑期(P2-11,方案 §10 P2)改为 "ceoboard-v2" 之类——同名 durable 会形成
	// queue-group 分流,新旧系统各收一半消息(R5 严重事故)。
	DurablePrefix string
	// DeliverAll 为 true 时新建 consumer 用 DeliverAllPolicy(从最早 offset 重放
	// 建库,双跑期口径);false 用 DeliverNewPolicy(默认,只看实时流量,历史回填
	// 会把窗口指标打爆)。注意:DeliverPolicy 对已存在 durable 不可变,本开关只
	// 影响新建 consumer;追平后无需调回(consumer 重放完自然进入实时流)。
	DeliverAll bool

	mu        sync.Mutex
	states    map[string]*NATSNodeState // key = wfID/nodeID
	consumers map[string]*ConsumerState // key = nodeKey/durable
	conns     []*nats.Conn
}

// NewNATSManager 构造管理器;platformOf 形如 {"192.168.4.44:14222":"hcf"}。log 可传 nil(单测静默)。
func NewNATSManager(st *store.Store, platformOf map[string]string, log loggateway.Logger) *NATSManager {
	m := &NATSManager{
		st: st, platformOf: platformOf, log: log,
		states: map[string]*NATSNodeState{}, consumers: map[string]*ConsumerState{},
		ConsumerSampleInterval: 15 * time.Second,
		DurablePrefix:          "ceoboard",
	}
	m.dial = func(endpoint string, opts ...nats.Option) (*nats.Conn, error) { return nats.Connect(endpoint, opts...) }
	m.jsOf = func(nc *nats.Conn) (jetAPI, error) {
		js, err := nc.JetStream()
		if err != nil {
			return nil, err
		}
		return realJS{js}, nil
	}
	m.RetryWait = func(attempt int) time.Duration {
		d := time.Duration(attempt) * 5 * time.Second
		if d > 30*time.Second {
			d = 30 * time.Second
		}
		return d
	}
	m.FetchRetryWait = 2 * time.Second
	return m
}

func (m *NATSManager) logInfo(msg string, fields ...loggateway.Field) {
	if m.log != nil {
		m.log.Info(msg, fields...)
	}
}

func (m *NATSManager) logWarn(msg string, fields ...loggateway.Field) {
	if m.log != nil {
		m.log.Warn(msg, fields...)
	}
}

func (m *NATSManager) logError(msg string, fields ...loggateway.Field) {
	if m.log != nil {
		m.log.Error(msg, fields...)
	}
}

// untrackConn 从纳管清单摘除连接(接线失败回滚用)。
func (m *NATSManager) untrackConn(nc *nats.Conn) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, c := range m.conns {
		if c == nc {
			m.conns = append(m.conns[:i], m.conns[i+1:]...)
			return
		}
	}
}

// Close 关闭全部纳管连接(评审 2026-11-09 H2,Runtime.Stop 调用,幂等)。
// 此前 conns 只进不出:接线重试失败与进程关停都泄漏 nats.Conn——其内部
// 读写/ping 协程与 socket 不随 ctx 取消退出,且 MaxReconnects(-1) 使断线
// 旧连接永远自我重连,重试风暴即放大为 FD 耗尽。
func (m *NATSManager) Close() {
	m.mu.Lock()
	conns := m.conns
	m.conns = nil
	m.mu.Unlock()
	for _, nc := range conns {
		if nc != nil {
			nc.Close()
		}
	}
}

// Lookup 取节点事件侧运行态(未接线返回 nil)。
func (m *NATSManager) Lookup(nodeKey string) *NATSNodeState {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.states[nodeKey]; ok {
		cp := *s
		return &cp
	}
	return nil
}

// ConsumerStates 返回全部 consumer 的最近一次成功采样及显式采样错误。
func (m *NATSManager) ConsumerStates() []ConsumerState {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]ConsumerState, 0, len(m.consumers))
	for _, state := range m.consumers {
		out = append(out, *state)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].NodeKey != out[j].NodeKey {
			return out[i].NodeKey < out[j].NodeKey
		}
		return out[i].Durable < out[j].Durable
	})
	return out
}

func (m *NATSManager) sampleConsumer(key string, sub pullSub, st *NATSNodeState) {
	info, err := sub.ConsumerInfo()
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.consumers[key]
	if state == nil {
		return
	}
	state.Connected = st.Connected
	if err != nil {
		state.SampleError = err.Error()
		return
	}
	state.NumPending = int(info.NumPending)
	state.AsOf = time.Now()
	state.SampleError = ""
}

// WatchNode 为一个 nats-jetstream / hybrid 节点建立消费,连接级失败带 backoff 自动重接线,
// 直到 ctx 取消。单节点一个协程;内部所有失败只更新本节点状态。
func (m *NATSManager) WatchNode(ctx context.Context, wfID string, n *registry.Node) {
	src := n.Source
	nodeKey := wfID + "/" + n.ID
	platform := m.platformOf[HostPort(src.Endpoint)]
	if platform == "" {
		platform = HostPort(src.Endpoint) // 未配置映射时用 endpoint 兜底,不编平台名
	}
	st := &NATSNodeState{NodeKey: nodeKey, Endpoint: src.Endpoint, Stream: src.Stream, StreamOptional: src.StreamOptional, StartedAt: time.Now()}
	m.mu.Lock()
	m.states[nodeKey] = st
	m.mu.Unlock()

	backoffAttempt := 0
	for {
		if ctx.Err() != nil {
			return
		}
		err := m.wireOnce(ctx, wfID, n, st, platform)
		if err == nil {
			return // 消费协程已启动,WatchNode 使命完成(断线重连由 nats.go 层负责)
		}
		m.setErr(st, err.Error())
		backoffAttempt++
		wait := m.RetryWait(backoffAttempt)
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		m.logWarn("[nats] 接线失败,退避后重试",
			loggateway.Str("node", nodeKey), loggateway.Err(err),
			loggateway.Str("wait", wait.String()), loggateway.Int("attempt", backoffAttempt))
	}
}

// wireOnce 完成一次"连接→StreamInfo 探测→建 consumers";任一失败返回错误(由 WatchNode 退避重试)。
func (m *NATSManager) wireOnce(ctx context.Context, wfID string, n *registry.Node, st *NATSNodeState, platform string) error {
	src := n.Source
	nodeKey := st.NodeKey
	nc, err := m.dial(src.Endpoint,
		nats.Name("ceoboard-"+nodeKey),
		nats.MaxReconnects(-1), // 无限重连(失败模式:断线必须自愈)
		nats.ReconnectWait(2*time.Second),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			m.mu.Lock()
			st.Connected = false
			if err != nil {
				st.LastErr = "断开: " + err.Error()
			}
			m.mu.Unlock()
			m.logWarn("[nats] 连接断开", loggateway.Str("node", nodeKey), loggateway.Err(err))
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			m.mu.Lock()
			st.Connected = true
			st.LastErr = ""
			st.Reconnects++
			m.mu.Unlock()
			m.logInfo("[nats] 重连成功", loggateway.Str("node", nodeKey), loggateway.Str("url", nc.ConnectedUrl()))
		}),
	)
	if err != nil {
		return errors.New("连接失败: " + err.Error())
	}
	// 评审 2026-11-09 H2:dial 后的任一失败路径必须摘除并关闭连接(wired 仅在
	// 正常接线完成后置位)——此前直接返回让该连接(含重连循环)永久泄漏。
	wired := false
	defer func() {
		if !wired {
			m.untrackConn(nc)
			if nc != nil {
				nc.Close()
			}
		}
	}()
	m.mu.Lock()
	m.conns = append(m.conns, nc)
	st.Connected = true
	m.mu.Unlock()

	js, err := m.jsOf(nc)
	if err != nil {
		return errors.New("JetStream 上下文: " + err.Error())
	}

	// 解析 stream:声明优先;未声明(NT 单流场景)按首个 subject 反查
	stream := src.Stream
	if stream == "" && len(src.Subjects) > 0 {
		if name, err := js.StreamNameBySubject(src.Subjects[0]); err == nil {
			stream = name
		}
	}
	if stream == "" {
		return errors.New("stream 未声明且按 subject 反查失败")
	}
	m.mu.Lock()
	st.Stream = stream
	m.mu.Unlock()

	// 声明漂移探测:声明了 stream 名就必须真实存在(声明显式可选者除外)
	if src.Stream != "" {
		if _, err := js.StreamInfo(stream); err != nil {
			m.mu.Lock()
			st.StreamMissing = true
			m.mu.Unlock()
			if src.StreamOptional {
				// 条件创建流(如 HCF MM_PUB 仅 MAP worker 启用时建):缺失=未启用,非漂移,不阻断重试
				m.logInfo("[nats] 声明流未创建(声明显式可选,平台方未启用)",
					loggateway.Str("node", nodeKey), loggateway.Str("stream", stream))
				wired = true // 空探测连接按设计保留(Stop 统一关闭)
				return nil
			}
			return errors.New("声明漂移: stream " + stream + " 在 JetStream 不存在(" + err.Error() + ")")
		}
		m.mu.Lock()
		st.StreamMissing = false
		m.mu.Unlock()
	}

	// 实体字段路径(field_map.entity_id 支持点路径,如 payload.signal_id;[待补] 已被加载器剔除)
	entityPath := ""
	if src.FieldMap != nil {
		entityPath = src.FieldMap["entity_id"]
	}

	anyConsuming := false
	prefix := m.DurablePrefix
	if prefix == "" { // 零值兜底(手工构造的 manager)
		prefix = "ceoboard"
	}
	for _, subject := range src.Subjects {
		durable := prefix + "-" + sanitizeDurable(nodeKey) + "-" + sanitizeDurable(SubjectKey(subject))
		if err := m.consumeSubject(ctx, js, stream, durable, subject, platform, entityPath, st); err != nil {
			// 单 subject 失败隔离:记录并继续其余 subject
			m.setErr(st, subject+": "+err.Error())
			continue
		}
		anyConsuming = true
		m.logInfo("[nats] 建立消费",
			loggateway.Str("node", nodeKey), loggateway.Str("stream", stream),
			loggateway.Str("subject", subject), loggateway.Str("durable", durable),
			loggateway.Str("platform", platform))
	}
	m.mu.Lock()
	st.Consuming = anyConsuming
	m.mu.Unlock()
	if !anyConsuming {
		return errors.New("全部 subject 消费建立失败")
	}
	wired = true
	return nil
}

func (m *NATSManager) setErr(st *NATSNodeState, msg string) {
	m.mu.Lock()
	st.LastErr = msg
	m.mu.Unlock()
	m.logWarn("[nats] "+msg, loggateway.Str("node", st.NodeKey))
}

// consumeSubject 建立单 subject 的 durable pull consumer 并启动拉取协程。
func (m *NATSManager) consumeSubject(ctx context.Context, js jetAPI,
	stream, durable, subject, platform, entityPath string, st *NATSNodeState) error {

	deliverPolicy := nats.DeliverNewPolicy // 只看实时流量;历史回填会把窗口指标打爆(乱序/回填风险)
	if m.DeliverAll {
		deliverPolicy = nats.DeliverAllPolicy // 双跑期:新 durable 从最早 offset 重放建库(仅新建生效)
	}
	cfg := &nats.ConsumerConfig{
		Durable:       durable,
		FilterSubject: subject,
		DeliverPolicy: deliverPolicy,
		AckPolicy:     nats.AckExplicitPolicy,
		MaxDeliver:    -1,
		AckWait:       30 * time.Second,
	}
	info, err := js.AddConsumer(stream, cfg)
	if err != nil || info.Config.MaxDeliver != cfg.MaxDeliver {
		// 已存在的 durable 也要迁移 MaxDeliver,否则旧配置仍会在持久化故障时终态丢弃。
		updated, updateErr := js.UpdateConsumer(stream, cfg)
		if updateErr != nil {
			return fmt.Errorf("创建 consumer: %v; 更新已有 consumer: %w", err, updateErr)
		}
		info = updated
	}
	sub, err := js.PullSubscribe(subject, durable, nats.BindStream(stream))
	if err != nil {
		return err
	}

	consumerKey := st.NodeKey + "/" + durable
	m.mu.Lock()
	m.consumers[consumerKey] = &ConsumerState{
		NodeKey: st.NodeKey, Platform: platform, Stream: stream, Durable: durable,
		Subject: subject, NumPending: int(info.NumPending), Connected: st.Connected, AsOf: time.Now(),
	}
	m.mu.Unlock()
	safego.Go(ctx, "ceoboard.nats_consumer_sample", func() {
		m.sampleConsumer(consumerKey, sub, st)
		ticker := time.NewTicker(m.ConsumerSampleInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.sampleConsumer(consumerKey, sub, st)
			}
		}
	})

	safego.Go(ctx, "ceoboard.nats_fetch", func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			msgs, err := sub.Fetch(50, nats.MaxWait(2*time.Second))
			if err != nil {
				if err == nats.ErrTimeout {
					m.mu.Lock()
					st.Backlog = false
					m.mu.Unlock()
					continue
				}
				if ctx.Err() != nil {
					return
				}
				// 连接中断等:短暂退避(可被 ctx 取消),重连由 nats.go 层负责
				select {
				case <-ctx.Done():
					return
				case <-time.After(m.FetchRetryWait):
				}
				continue
			}
			// 满批 = 可能有积压(下一批立即还有);供看板提示"消费积压中"
			m.mu.Lock()
			st.Backlog = len(msgs) >= 50
			m.mu.Unlock()
			for _, msg := range msgs {
				m.handleMsg(ctx, msg, platform, entityPath, st)
			}
		}
	})
	return nil
}

// handleMsg 归一化一条消息并落库(幂等);空 payload 记指标并直接 Ack,其他解码失败进死信但仍以摘要键落库。
func (m *NATSManager) handleMsg(ctx context.Context, msg *nats.Msg, platform, entityPath string, st *NATSNodeState) {
	emptyPayload := len(msg.Data) == 0
	ev, decodeErr := NormalizeEvent(platform, msg.Subject, msg.Data, msg.Header, entityPath)
	m.mu.Lock()
	if emptyPayload {
		st.EmptyDrops++
	} else if decodeErr != nil {
		st.DecodeErr++
	}
	m.mu.Unlock()
	result, err := m.persistEventResult(ctx, msg, ev, decodeErr)
	if err != nil {
		m.logError("[nats] 事件持久化失败", loggateway.Str("uid", ev.UID), loggateway.Err(err))
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if emptyPayload {
		return
	}
	st.LastMsgAt = ev.ReceivedAt
	if result.Inserted || result.Spooled {
		st.MsgCount++
	} else {
		st.DupCount++
	}
}

type ackNakMessage interface {
	Ack(...nats.AckOpt) error
	NakWithDelay(time.Duration, ...nats.AckOpt) error
}

func (m *NATSManager) persistEvent(ctx context.Context, msg ackNakMessage, ev *store.Event) error {
	_, err := m.persistEventResult(ctx, msg, ev, nil)
	return err
}

func (m *NATSManager) persistEventResult(ctx context.Context, msg ackNakMessage, ev *store.Event, decodeErr error) (store.IngestResult, error) {
	if len(ev.Payload) == 0 && decodeErr != nil {
		_ = msg.Ack()
		return store.IngestResult{}, nil
	}
	if decodeErr != nil {
		// 死信落库失败不再 Nak(评审 2026-11-09 H3):死信是标注轨,事件本体的
		// 持久化在下方 IngestEvent(含 spool 兜底)——「仅死信写不进」时 Nak 会
		// 让该消息无限重投(原 PG 22021 毒消息死循环,现写入侧已消毒,此分支
		// 只剩瞬态 DB 故障,由 IngestEvent 的 spool 路径正确承接)。
		if _, err := m.st.AddDeadLetter(ctx, "nats-decode", ev.Platform, ev.UID, ev.Payload, decodeErr.Error()); err != nil {
			m.logError("[nats] 死信落库失败(事件本体仍走摄入/spool 兜底)",
				loggateway.Str("uid", ev.UID), loggateway.Err(err))
		}
	}
	result, err := m.st.IngestEvent(ctx, ev, nil)
	if err != nil {
		_ = msg.NakWithDelay(2 * time.Second)
		return result, err
	}
	_ = msg.Ack()
	return result, nil
}

// NormalizeEvent 把一条 NATS 消息归一化为 CEOEvent(store.Event)。
// event_uid = 平台:原去重键(信封 event_id > id > Nats-Msg-Id > 内容摘要)。
func NormalizeEvent(platform, subject string, body []byte, header nats.Header, entityPath string) (*store.Event, error) {
	ev := &store.Event{
		Platform:   platform,
		Subject:    subject,
		SubjectKey: SubjectKey(subject),
		ReceivedAt: time.Now(),
	}
	sum := sha256.Sum256(body)

	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		// 非 JSON:仍能落库(摘要键),但记解码错误进死信
		ev.EventID = header.Get("Nats-Msg-Id")
		if ev.EventID == "" {
			ev.EventID = "sha256:" + hex.EncodeToString(sum[:8])
		}
		ev.UID = platform + ":" + ev.EventID
		ev.Payload = string(body)
		return ev, err
	}

	ev.EventID = firstNonEmpty(
		strOf(doc["event_id"]), strOf(doc["id"]), header.Get("Nats-Msg-Id"), "sha256:"+hex.EncodeToString(sum[:8]),
	)
	ev.UID = platform + ":" + ev.EventID
	if entityPath != "" {
		ev.EntityID = strOf(digPath(doc, entityPath))
	}
	ev.OccurredAt = parseOccurredAt(doc)
	ev.Payload = string(body)
	return ev, nil
}

// digPath 按点路径取值(payload.signal_id → doc["payload"]["signal_id"])。
func digPath(doc map[string]any, path string) any {
	cur := any(doc)
	for _, seg := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[seg]
	}
	return cur
}

// parseOccurredAt 解析信封时间戳:occurred_at / timestamp / time,支持 RFC3339 与 unix(秒/毫秒)。
func parseOccurredAt(doc map[string]any) time.Time {
	for _, k := range []string{"occurred_at", "timestamp", "time"} {
		v, ok := doc[k]
		if !ok {
			continue
		}
		switch t := v.(type) {
		case string:
			if parsed, err := time.Parse(time.RFC3339Nano, t); err == nil {
				return parsed
			}
			if parsed, err := time.Parse(time.RFC3339, t); err == nil {
				return parsed
			}
		case float64:
			// 秒或毫秒启发:>1e12 按毫秒
			if t > 1e12 {
				return time.UnixMilli(int64(t))
			}
			return time.Unix(int64(t), 0)
		}
	}
	return time.Time{}
}

// sanitizeDurable 把节点键/subject 清洗为合法 durable 名(仅字母数字与 -)。
func sanitizeDurable(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}
