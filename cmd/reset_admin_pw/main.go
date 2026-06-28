package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	dsn := "postgres://postgres:Hangshan%40123@127.0.0.1:5432/aranea?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Reset name, email, password for id=1
	hashed, err := bcrypt.GenerateFromPassword([]byte("changeme"), bcrypt.DefaultCost)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bcrypt: %v\n", err)
		os.Exit(1)
	}

	res, err := db.Exec(`UPDATE admins SET name = 'admin', email = 'admin@aranea.local', password = $1 WHERE id = 1`, string(hashed))
	if err != nil {
		fmt.Fprintf(os.Stderr, "update: %v\n", err)
		os.Exit(1)
	}
	rowsAffected, _ := res.RowsAffected()
	fmt.Printf("updated rows: %d\n", rowsAffected)

	// Verify
	var name, email string
	err = db.QueryRow("SELECT name, email FROM admins WHERE id = 1").Scan(&name, &email)
	if err != nil {
		fmt.Fprintf(os.Stderr, "verify: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("verified: name=%q email=%q\n", name, email)
}
