// 一次性排查脚本：统计 skill_invocation 按 source 的分布，确认「未知 Agent / 计数膨胀」根因。
package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq"
)

func main() {
	dsn := "postgres://postgres:Hangshan%40123@127.0.0.1:5432/aranea?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fmt.Println("open:", err)
		os.Exit(1)
	}
	defer db.Close()

	fmt.Println("== 按 source 分布 ==")
	rows, err := db.Query(`SELECT COALESCE(NULLIF(source,''),'(empty)'), COUNT(*),
		COUNT(*) FILTER (WHERE COALESCE(agent_id,'') <> '') AS with_agent,
		MAX(COALESCE(NULLIF(started_at,''), created_at)) AS latest
		FROM skill_invocation GROUP BY 1 ORDER BY 2 DESC`)
	if err != nil {
		fmt.Println("query:", err)
		os.Exit(1)
	}
	for rows.Next() {
		var src string
		var cnt, withAgent int
		var latest string
		if err := rows.Scan(&src, &cnt, &withAgent, &latest); err != nil {
			fmt.Println("scan:", err)
			os.Exit(1)
		}
		fmt.Printf("source=%-22s count=%-6d with_agent=%-6d latest=%s\n", src, cnt, withAgent, latest)
	}
	rows.Close()

	fmt.Println("\n== 每个 skill 最新一条 runtime 调用 vs 最新一条任意调用 ==")
	rows2, err := db.Query(`SELECT si.skill_id,
		MAX(CASE WHEN si.source='runtime' THEN COALESCE(NULLIF(si.started_at,''),si.created_at) END) AS latest_runtime,
		MAX(COALESCE(NULLIF(si.started_at,''),si.created_at)) AS latest_any
		FROM skill_invocation si GROUP BY si.skill_id ORDER BY latest_any DESC LIMIT 10`)
	if err != nil {
		fmt.Println("query2:", err)
		os.Exit(1)
	}
	for rows2.Next() {
		var sid string
		var lr, la sql.NullString
		if err := rows2.Scan(&sid, &lr, &la); err != nil {
			fmt.Println("scan2:", err)
			os.Exit(1)
		}
		fmt.Printf("skill=%s latest_runtime=%v latest_any=%v\n", sid, lr.String, la.String)
	}
	rows2.Close()

	fmt.Println("\n== 标签字典（skill_tags） ==")
	rows4, err := db.Query(`SELECT name, COALESCE(dimension,''), COALESCE(source,'') FROM skill_tags ORDER BY dimension, name LIMIT 100`)
	if err != nil {
		fmt.Println("query4:", err)
	} else {
		for rows4.Next() {
			var name, dim, src string
			if err := rows4.Scan(&name, &dim, &src); err != nil {
				fmt.Println("scan4:", err)
				break
			}
			fmt.Printf("name=%-32s dim=%-12s source=%-8s\n", name, dim, src)
		}
		rows4.Close()
	}

	fmt.Println("\n== 实际使用中的标签（skill metadata/config 原文样本） ==")
	rows5, err := db.Query(`SELECT name, left(metadata_json, 400), left(config_json, 300) FROM skill WHERE metadata_json LIKE '%tags%' OR config_json LIKE '%tags%' LIMIT 6`)
	if err != nil {
		fmt.Println("query5:", err)
	} else {
		for rows5.Next() {
			var tag string
			var m, c string
			if err := rows5.Scan(&tag, &m, &c); err != nil {
				fmt.Println("scan5:", err)
				break
			}
			fmt.Printf("skill=%s\n  meta=%s\n  config=%s\n", tag, m, c)
		}
		rows5.Close()
	}

	fmt.Println("\n== skill 名称/描述样本 ==")
	rows6, err := db.Query(`SELECT name, skill_key, left(description, 200) FROM skill ORDER BY updated_at DESC LIMIT 30`)
	if err != nil {
		fmt.Println("query6:", err)
	} else {
		for rows6.Next() {
			var n, s, d string
			if err := rows6.Scan(&n, &s, &d); err != nil {
				fmt.Println("scan6:", err)
				break
			}
			fmt.Printf("name=%s | slug=%s\n  desc=%s\n", n, s, d)
		}
		rows6.Close()
	}

	fmt.Println("\n== runtime 调用样本（agent_id 是否为空） ==")
	rows3, err := db.Query(`SELECT skill_id, COALESCE(agent_id,'(empty)'), status, COALESCE(NULLIF(started_at,''),created_at)
		FROM skill_invocation WHERE source='runtime' ORDER BY created_at DESC LIMIT 10`)
	if err != nil {
		fmt.Println("query3:", err)
		os.Exit(1)
	}
	for rows3.Next() {
		var sid, aid, st, ts string
		if err := rows3.Scan(&sid, &aid, &st, &ts); err != nil {
			fmt.Println("scan3:", err)
			os.Exit(1)
		}
		fmt.Printf("skill=%s agent=%s status=%s at=%s\n", sid, aid, st, ts)
	}
	rows3.Close()

	fmt.Println("\n== tool 表 skill 类工具分类（classifyToolInvocation 判定依据） ==")
	rows7, err := db.Query(`SELECT tool_key, COALESCE(source,''), COALESCE(category,'') FROM tools
		WHERE tool_key ILIKE '%skill%' OR source ILIKE '%skill%' OR category ILIKE '%skill%' LIMIT 30`)
	if err != nil {
		fmt.Println("query7:", err)
	} else {
		n := 0
		for rows7.Next() {
			var k, s, c string
			if err := rows7.Scan(&k, &s, &c); err != nil {
				fmt.Println("scan7:", err)
				break
			}
			fmt.Printf("tool=%-36s source=%-10s category=%-10s\n", k, s, c)
			n++
		}
		if n == 0 {
			fmt.Println("(no rows)")
		}
		rows7.Close()
	}

	fmt.Println("\n== tool_invocation 中 skill 相关调用 ==")
	rows8, err := db.Query(`SELECT tool_key, COUNT(*), MAX(COALESCE(NULLIF(started_at,''),created_at)) FROM tool_invocations
		WHERE tool_key ILIKE '%skill%' GROUP BY tool_key ORDER BY 2 DESC LIMIT 10`)
	if err != nil {
		fmt.Println("query8:", err)
	} else {
		n := 0
		for rows8.Next() {
			var k, ts string
			var cnt int
			if err := rows8.Scan(&k, &cnt, &ts); err != nil {
				fmt.Println("scan8:", err)
				break
			}
			fmt.Printf("tool=%-36s count=%-6d latest=%s\n", k, cnt, ts)
			n++
		}
		if n == 0 {
			fmt.Println("(no rows)")
		}
		rows8.Close()
	}
}
