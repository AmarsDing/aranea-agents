package main

import (
	"context"
	"database/sql"
	"fmt"

	"aranea-agents/internal/conf"
	"aranea-agents/internal/data/ent"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/glebarez/go-sqlite/compat"

	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/file"
)

func main() {
	c := config.New(config.WithSource(file.NewSource("configs")))
	defer c.Close()
	if err := c.Load(); err != nil {
		panic(err)
	}
	var bc conf.Bootstrap
	if err := c.Scan(&bc); err != nil {
		panic(err)
	}
	ia := bc.Data.GetInitialAdmin()
	fmt.Printf("InitialAdmin: %+v\n", ia)
	if ia != nil {
		fmt.Printf("  Name: %q, Email: %q, Password: %q, Access: %q\n",
			ia.GetName(), ia.GetEmail(), ia.GetPassword(), ia.GetAccess())
	}

	rawDB, err := sql.Open(dialect.SQLite, "file:./data/arenea.sqlite?cache=shared&_fk=1")
	if err != nil {
		fmt.Println("DB open error:", err)
		return
	}
	defer rawDB.Close()
	rawDB.SetMaxOpenConns(1)

	entClient := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, rawDB)))
	admins, err := entClient.Admin.Query().All(context.Background())
	if err != nil {
		fmt.Println("Query error:", err)
		return
	}
	fmt.Printf("\nAdmins in DB (%d):\n", len(admins))
	for _, a := range admins {
		fmt.Printf("  ID=%d Name=%q Email=%q Access=%q Password=%q\n",
			a.ID, a.Name, a.Email, a.Access, a.Password)
	}
}
