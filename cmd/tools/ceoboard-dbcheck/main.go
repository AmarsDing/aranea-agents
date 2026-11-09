// ceoboard-dbcheck 一次性诊断(整合 P2-10,自 ceo-dashboard/cmd/dbcheck 移植):
// 打印 ceoboard_events / ceoboard_latency_samples / ceoboard_audit 概览。
//
// 与原版的口径差异:
//   - 存储 SQLite → PG(tables ceoboard_*;received_at/at 由 unix 纳秒 int64
//     改 timestamptz,直接按 time.Time 扫);
//   - sessions 段删除——sessions 表随 P1 废止(JWT 无状态 + token_version 吊销),
//     同时这也是 mkpasswd 废止的原因(见 P2 任务分解 切片D-2 登记);
//   - 评审 2026-11-09 D7:Scan 错误与 rows.Err() 全链路检查——原实现两处
//     rows.Scan 的返回被丢弃、rows.Err() 未查,列类型漂移/连接中断会静默输出
//     残缺概览且 exit 0。现任何一节失败 → stderr 留痕 + 该节跳过,继续其余节,
//     进程末统一 exit 1(不首错即退:诊断工具的价值在于看全三段)。
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
	"io"
	"os"
	"strings"
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

// scanRows 逐行执行 scan 回调并统一上报两类错误(评审 2026-11-09 D7):
// 行内 Scan 失败与迭代期 rows.Err()(连接中断/驱动错误)。任一失败即中止该节
// 并返回错误,杜绝「扫到一半静默残缺」的旧行为。
func scanRows(rows *sql.Rows, scan func() error) error {
	defer rows.Close()
	for rows.Next() {
		if err := scan(); err != nil {
			return err
		}
	}
	return rows.Err()
}

// asWallClock 把 MAX(received_at) 一类聚合结果归一为 time.Time:PG 驱动直返
// time.Time;sqlite 对 TIMESTAMP 聚合返回文本(PG 文本形态/RFC3339 均可解析)。
// 无法归一的值返回错误——诊断工具的残缺/毒值仍按 D7 上浮,不静默。
func asWallClock(v any) (time.Time, error) {
	switch t := v.(type) {
	case time.Time:
		return t, nil
	case nil:
		return time.Time{}, nil
	case string:
		if t == "" {
			return time.Time{}, nil
		}
		// modernc/sqlite 把 driver 侧 time.Time 落为 Go String() 形态(带单调钟
		// 后缀 " m=+…",RFC 解析不了),剥掉再试。
		if cut, _, ok := strings.Cut(t, " m=+"); ok {
			t = strings.TrimSpace(cut)
		}
		for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999 -0700 MST", "2006-01-02 15:04:05.999999999-07", "2006-01-02 15:04:05.999999999", "2006-01-02 15:04:05"} {
			if parsed, err := time.Parse(layout, t); err == nil {
				return parsed, nil
			}
		}
		return time.Time{}, fmt.Errorf("无法解析时间聚合值 %q", t)
	default:
		return time.Time{}, fmt.Errorf("无法解析时间聚合值 %T", v)
	}
}

// eventsSection 打印 ceoboard_events 平台/subject 分布(返回错误即该节失败)。
func eventsSection(db *sql.DB, w io.Writer) error {
	rows, err := db.Query(`SELECT platform, subject_key, COUNT(*), MAX(received_at) FROM ceoboard_events GROUP BY 1,2 ORDER BY 3 DESC`)
	if err != nil {
		return err
	}
	return scanRows(rows, func() error {
		var p, k string
		var n int
		var rawMax any
		if err := rows.Scan(&p, &k, &n, &rawMax); err != nil {
			return err
		}
		maxAt, err := asWallClock(rawMax)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(w, "  %-12s %-40s %5d  最近 %s\n", p, k, n, maxAt.Format("15:04:05"))
		return err
	})
}

// latencySection 打印 ceoboard_latency_samples 指标分布。
func latencySection(db *sql.DB, w io.Writer) error {
	rows, err := db.Query(`SELECT metric_id, COUNT(*), MIN(latency_ms), MAX(latency_ms) FROM ceoboard_latency_samples GROUP BY 1`)
	if err != nil {
		return err
	}
	n := 0
	err = scanRows(rows, func() error {
		var m string
		var cnt int
		var mn, mx int64
		if err := rows.Scan(&m, &cnt, &mn, &mx); err != nil {
			return err
		}
		n++
		_, err := fmt.Fprintf(w, "  %s: %d 样本 [%dms, %dms]\n", m, cnt, mn, mx)
		return err
	})
	if err != nil {
		return err
	}
	if n == 0 {
		fmt.Fprintln(w, "  (空)")
	}
	return nil
}

// auditSection 打印 ceoboard_audit 最近 5 条。
func auditSection(db *sql.DB, w io.Writer) error {
	rows, err := db.Query(`SELECT at, actor, action, ok, detail FROM ceoboard_audit ORDER BY at DESC LIMIT 5`)
	if err != nil {
		return err
	}
	return scanRows(rows, func() error {
		var at time.Time
		var actor, action, detail string
		var ok bool
		if err := rows.Scan(&at, &actor, &action, &ok, &detail); err != nil {
			return err
		}
		_, err := fmt.Fprintf(w, "  %s %-10s %-18s ok=%v %s\n", at.Format("15:04:05"), actor, action, ok, detail)
		return err
	})
}

// run 逐节诊断并汇总退出码:任一节失败 → stderr 留痕、继续其余节,末态 exit 1。
func run(db *sql.DB, w io.Writer, errw io.Writer) int {
	failed := false
	fmt.Fprintln(w, "== ceoboard_events 分布(platform/subject_key) ==")
	if err := eventsSection(db, w); err != nil {
		fmt.Fprintf(errw, "ceoboard_events 段失败: %v\n", err)
		failed = true
	}
	fmt.Fprintln(w, "== ceoboard_latency_samples ==")
	if err := latencySection(db, w); err != nil {
		fmt.Fprintf(errw, "ceoboard_latency_samples 段失败: %v\n", err)
		failed = true
	}
	fmt.Fprintln(w, "== ceoboard_audit 最近 5 条 ==")
	if err := auditSection(db, w); err != nil {
		fmt.Fprintf(errw, "ceoboard_audit 段失败: %v\n", err)
		failed = true
	}
	fmt.Fprintln(w, "== sessions:已废止(P1 JWT 无状态),不再统计 ==")
	if failed {
		return 1
	}
	return 0
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

	os.Exit(run(db, os.Stdout, os.Stderr))
}
