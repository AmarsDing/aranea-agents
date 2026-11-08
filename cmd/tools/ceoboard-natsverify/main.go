// ceoboard-natsverify 一次性取证工具(整合 P2-10,自 ceo-dashboard/cmd/natsverify
// 语义等价移植):从真实 NT/HCF 流各取最近 N 条消息(临时 consumer,DeliverLast),
// 走与看板完全相同的归一化+落库路径(NormalizeEvent → InsertEvent),验证真实消费、
// 幂等重放去重。不写源系统、不 publish;仅消费(任务明示允许)。
//
// 与原版的口径差异:
//   - 存储 SQLite → PG/Ent(store.New 包装 entsql.OpenDB,与 cmd/admin 装配同款);
//   - InsertEvent/TotalEvents 全量穿 ctx(P2-5 起存储口径);
//   - -db sqlite 路径参数 → -dsn PG DSN(缺省环境变量 ARANEA_PG_DSN;皆空即退出,
//     凭据只经环境注入,不写进仓库)。
//
// 用法:
//
//	go run ./cmd/tools/ceoboard-natsverify -dsn postgres://user:pass@127.0.0.1:55432/aranea?sslmode=disable [-n 3]
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
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

func main() {
	dsnFlag := flag.String("dsn", "", "PG DSN(F 盘部署环境一般为 127.0.0.1:55432);缺省取 ARANEA_PG_DSN")
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

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer db.Close()
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	defer client.Close()
	st := store.New(client, db)
	ctx := context.Background()

	// NT:候选 subject 逐个试,直到取到真实消息
	fmt.Println("=== NovaTrader(trade.> / sys.> / risk.> 等) ===")
	ntOK := false
	for _, sub := range []string{"trade.fill", "trade.order", "trade.reconciliation", "risk.alert", "strategy.signal.updated", "notify.>", "intel.>", "md.>"} {
		ins, dup, err := drain(ctx, "nats://192.168.4.44:24222", "novatrader", "NOVATRADER", sub, *n, st)
		if err != nil {
			fmt.Printf("  %s: %v\n", sub, err)
			continue
		}
		fmt.Printf("  %s: 新插入 %d,去重命中 %d\n", sub, ins, dup)
		if ins+dup > 0 {
			ntOK = true
		}
	}
	if !ntOK {
		fmt.Println("  所有候选 subject 均无消息")
	}

	fmt.Println("=== HCF(mm.intel.> / mm.trend.>) ===")
	for _, it := range [][2]string{{"MM_INTEL", "mm.intel.>"}, {"MM_TREND", "mm.trend.>"}} {
		ins, dup, err := drain(ctx, "nats://192.168.4.44:14222", "hcf", it[0], it[1], *n, st)
		if err != nil {
			fmt.Printf("  %s/%s: %v\n", it[0], it[1], err)
			continue
		}
		fmt.Printf("  %s/%s: 新插入 %d,去重命中 %d\n", it[0], it[1], ins, dup)
	}
	total, _ := st.TotalEvents(ctx)
	fmt.Printf("事件表总数: %d\n", total)
}
