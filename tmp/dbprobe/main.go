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

	fmt.Println("== 4 core butlers ==")
	rows, err := db.Query(`SELECT agent_key, display_name, is_default FROM agents WHERE kind='system_builtin' AND (agent_variant='' OR agent_variant IS NULL) AND deleted_at=''`)
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		var k, n string
		var d bool
		rows.Scan(&k, &n, &d)
		fmt.Printf("%s | %s | is_default=%v\n", k, n, d)
	}
	rows.Close()

	fmt.Println("\n== copy agents and candidate sources ==")
	rows, err = db.Query(`SELECT agent_key, display_name, position_id, position_key FROM agents WHERE agent_key LIKE '%-copy-%' AND deleted_at=''`)
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		var k, n, pid, pk string
		rows.Scan(&k, &n, &pid, &pk)
		fmt.Printf("COPY %s | %s | pid=%s pk=%s\n", k, n, pid, pk)
	}
	rows.Close()

	fmt.Println("\n== candidate source agents ==")
	rows, err = db.Query(`SELECT agent_key, position_id, position_key FROM agents WHERE agent_key IN ('ppc_strategist__general','pr_communications_manager__general','private_domain_operator__general','programmatic_buyer__general') AND deleted_at=''`)
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		var k, pid, pk string
		rows.Scan(&k, &pid, &pk)
		fmt.Printf("SRC %s | pid=%s pk=%s\n", k, pid, pk)
	}
	rows.Close()

	fmt.Println("\n== organizations enabled sample ==")
	rows, err = db.Query(`SELECT level, org_key, name, enabled FROM organizations WHERE level='company' LIMIT 5`)
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		var l, k, n string
		var e bool
		rows.Scan(&l, &k, &n, &e)
		fmt.Printf("%s | %s | %s | enabled=%v\n", l, k, n, e)
	}
	rows.Close()

	fmt.Println("\n== dept_lead agents position_id check ==")
	rows, err = db.Query(`SELECT count(*), coalesce(position_id,'<empty>') FROM agents WHERE agent_variant='dept_lead' AND deleted_at='' GROUP BY 2`)
	if err != nil {
		panic(err)
	}
	for rows.Next() {
		var c int
		var pid string
		rows.Scan(&c, &pid)
		fmt.Printf("count=%d pid=%s\n", c, pid)
	}
	rows.Close()
}
