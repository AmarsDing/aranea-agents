package main
import (
  "database/sql"
  "fmt"
  "strings"
  _ "modernc.org/sqlite"
)
func main() {
  db, err := sql.Open("sqlite", `file:f:/aranea-agents/cmd/data/arenea.sqlite?mode=ro`)
  if err != nil { panic(err) }
  defer db.Close()
  rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' ORDER BY name`)
  if err != nil { panic(err) }
  var tables []string
  for rows.Next() { var n string; rows.Scan(&n); tables = append(tables, n) }
  fmt.Println("TABLES:", strings.Join(tables, ", "))
}
