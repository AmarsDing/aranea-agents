// 查询 missing4 批次（最近15分钟）的 import jobs 决策详情
//go:build ignore

package main

import (
	"database/sql"
	"encoding/json"
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

	rows, err := db.Query(`SELECT id, status, validation_status, message, candidates_json::text, conflict_groups_json::text, created_at FROM skill_import_jobs WHERE created_at::timestamptz > NOW() - INTERVAL '20 minutes' ORDER BY created_at ASC`)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, st, vs, msg, cand, cg, ca string
		rows.Scan(&id, &st, &vs, &msg, &cand, &cg, &ca)
		fmt.Printf("\n=== job %s status=%s validation=%s created=%s ===\n  msg=%s\n", id, st, vs, ca, msg)
		// 提取 candidate slug + validation_status
		var c struct {
			Items []struct {
				Slug             string `json:"slug"`
				ValidationStatus string `json:"validation_status"`
			} `json:"items"`
		}
		if err := json.Unmarshal([]byte(cand), &c); err == nil {
			for _, it := range c.Items {
				fmt.Printf("  candidate: slug=%s validation=%s\n", it.Slug, it.ValidationStatus)
			}
		}
		// 提取 conflict group recommendation
		var g struct {
			Items []struct {
				GroupID string `json:"group_id"`
				Metrics struct {
					Recommendation string `json:"recommendation"`
					ConflictRisk   string `json:"conflict_risk"`
				} `json:"metrics"`
			} `json:"items"`
		}
		if err := json.Unmarshal([]byte(cg), &g); err == nil {
			for _, it := range g.Items {
				fmt.Printf("  conflict: group=%s recommendation=%s risk=%s\n", it.GroupID, it.Metrics.Recommendation, it.Metrics.ConflictRisk)
			}
		}
	}
}
