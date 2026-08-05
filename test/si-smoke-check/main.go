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

	var provider, model, baseURL, apiKey string
	err = db.QueryRow(`SELECT refine_llm_provider, refine_llm_model, refine_llm_base_url, refine_llm_api_key FROM system_settings LIMIT 1`).Scan(&provider, &model, &baseURL, &apiKey)
	if err != nil {
		fmt.Println("refine_llm query:", err)
	} else {
		keyState := "EMPTY"
		if len(apiKey) > 0 {
			keyState = fmt.Sprintf("set(len=%d)", len(apiKey))
		}
		fmt.Printf("refine_llm: provider=%q model=%q base_url=%q api_key=%s\n", provider, model, baseURL, keyState)
	}

	var cnt int
	var code string
	rows, err := db.Query(`SELECT error_code, COUNT(*) FROM model_token_usage_events WHERE status='failed' AND error_code<>'' AND occurred_at >= now() - interval '7 days' GROUP BY error_code ORDER BY 2 DESC LIMIT 5`)
	if err != nil {
		fmt.Println("cluster query:", err)
		return
	}
	defer rows.Close()
	fmt.Println("recent 7d failed error_code clusters:")
	found := false
	for rows.Next() {
		found = true
		rows.Scan(&code, &cnt)
		fmt.Printf("  %s x%d\n", code, cnt)
	}
	if !found {
		fmt.Println("  (none)")
	}
}
