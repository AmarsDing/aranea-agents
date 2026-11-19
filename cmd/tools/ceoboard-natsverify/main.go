// ceoboard-natsverify 一次性取证工具(整合 P2-10,自 ceo-dashboard/cmd/natsverify
// 语义等价移植):从真实 NT/HCF 流各取最近 N 条消息(临时 consumer,DeliverLast),
// 走与看板完全相同的归一化+落库路径(NormalizeEvent → InsertEvent),验证真实消费、
// 幂等重放去重。不写源系统、不 publish;仅消费(任务明示允许)。
//
// 与原版的口径差异:
//   - 存储 SQLite → PG/Ent(store.New 包装 entsql.OpenDB,与 cmd/admin 装配同款);
//   - InsertEvent/TotalEvents 全量穿 ctx(P2-5 起存储口径);
//   - -db sqlite 路径参数 → -dsn PG DSN(缺省环境变量 ARANEA_PG_DSN;皆空即退出,
//     凭据只经环境注入,不写进仓库);
//   - 评审 2026-11-09 D5:NATS 地址不再硬编码——-nt-url/-hcf-url 注入,缺省取
//     ARANEA_CEOBOARD_NT_NATS_URL / ARANEA_CEOBOARD_HCF_NATS_URL;单侧为空即跳过
//     该段并明示,两侧皆空 exit 2(与 DSN 处理同款);
//   - 评审 2026-11-09 D6:任一平台段全部候选失败(含死信自检失败)→ 非零退出,
//     摘要输出形态不变,仅返回码修正;
//   - 评审 2026-11-09 D8:每平台段末追加死信路径自检——malformed 消息走与生产
//     natsjs.go persistEventResult 完全相同的调用(NormalizeEvent 返回 decodeErr
//     → AddDeadLetter("nats-decode", …) → GetDeadLetter 断言往返 → MarkReplayed
//     清理),补齐原注释声称"identical path"却跳过死信分支的覆盖缺口。
//
// 用法:
//
//	go run ./cmd/tools/ceoboard-natsverify -dsn postgres://user:pass@127.0.0.1:55432/aranea?sslmode=disable [-nt-url nats://…] [-hcf-url nats://…] [-n 3]
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/nats-io/nats.go"

	"aranea-agents/internal/biz/ceoboard/adapter"
	"aranea-agents/internal/biz/ceoboard/store"
	"aranea-agents/internal/data/ent"
)

func drain(ctx context.Context, url, platform, stream, subject string, n int, st *store.Store) (inserted, dup int, err error) {
	nc, err := nats.Connect(url, nats.Timeout(3*time.Second), nats.Name("ceoboard-verify"))
	if err != nil {
		return 0, 0, err
	}
	defer nc.Close()
	js, err := nc.JetStream()
	if err != nil {
		return 0, 0, err
	}
	if stream == "" {
		stream, err = js.StreamNameBySubject(subject)
		if err != nil {
			return 0, 0, fmt.Errorf("stream 反查失败: %w", err)
		}
	}
	// 用完即弃的 durable(pull subscribe 需 durable 名;删除清理)
	name := fmt.Sprintf("ceoboard-verify-%d", time.Now().UnixNano())
	if _, err = js.AddConsumer(stream, &nats.ConsumerConfig{
		Name: name, Durable: name, FilterSubject: subject,
		DeliverPolicy: nats.DeliverLastPolicy, AckPolicy: nats.AckExplicitPolicy,
	}); err != nil {
		return 0, 0, err
	}
	defer func() { _ = js.DeleteConsumer(stream, name) }()
	sub, err := js.PullSubscribe(subject, name, nats.BindStream(stream))
	if err != nil {
		return 0, 0, err
	}
	msgs, err := sub.Fetch(n, nats.MaxWait(5*time.Second))
	if err != nil {
		return 0, 0, fmt.Errorf("拉取: %w", err)
	}
	for _, m := range msgs {
		ev, derr := adapter.NormalizeEvent(platform, m.Subject, m.Data, m.Header, "")
		ok, ierr := st.InsertEvent(ctx, ev)
		if ierr != nil {
			return inserted, dup, ierr
		}
		if ok {
			inserted++
		} else {
			dup++
		}
		occ := "(无)"
		if !ev.OccurredAt.IsZero() {
			occ = ev.OccurredAt.Format(time.RFC3339)
		}
		fmt.Printf("  [%s] uid=%s subject=%s key=%s occurred=%s decode_err=%v → %s\n",
			platform, ev.UID, ev.Subject, ev.SubjectKey, occ, derr != nil, map[bool]string{true: "新插入", false: "去重命中"}[ok])
		_ = m.Ack()
	}
	return inserted, dup, nil
}

// verifyDeadLetterPath 死信路径自检(评审 2026-11-09 D8):构造一条 malformed
// 消息,走与生产 natsjs.go persistEventResult 完全相同的 store 调用序列——
// NormalizeEvent 返回 decodeErr → AddDeadLetter("nats-decode", platform, uid,
// payload, decodeErr) → GetDeadLetter 断言原文往返 → MarkReplayed 清理(该行
// 留库但已 replayed,不进 pending 口径)。payload 含纳秒时间戳,uid 天然唯一,
// 重复运行不构成 (kind,source,ref_key) 未重放冲突。
func verifyDeadLetterPath(ctx context.Context, st *store.Store, platform string) error {
	payload := fmt.Sprintf(`{"natsverify-selfcheck":%d, "truncated":`, time.Now().UnixNano())
	ev, decodeErr := adapter.NormalizeEvent(platform, "verify.selfcheck", []byte(payload), nil, "")
	if decodeErr == nil {
		return fmt.Errorf("malformed 消息应产生 decodeErr")
	}
	id, err := st.AddDeadLetter(ctx, "nats-decode", platform, ev.UID, ev.Payload, decodeErr.Error())
	if err != nil {
		return fmt.Errorf("死信写入: %w", err)
	}
	dl, err := st.GetDeadLetter(ctx, id)
	if err != nil {
		return fmt.Errorf("死信读回: %w", err)
	}
	if dl.Payload != ev.Payload || dl.Kind != "nats-decode" || dl.Reason == "" || dl.ReplayedAt != nil {
		return fmt.Errorf("死信往返失真: %+v", dl)
	}
	pending, err := st.PendingDeadLetters(ctx)
	if err != nil || pending < 1 {
		return fmt.Errorf("pending 死信应含自检行: pending=%d err=%v", pending, err)
	}
	if err := st.MarkReplayed(ctx, id); err != nil {
		return fmt.Errorf("死信标重放: %w", err)
	}
	return nil
}

// verifySection 跑一个平台段:逐个候选 subject 取证(NATS 不可达/无流时全部
// 失败),段末追加死信路径自检。返回 false = 该段失败(全部候选 drain 失败或
// 死信自检失败)。
func verifySection(ctx context.Context, st *store.Store, platform, url, stream string, candidates []string, n int, w io.Writer) bool {
	drained := false
	for _, subj := range candidates {
		ins, dup, err := drain(ctx, url, platform, stream, subj, n, st)
		if err != nil {
			fmt.Fprintf(w, "  %s: %v\n", subj, err)
			continue
		}
		drained = true
		fmt.Fprintf(w, "  %s: 新插入 %d,去重命中 %d\n", subj, ins, dup)
	}
	if !drained {
		fmt.Fprintln(w, "  所有候选 subject 均失败(NATS 不可达或无流)")
		return false
	}
	if err := verifyDeadLetterPath(ctx, st, platform); err != nil {
		fmt.Fprintf(w, "  死信路径自检失败: %v\n", err)
		return false
	}
	fmt.Fprintln(w, "  死信路径自检通过")
	return true
}

// run 汇总退出码(D6):已启用(提供 URL)的平台段任一失败 → 1,全通过 → 0;
// 未启用的段跳过并明示,不影响退出码(纯函数,供测试)。
func run(ctx context.Context, st *store.Store, ntURL, hcfURL string, n int, w io.Writer) int {
	failed := false
	if ntURL == "" {
		fmt.Fprintln(w, "=== NovaTrader:跳过(未提供 -nt-url / ARANEA_CEOBOARD_NT_NATS_URL) ===")
	} else {
		fmt.Fprintln(w, "=== NovaTrader(trade.> / sys.> / risk.> 等) ===")
		if !verifySection(ctx, st, "novatrader", ntURL, "NOVATRADER",
			[]string{"trade.fill", "trade.order", "trade.reconciliation", "risk.alert", "strategy.signal.updated", "notify.>", "intel.>", "md.>"}, n, w) {
			failed = true
		}
	}
	if hcfURL == "" {
		fmt.Fprintln(w, "=== HCF:跳过(未提供 -hcf-url / ARANEA_CEOBOARD_HCF_NATS_URL) ===")
	} else {
		fmt.Fprintln(w, "=== HCF(mm.intel.> / mm.trend.>) ===")
		if !verifySection(ctx, st, "hcf", hcfURL, "", []string{"mm.intel.>", "mm.trend.>"}, n, w) {
			failed = true
		}
	}
	total, _ := st.TotalEvents(ctx)
	fmt.Fprintf(w, "事件表总数: %d\n", total)
	if failed {
		return 1
	}
	return 0
}

func main() {
	dsnFlag := flag.String("dsn", "", "PG DSN(F 盘部署环境一般为 127.0.0.1:55432);缺省取 ARANEA_PG_DSN")
	ntURL := flag.String("nt-url", "", "NovaTrader NATS 地址;缺省取 ARANEA_CEOBOARD_NT_NATS_URL")
	hcfURL := flag.String("hcf-url", "", "HCF NATS 地址;缺省取 ARANEA_CEOBOARD_HCF_NATS_URL")
	n := flag.Int("n", 3, "每个 subject 拉取条数")
	flag.Parse()
	dsn := *dsnFlag
	if dsn == "" {
		dsn = os.Getenv("ARANEA_PG_DSN")
	}
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "缺 PG 连接串:传 -dsn 或设 ARANEA_PG_DSN 环境变量")
		os.Exit(2)
	}
	// D5:NATS 地址经 flag/环境注入,不再硬编码;两侧皆空无活可干,exit 2 与 DSN 同款。
	if *ntURL == "" {
		*ntURL = os.Getenv("ARANEA_CEOBOARD_NT_NATS_URL")
	}
	if *hcfURL == "" {
		*hcfURL = os.Getenv("ARANEA_CEOBOARD_HCF_NATS_URL")
	}
	if *ntURL == "" && *hcfURL == "" {
		fmt.Fprintln(os.Stderr, "缺 NATS 地址:传 -nt-url/-hcf-url 或设 ARANEA_CEOBOARD_NT_NATS_URL/ARANEA_CEOBOARD_HCF_NATS_URL")
		os.Exit(2)
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer db.Close()
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	defer client.Close()
	st := store.New(client, db)

	os.Exit(run(context.Background(), st, *ntURL, *hcfURL, *n, os.Stdout))
}
