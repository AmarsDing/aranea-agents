package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	query := os.Args[1]
	if query == "" {
		fmt.Fprintln(os.Stderr, "usage: dbquery <sql>")
		os.Exit(1)
	}
	pool, err := pgxpool.New(context.Background(), "postgres://postgres:Hangshan%40123@127.0.0.1:5432/aranea?sslmode=disable")
	if err != nil {
		fmt.Fprintln(os.Stderr, "connect error:", err)
		os.Exit(1)
	}
	defer pool.Close()
	rows, err := pool.Query(context.Background(), query)
	if err != nil {
		fmt.Fprintln(os.Stderr, "query error:", err)
		os.Exit(1)
	}
	defer rows.Close()
	fields := rows.FieldDescriptions()
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			fmt.Fprintln(os.Stderr, "values error:", err)
			continue
		}
		for i, v := range vals {
			if i > 0 {
				fmt.Print(" | ")
			}
			if b, ok := v.([]byte); ok {
				fmt.Print(string(b))
			} else {
				fmt.Print(v)
			}
		}
		fmt.Println()
	}
	_ = fields
	// Also print as JSON for structured data
	rows2, err := pool.Query(context.Background(), query)
	if err != nil {
		return
	}
	defer rows2.Close()
	fmt.Println("\n=== JSON ===")
	for rows2.Next() {
		vals, err := rows2.Values()
		if err != nil {
			continue
		}
		j, _ := json.Marshal(vals)
		fmt.Println(string(j))
	}
}
