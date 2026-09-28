// One-off helper for local development: drop baseline business tables +
// schema_migrations so we can replay the baseline on a fresh DB.
//
// Run: go run ./scripts/reset_local_db
package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	db, err := sql.Open("mysql", "root:root123@tcp(127.0.0.1:3307)/ai_go_mall_local?parseTime=true&loc=Local&charset=utf8mb4")
	if err != nil {
		log.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("ping: %v", err)
	}
	for _, t := range []string{"config", "captchas", "tokens", "admins", "users", "schema_migrations"} {
		if _, err := db.Exec("DROP TABLE IF EXISTS `" + t + "`"); err != nil {
			log.Fatalf("drop %s: %v", t, err)
		}
		fmt.Printf("dropped %s\n", t)
	}
}
