package main

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

func main() {
	dsn := "postgres://postgres:Hangshan%40123@127.0.0.1:5432/aranea?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		panic(err)
	}

	fmt.Println("== agents count by kind ==")
	rows, err := db.Query(`SELECT kind, count(*) FROM agents WHERE deleted_at='' GROUP BY kind ORDER BY 2 DESC`)
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		var k string
		var c int
		rows.Scan(&k, &c)
		fmt.Printf("kind=%s count=%d\n", k, c)
	}
	rows.Close()

	fmt.Println("\n== agents count by variant ==")
	rows, err = db.Query(`SELECT coalesce(agent_variant,'<null>'), count(*) FROM agents WHERE deleted_at='' GROUP BY 1 ORDER BY 2 DESC`)
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		var v string
		var c int
		rows.Scan(&v, &c)
		fmt.Printf("variant=%s count=%d\n", v, c)
	}
	rows.Close()

	fmt.Println("\n== is_default distribution ==")
	rows, err = db.Query(`SELECT is_default, count(*) FROM agents WHERE deleted_at='' GROUP BY 1`)
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		var d bool
		var c int
		rows.Scan(&d, &c)
		fmt.Printf("is_default=%v count=%d\n", d, c)
	}
	rows.Close()

	fmt.Println("\n== position empty (non-system) ==")
	rows, err = db.Query(`SELECT (coalesce(position_id,'')='') AS empty_pid, count(*) FROM agents WHERE deleted_at='' AND kind != 'system_builtin' GROUP BY 1`)
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		var e bool
		var c int
		rows.Scan(&e, &c)
		fmt.Printf("empty_position_id=%v count=%d\n", e, c)
	}
	rows.Close()

	fmt.Println("\n== updated_at duplicates (top 10) ==")
	rows, err = db.Query(`SELECT updated_at, count(*) FROM agents WHERE deleted_at='' GROUP BY 1 HAVING count(*)>1 ORDER BY 2 DESC LIMIT 10`)
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		var u string
		var c int
		rows.Scan(&u, &c)
		fmt.Printf("updated_at=%s count=%d\n", u, c)
	}
	rows.Close()

	fmt.Println("\n== all agents ==")
	rows, err = db.Query(`SELECT agent_key, kind, coalesce(agent_variant,''), is_default, coalesce(position_key,''), updated_at FROM agents WHERE deleted_at='' ORDER BY is_default DESC, updated_at DESC`)
	if err != nil {
		panic(err)
	}
	i := 0
	for rows.Next() {
		var k, kind, v, pk, u string
		var d bool
		rows.Scan(&k, &kind, &v, &d, &pk, &u)
		i++
		fmt.Printf("%3d. %s | %s | %s | default=%v | pos=%s | %s\n", i, k, kind, v, d, pk, u)
	}
	rows.Close()
	fmt.Printf("total=%d\n", i)
}
