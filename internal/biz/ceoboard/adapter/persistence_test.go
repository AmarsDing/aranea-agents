// 持久化故障语义单测(移植自 ceo-dashboard/internal/adapter/persistence_test.go)。
// 核心协议:持久化失败必须 Nak(2s 退避)让 JetStream 重投/让源 503 重试,
// 只有落库或 spool fsync 成功才 Ack/202——任何路径不丢、不重。
// 移植差异:SQLite 触发器注障的表名随 ent 迁移改为 ceoboard_*;存储调用穿 ctx;
// store.Close → 关闭底层 *sql.DB(模拟宕机)。
package adapter

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"aranea-agents/internal/biz/ceoboard/store"
)

type recordingAck struct {
	acks     atomic.Int32
	naks     atomic.Int32
	nakDelay atomic.Int64
}

func (a *recordingAck) Ack(...nats.AckOpt) error { a.acks.Add(1); return nil }
func (a *recordingAck) NakWithDelay(delay time.Duration, _ ...nats.AckOpt) error {
	a.naks.Add(1)
	a.nakDelay.Store(int64(delay))
	return nil
}

func TestNATSDBFailureSpoolSuccessAcks(t *testing.T) {
	ctx := context.Background()
	st, db := openFileStore(t, filepath.Join(t.TempDir(), "events.db"))
	if err := st.ConfigureSpool(filepath.Join(t.TempDir(), "spool"), 4096, 4096); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil { // 模拟 DB 宕机
		t.Fatal(err)
	}
	mgr := NewNATSManager(st, nil, nil)
	ack := &recordingAck{}

	if err := mgr.persistEvent(ctx, ack, &store.Event{UID: "hcf:spool-ack", Platform: "hcf", Subject: "a", SubjectKey: "a"}); err != nil {
		t.Fatalf("spool 成功后不应重投: %v", err)
	}
	if ack.acks.Load() != 1 || ack.naks.Load() != 0 {
		t.Fatalf("spool fsync 成功应 Ack 一次: ack=%d nak=%d", ack.acks.Load(), ack.naks.Load())
	}
}

func TestNATSDBAndSpoolFailureDoesNotAck(t *testing.T) {
	ctx := context.Background()
	st, db := openFileStore(t, filepath.Join(t.TempDir(), "events.db"))
	if err := st.ConfigureSpool(filepath.Join(t.TempDir(), "spool"), 4096, 1); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil { // 模拟 DB 宕机
		t.Fatal(err)
	}
	mgr := NewNATSManager(st, nil, nil)
	ack := &recordingAck{}

	if err := mgr.persistEvent(ctx, ack, &store.Event{UID: "hcf:retry", Platform: "hcf", Subject: "a", SubjectKey: "a"}); err == nil {
		t.Fatal("双失败必须返回错误并持续重投")
	}
	if ack.acks.Load() != 0 || ack.naks.Load() != 1 {
		t.Fatalf("双失败不得 Ack,应 Nak: ack=%d nak=%d", ack.acks.Load(), ack.naks.Load())
	}
	if time.Duration(ack.nakDelay.Load()) != 2*time.Second {
		t.Fatalf("持久化失败 Nak 必须退避 2s,实际 %s", time.Duration(ack.nakDelay.Load()))
	}
}

func TestNATSEmptyPayloadAcksAndDropsWithoutPersistence(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	mgr := NewNATSManager(st, nil, nil)
	ack := &recordingAck{}
	ev, decodeErr := NormalizeEvent("hcf", "mm.intel.raw", nil, nil, "")
	if decodeErr == nil {
		t.Fatal("空 payload 应识别为解码失败")
	}
	if _, err := mgr.persistEventResult(ctx, ack, ev, decodeErr); err != nil {
		t.Fatalf("空 payload 应直接 Ack 丢弃: %v", err)
	}
	if ack.acks.Load() != 1 || ack.naks.Load() != 0 {
		t.Fatalf("空 payload 只能 Ack 不得 Nak: ack=%d nak=%d", ack.acks.Load(), ack.naks.Load())
	}
	if n, _ := st.TotalEvents(ctx); n != 0 {
		t.Fatalf("空 payload 不得写 events,实际 %d", n)
	}
	if n, _ := st.PendingDeadLetters(ctx); n != 0 {
		t.Fatalf("空 payload 不得写 dead_letters,实际 %d", n)
	}
}

func TestNATSEmptyPayloadHandleMsgOnlyCountsEmptyDrop(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	mgr := NewNATSManager(st, nil, nil)
	state := &NATSNodeState{}
	mgr.handleMsg(ctx, &nats.Msg{Subject: "mm.intel.raw"}, "hcf", "", state)
	if state.EmptyDrops != 1 || state.DecodeErr != 0 || state.MsgCount != 0 || state.DupCount != 0 || !state.LastMsgAt.IsZero() {
		t.Fatalf("空 payload 只能增加 empty_drops: %+v", state)
	}
	if n, _ := st.TotalEvents(ctx); n != 0 {
		t.Fatalf("空 payload 不得写 events,实际 %d", n)
	}
	if n, _ := st.PendingDeadLetters(ctx); n != 0 {
		t.Fatalf("空 payload 不得写 dead_letters,实际 %d", n)
	}
}

func TestNATSDecodeDeadLetterFailureNaksAndRetriesIdempotently(t *testing.T) {
	ctx := context.Background()
	st, db := openFileStore(t, filepath.Join(t.TempDir(), "events.db"))
	if _, err := db.Exec(`CREATE TRIGGER reject_dead_letter BEFORE INSERT ON ceoboard_dead_letters BEGIN SELECT RAISE(FAIL,'dead letter unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	mgr := NewNATSManager(st, nil, nil)
	ack := &recordingAck{}
	ev := &store.Event{UID: "hcf:poison", Platform: "hcf", Subject: "a", SubjectKey: "a", Payload: "{bad"}
	if _, err := mgr.persistEventResult(ctx, ack, ev, errors.New("bad json")); err == nil {
		t.Fatal("decode 死信写失败必须返回错误并 Nak")
	}
	if ack.acks.Load() != 0 || ack.naks.Load() != 1 {
		t.Fatalf("死信未持久化不得 Ack: ack=%d nak=%d", ack.acks.Load(), ack.naks.Load())
	}
	if _, err := db.Exec(`DROP TRIGGER reject_dead_letter`); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.persistEventResult(ctx, ack, ev, errors.New("bad json")); err != nil {
		t.Fatalf("恢复后重试应成功: %v", err)
	}
	if ack.acks.Load() != 1 || ack.naks.Load() != 1 {
		t.Fatalf("恢复后应只补一次 Ack: ack=%d nak=%d", ack.acks.Load(), ack.naks.Load())
	}
	if n, _ := st.PendingDeadLetters(ctx); n != 1 {
		t.Fatalf("decode 重试不得重复死信,实际 %d", n)
	}
	if n, _ := st.TotalEvents(ctx); n != 1 {
		t.Fatalf("decode 重试不得重复事件,实际 %d", n)
	}
}

type captureConsumerJS struct {
	*fakeJS
	cfg nats.ConsumerConfig
}

func (j *captureConsumerJS) AddConsumer(stream string, cfg *nats.ConsumerConfig) (*nats.ConsumerInfo, error) {
	j.cfg = *cfg
	return &nats.ConsumerInfo{}, nil
}

func TestNATSConsumerRedeliversDuringPersistenceOutage(t *testing.T) {
	mgr := NewNATSManager(openTestStore(t), nil, nil)
	var addCalls, fetchCalls atomic.Int32
	js := &captureConsumerJS{fakeJS: &fakeJS{addConsumerCalls: &addCalls, sub: &fakeSub{fetchCalls: &fetchCalls}}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := mgr.consumeSubject(ctx, js, "S", "D", "a", "hcf", "", &NATSNodeState{}); err != nil {
		t.Fatal(err)
	}
	if js.cfg.MaxDeliver != -1 {
		t.Fatalf("持久化故障需无限重投,MaxDeliver 应为 -1,实际 %d", js.cfg.MaxDeliver)
	}
}

type existingConsumerJS struct {
	*fakeJS
	updated nats.ConsumerConfig
}

func (j *existingConsumerJS) AddConsumer(stream string, cfg *nats.ConsumerConfig) (*nats.ConsumerInfo, error) {
	return nil, errors.New("consumer already exists")
}

func (j *existingConsumerJS) UpdateConsumer(stream string, cfg *nats.ConsumerConfig) (*nats.ConsumerInfo, error) {
	j.updated = *cfg
	return &nats.ConsumerInfo{}, nil
}

type legacyConsumerJS struct {
	*existingConsumerJS
}

func (j *legacyConsumerJS) AddConsumer(stream string, cfg *nats.ConsumerConfig) (*nats.ConsumerInfo, error) {
	return &nats.ConsumerInfo{Config: nats.ConsumerConfig{MaxDeliver: 3}}, nil
}

func TestNATSAddReturningLegacyConfigStillUpdates(t *testing.T) {
	mgr := NewNATSManager(openTestStore(t), nil, nil)
	var addCalls, fetchCalls atomic.Int32
	base := &existingConsumerJS{fakeJS: &fakeJS{addConsumerCalls: &addCalls, sub: &fakeSub{fetchCalls: &fetchCalls}}}
	js := &legacyConsumerJS{existingConsumerJS: base}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := mgr.consumeSubject(ctx, js, "S", "D", "a", "hcf", "", &NATSNodeState{}); err != nil {
		t.Fatal(err)
	}
	if base.updated.MaxDeliver != -1 {
		t.Fatalf("Add 返回旧配置时也必须 Update,实际 %+v", base.updated)
	}
}

func TestNATSExistingConsumerMigratesMaxDeliver(t *testing.T) {
	mgr := NewNATSManager(openTestStore(t), nil, nil)
	var addCalls, fetchCalls atomic.Int32
	js := &existingConsumerJS{fakeJS: &fakeJS{addConsumerCalls: &addCalls, sub: &fakeSub{fetchCalls: &fetchCalls}}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := mgr.consumeSubject(ctx, js, "S", "D", "a", "hcf", "", &NATSNodeState{}); err != nil {
		t.Fatal(err)
	}
	if js.updated.MaxDeliver != -1 {
		t.Fatalf("已有 durable 必须迁移为无限重投,实际 %+v", js.updated)
	}
}

func TestWebhookRejectsOversizedBodyWithoutPersisting(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	r := NewWebhookReceiver(st, "")
	body := bytes.Repeat([]byte("x"), (1<<20)+1)
	req := httptest.NewRequest(http.MethodPost, "/v1/ceoboard/webhooks/aranea", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	r.Handler("aranea").ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("超过 1 MiB 必须拒绝而非截断后 202,实际 %d", rec.Code)
	}
	if n, _ := st.TotalEvents(ctx); n != 0 {
		t.Fatalf("被拒绝的截断原文不得入库,实际 %d 行", n)
	}
}

func TestWebhookDeliveryFailureSpoolsEventButReturns503(t *testing.T) {
	ctx := context.Background()
	st, db := openFileStore(t, filepath.Join(t.TempDir(), "events.db"))
	if err := st.ConfigureSpool(filepath.Join(t.TempDir(), "spool"), 4096, 16*1024); err != nil {
		t.Fatal(err)
	}
	for _, ddl := range []string{
		`CREATE TRIGGER reject_delivery BEFORE INSERT ON ceoboard_webhook_deliveries BEGIN SELECT RAISE(FAIL,'delivery unavailable'); END`,
		`CREATE TRIGGER reject_event BEFORE INSERT ON ceoboard_events BEGIN SELECT RAISE(FAIL,'event unavailable'); END`,
	} {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatal(err)
		}
	}
	r := NewWebhookReceiver(st, "")
	send := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/ceoboard/webhooks/aranea", strings.NewReader(`{"event_id":"delivery-spool","event":"opportunity.created"}`))
		rec := httptest.NewRecorder()
		r.Handler("aranea").ServeHTTP(rec, req)
		return rec
	}
	if rec := send(); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("投递流水未持久化时即使 event 已 spool 也应 503,实际 %d", rec.Code)
	}
	for _, name := range []string{"reject_delivery", "reject_event"} {
		if _, err := db.Exec(`DROP TRIGGER ` + name); err != nil {
			t.Fatal(err)
		}
	}
	if rec := send(); rec.Code != http.StatusAccepted {
		t.Fatalf("恢复后重试应补齐投递并受理,实际 %d body=%s", rec.Code, rec.Body.String())
	}
	if rec := send(); rec.Code != http.StatusAccepted {
		t.Fatalf("重复重试应幂等受理,实际 %d", rec.Code)
	}
	if _, err := st.ImportSpool(ctx); err != nil {
		t.Fatal(err)
	}
	if n, _ := st.CountDeliveries(ctx, "aranea", "", time.Now().Add(-time.Hour)); n != 1 {
		t.Fatalf("重试后成功投递应为 1,实际 %d", n)
	}
	if n, _ := st.TotalEvents(ctx); n != 1 {
		t.Fatalf("重试与 spool 恢复后事件应只有 1 行,实际 %d", n)
	}
}

func TestWebhookBadJSONDeadLetterFailureRetriesWithoutDuplicates(t *testing.T) {
	ctx := context.Background()
	st, db := openFileStore(t, filepath.Join(t.TempDir(), "events.db"))
	if _, err := db.Exec(`CREATE TRIGGER reject_dead_letter BEFORE INSERT ON ceoboard_dead_letters BEGIN SELECT RAISE(FAIL,'dead letter unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	r := NewWebhookReceiver(st, "")
	send := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/ceoboard/webhooks/aranea", strings.NewReader(`{"event_id":"bad-json","event":`))
		rec := httptest.NewRecorder()
		r.Handler("aranea").ServeHTTP(rec, req)
		return rec
	}
	if rec := send(); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("必需死信写失败应 503,实际 %d", rec.Code)
	}
	if _, err := db.Exec(`DROP TRIGGER reject_dead_letter`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if rec := send(); rec.Code != http.StatusAccepted {
			t.Fatalf("恢复后第 %d 次重试应受理,实际 %d", i+1, rec.Code)
		}
	}
	if n, _ := st.PendingDeadLetters(ctx); n != 1 {
		t.Fatalf("重试补写死信必须幂等,实际 %d", n)
	}
	if deliveries, _ := st.RecentDeliveries(ctx, "aranea", 10); len(deliveries) != 1 {
		t.Fatalf("重复重试不得重复计投递,实际 %d", len(deliveries))
	}
	if n, _ := st.TotalEvents(ctx); n != 1 {
		t.Fatalf("重复重试不得重复计事件,实际 %d", n)
	}
}

func TestWebhookBadJSONStillPersistsRawEvent(t *testing.T) {
	ctx := context.Background()
	st, db := openFileStore(t, filepath.Join(t.TempDir(), "events.db"))
	if err := st.ConfigureSpool(filepath.Join(t.TempDir(), "spool"), 4096, 4096); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER reject_dead_letter BEFORE INSERT ON ceoboard_dead_letters BEGIN SELECT RAISE(FAIL,'dead letter unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	r := NewWebhookReceiver(st, "")
	body := []byte(`{not-json`)
	ev := r.Ingest(ctx, "aranea", make(http.Header), body)
	if ev.PersistErr == nil {
		t.Fatal("死信写失败必须让源重试,但不得妨碍 events 保存原文")
	}
	n, err := st.CountEvents(ctx, "aranea", "", time.Now().Add(-time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("坏 JSON 原文必须进入统一事件流: n=%d err=%v", n, err)
	}
}

func TestReplayPersistenceFailureDoesNotMarkReplayed(t *testing.T) {
	ctx := context.Background()
	st, db := openFileStore(t, filepath.Join(t.TempDir(), "events.db"))
	if err := st.ConfigureSpool(filepath.Join(t.TempDir(), "spool"), 4096, 1); err != nil {
		t.Fatal(err)
	}
	id, err := st.AddDeadLetter(ctx, "webhook-process", "aranea", "retry-raw", `{"event":"opportunity.created"}`, "bad")
	if err != nil {
		t.Fatal(err)
	}
	for _, ddl := range []string{
		`CREATE TRIGGER reject_delivery BEFORE INSERT ON ceoboard_webhook_deliveries BEGIN SELECT RAISE(FAIL,'delivery unavailable'); END`,
		`CREATE TRIGGER reject_event BEFORE INSERT ON ceoboard_events BEGIN SELECT RAISE(FAIL,'event unavailable'); END`,
	} {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatal(err)
		}
	}
	r := NewWebhookReceiver(st, "")
	if _, err := r.ReplayDeadLetter(ctx, id); err == nil {
		t.Fatal("重放持久化失败必须返回错误")
	}
	dl, err := st.GetDeadLetter(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if dl.ReplayedAt != nil {
		t.Fatal("持久化失败不得标记已重放")
	}
}

func TestWebhookDBAndSpoolFailureReturns503(t *testing.T) {
	st, db := openFileStore(t, filepath.Join(t.TempDir(), "events.db"))
	if err := st.ConfigureSpool(filepath.Join(t.TempDir(), "spool"), 4096, 1); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil { // 模拟 DB 宕机
		t.Fatal(err)
	}
	r := NewWebhookReceiver(st, "")
	req := httptest.NewRequest(http.MethodPost, "/v1/ceoboard/webhooks/aranea", strings.NewReader(`{"event_id":"dual-failure","event":"opportunity.created"}`))
	rec := httptest.NewRecorder()
	r.Handler("aranea").ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("DB 与 spool 双失败应 503,实际 %d body=%s", rec.Code, rec.Body.String())
	}
}
