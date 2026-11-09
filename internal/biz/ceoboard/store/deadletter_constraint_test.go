// 死信幂等库层兜底测试(评审 2026-11-09 M2):
//   - 部分唯一索引 ceoboard_dead_letters_unreplayed_ref_uidx 硬约束生效,
//     且 ent.IsConstraintError 可识别(兜底分支可触发性);
//   - 索引谓词边界:ref_key=” 与已重放行不参与幂等;
//   - 双 Store 实例(无共享 dlMu)压同库:全部幂等收敛为同一 id、零错误——
//     无论命中"先查后插"还是唯一冲突兜底路(迁移/约束语义与 PG 侧同语法)。
package store

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/data/ent"
)

// 部分唯一索引硬约束:绕开 store 直插同 (kind,source,ref_key) 未重放行必冲突。
func TestDeadLetter_PartialUniqueIndexEnforced(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	create := func(refKey string) error {
		return s.client.CeoboardDeadLetter.Create().
			SetKind("webhook-process").
			SetSource("aranea").
			SetRefKey(refKey).
			SetPayload("{}").
			SetReason("r").
			SetFailedAt(time.Now().UTC()).
			Exec(ctx)
	}
	if err := create("k-1"); err != nil {
		t.Fatalf("首插应成功: %v", err)
	}
	err := create("k-1")
	if err == nil {
		t.Fatal("同 (kind,source,ref_key) 未重放直插必须被唯一索引拦下")
	}
	// 兜底分支依赖 ISConstraintError 识别约束冲突(PG 23505 / SQLite 同语义)。
	if !ent.IsConstraintError(err) {
		t.Fatalf("冲突应被 ent.IsConstraintError 识别,实际 %v", err)
	}

	// 已重放行释放该键:允许产生新一轮死信。
	row, err := s.client.CeoboardDeadLetter.Query().Only(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.client.CeoboardDeadLetter.UpdateOneID(row.ID).
		SetReplayedAt(time.Now().UTC()).
		Exec(ctx); err != nil {
		t.Fatal(err)
	}
	if err := create("k-1"); err != nil {
		t.Fatalf("旧行已重放后同键应可再插: %v", err)
	}

	// ref_key=''(receive 阶段无业务键)不参与幂等,可重复。
	if err := create(""); err != nil {
		t.Fatal(err)
	}
	if err := create(""); err != nil {
		t.Fatalf("ref_key='' 不受唯一约束,实际 %v", err)
	}
}

// 双实例竞态收敛:两个 Store(各自 dlMu,不互斥)并发写同 (kind,source,ref_key)——
// 查重窗口被另一实例抢插时走唯一冲突兜底重查,与先查后插等幂。
// 不变量(逐轮断言,不依赖是否真撞窗口):双方零错误、返回同一 id、每键仅一行。
func TestAddDeadLetter_CrossInstanceRaceConverges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dl-race.db")
	s1, db1 := openFileStore(t, path)
	s2, db2 := openFileStore(t, path)
	t.Cleanup(func() { s1.client.Close(); s2.client.Close(); db1.Close(); db2.Close() })
	ctx := context.Background()

	const rounds = 200
	for i := 0; i < rounds; i++ {
		key := fmt.Sprintf("race-%d", i)
		var wg sync.WaitGroup
		ids := [2]int64{}
		errs := [2]error{}
		wg.Add(2)
		go func() {
			defer wg.Done()
			ids[0], errs[0] = s1.AddDeadLetter(ctx, "webhook-process", "aranea", key, "{}", "r1")
		}()
		go func() {
			defer wg.Done()
			ids[1], errs[1] = s2.AddDeadLetter(ctx, "webhook-process", "aranea", key, "{}", "r2")
		}()
		wg.Wait()
		if errs[0] != nil || errs[1] != nil {
			t.Fatalf("第 %d 轮并发写死信不应报错: %v / %v", i, errs[0], errs[1])
		}
		if ids[0] != ids[1] {
			t.Fatalf("第 %d 轮同键应幂等收敛同一 id,实际 %d / %d", i, ids[0], ids[1])
		}
	}
	// 每键恰一行未重放。
	if n, err := s1.PendingDeadLetters(ctx); err != nil || n != rounds {
		t.Fatalf("未重放死信应为 %d(每键一行),实际 %d err=%v", rounds, n, err)
	}
}
