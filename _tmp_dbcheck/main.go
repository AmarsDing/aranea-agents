package main

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

func main() {
	db, err := sql.Open("postgres", "postgres://postgres:Hangshan%40123@127.0.0.1:5432/aranea?sslmode=disable")
	if err != nil { panic(err) }
	defer db.Close()

	rows, err := db.Query(`SELECT id, agent_id, observed_at FROM learning_observations LIMIT 20`)
	if err != nil { panic(err) }
	fmt.Println("== learning_observations ==")
	for rows.Next() { var id, aid, at string; rows.Scan(&id, &aid, &at); fmt.Printf("id=%s agent=%s observed_at=%q\n", id, aid, at) }
	rows.Close()

	rows2, err := db.Query(`SELECT id, agent_id, detected_at FROM learning_patterns LIMIT 10`)
	if err != nil { fmt.Println("patterns err:", err) } else {
		fmt.Println("== learning_patterns ==")
		for rows2.Next() { var id, aid, at string; rows2.Scan(&id, &aid, &at); fmt.Printf("id=%s agent=%s detected_at=%q\n", id, aid, at) }
		rows2.Close()
	}

	rows3, err := db.Query(`SELECT id, agent_id, validated_at, created_at, updated_at FROM learning_proposals LIMIT 10`)
	if err != nil { fmt.Println("proposals err:", err) } else {
		fmt.Println("== learning_proposals ==")
		for rows3.Next() { var id, aid, ca, ua string; var va sql.NullString; rows3.Scan(&id, &aid, &va, &ca, &ua); fmt.Printf("id=%s agent=%s validated_at=%v created_at=%q updated_at=%q\n", id, aid, va, ca, ua) }
		rows3.Close()
	}

	// usage_quotas content
	rows4, err := db.Query(`SELECT scope_type, scope_id, monthly_micro_usd FROM usage_quotas LIMIT 10`)
	if err != nil { fmt.Println("quotas err:", err) } else {
		fmt.Println("== usage_quotas ==")
		for rows4.Next() { var st, sid string; var m int64; rows4.Scan(&st, &sid, &m); fmt.Printf("scope=%s/%s monthly=%d\n", st, sid, m) }
		rows4.Close()
	}
}
