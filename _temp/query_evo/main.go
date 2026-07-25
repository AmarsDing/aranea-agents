package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq"
)

func main() {
	db, err := sql.Open("postgres", "postgres://postgres:Hangshan%40123@127.0.0.1:5432/aranea?sslmode=disable")
	if err != nil {
		fmt.Println("open:", err)
		os.Exit(1)
	}
	defer db.Close()

	fmt.Println("== learning_patterns (spirit) ==")
	rows, err := db.Query(`SELECT id, kind, status, frequency, confidence, substr(description,1,100) FROM learning_patterns WHERE agent_id='agent___spirit__' ORDER BY updated_at DESC LIMIT 5`)
	if err != nil {
		fmt.Println("patterns query:", err)
	} else {
		for rows.Next() {
			var id, kind, status, desc string
			var freq int
			var conf float64
			_ = rows.Scan(&id, &kind, &status, &freq, &conf, &desc)
			fmt.Printf("  id=%s kind=%s status=%s freq=%d conf=%.2f desc=%s\n", id, kind, status, freq, conf, desc)
		}
		rows.Close()
	}

	fmt.Println("== skill_proposals (spirit) ==")
	rows2, err := db.Query(`SELECT id, status, coalesce(skill_name,''), substr(coalesce(pattern_hash,''),1,12), created_at FROM skill_proposals WHERE agent_id='agent___spirit__' ORDER BY created_at DESC LIMIT 5`)
	if err != nil {
		fmt.Println("proposals query:", err)
	} else {
		for rows2.Next() {
			var id, status, name, hash, ts string
			_ = rows2.Scan(&id, &status, &name, &hash, &ts)
			fmt.Printf("  id=%s status=%s name=%s hash=%s at=%s\n", id, status, name, hash, ts)
		}
		rows2.Close()
	}

	fmt.Println("== learning_observations (spirit, recent) ==")
	rows3, err := db.Query(`SELECT kind, count(*) FROM learning_observations WHERE agent_id='agent___spirit__' GROUP BY kind`)
	if err != nil {
		fmt.Println("obs query:", err)
	} else {
		for rows3.Next() {
			var kind string
			var n int
			_ = rows3.Scan(&kind, &n)
			fmt.Printf("  kind=%s count=%d\n", kind, n)
		}
		rows3.Close()
	}
}
