// ceoboard-dbcheck 一次性诊断(整合 P2-10,自 ceo-dashboard/cmd/dbcheck 移植):
// 打印 ceoboard_events / ceoboard_latency_samples / ceoboard_audit 概览。
//
// 与原版的口径差异:
//   - 存储 SQLite → PG(tables ceoboard_*;received_at/at 由 unix 纳秒 int64
//     改 timestamptz,直接按 time.Time 扫);
//   - sessions 段删除——sessions 表随 P1 废止(JWT 无状态 + token_version 吊销),
//     同时这也是 mkpasswd 废止的原因(见 P2 任务分解 切片D-2 登记)。
//
// 用法:
//
//	go run ./cmd/tools/ceoboard-dbcheck -dsn postgres://user:pass@127.0.0.1:55432/aranea?sslmode=disable
//	DSN 缺省取环境变量 ARANEA_PG_DSN;两者都缺即退出(不在仓内落任何凭据)。
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"time"

	_ "github.com/lib/pq"
)

// resolveDSN 取 PG 连接串:-dsn flag 优先,其次 ARANEA_PG_DSN 环境变量;
// 皆空即报错退出——凭据只经环境注入,不写进仓库(区别于历史 dbprobe 的硬编码默认值)。
func resolveDSN(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	return os.Getenv("ARANEA_PG_DSN")
}

func main() {
	dsnFlag := flag.String("dsn", "", "PG DSN(F 盘部署环境一般为 127.0.0.1:55432);缺省取 ARANEA_PG_DSN")
	flag.Parse()
	dsn := resolveDSN(*dsnFlag)
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
	if err := db.Ping(); err != nil {
		fmt.Fprintln(os.Stderr, "ping:", err)
		os.Exit(1)
	}

	fmt.Println("== ceoboard_events 分布(platform/subject_key) ==")
	rows, err := db.Query(`SELECT platform, subject_key, COUNT(*), MAX(received_at) FROM ceoboard_events GROUP BY 1,2 ORDER BY 3 DESC`)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for rows.Next() {
		var p, k string
		var n int
		var maxAt time.Time
		rows.Scan(&p, &k, &n, &maxAt)
		fmt.Printf("  %-12s %-40s %5d  最近 %s\n", p, k, n, maxAt.Format("15:04:05"))
	}
	rows.Close()

	fmt.Println("== ceoboard_latency_samples ==")
	rows2, err := db.Query(`SELECT metric_id, COUNT(*), MIN(latency_ms), MAX(latency_ms) FROM ceoboard_latency_samples GROUP BY 1`)
	if err == nil {
		any := false
		for rows2.Next() {
			var m string
			var n int
			var mn, mx int64
			rows2.Scan(&m, &n, &mn, &mx)
			fmt.Printf("  %s: %d 样本 [%dms, %dms]\n", m, n, mn, mx)
			any = true
		}
		if !any {
			fmt.Println("  (空)")
		}
		rows2.Close()
	} else {
		fmt.Println("  查询失败:", err)
	}

	fmt.Println("== ceoboard_audit 最近 5 条 ==")
	rows3, err := db.Query(`SELECT at, actor, action, ok, detail FROM ceoboard_audit ORDER BY at DESC LIMIT 5`)
	if err == nil {
		for rows3.Next() {
			var at time.Time
			var actor, action, detail string
			var ok bool
			rows3.Scan(&at, &actor, &action, &ok, &detail)
			fmt.Printf("  %s %-10s %-18s ok=%v %s\n", at.Format("15:04:05"), actor, action, ok, detail)
		}
		rows3.Close()
	}

	fmt.Println("== sessions:已废止(P1 JWT 无状态),不再统计 ==")
}
