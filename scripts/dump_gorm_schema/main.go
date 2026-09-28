// One-off introspection: dumps CREATE TABLE-equivalent DDL for every table
// in ai_go_mall_local on 127.0.0.1:3307, by querying information_schema.
// This tells us exactly what GORM AutoMigrate produced so we can align the
// golang-migrate baseline DDL byte-for-byte.
//
// Run: go run ./scripts/dump_gorm_schema
package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	dsn := "root:root123@tcp(127.0.0.1:3307)/ai_go_mall_local?parseTime=true&loc=Local&charset=utf8mb4"
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		log.Fatalf("ping: %v", err)
	}

	tables, err := listTables(db)
	if err != nil {
		log.Fatalf("list: %v", err)
	}
	for _, t := range tables {
		fmt.Printf("===== TABLE %s =====\n", t)
		cols, err := describeTable(db, t)
		if err != nil {
			log.Fatalf("describe %s: %v", t, err)
		}
		for _, c := range cols {
			fmt.Printf("  %s | %s | %s | default=%q | nullable=%v | key=%s | extra=%s | comment=%s\n",
				c.name, c.colType, c.collation, c.defVal, c.nullable, c.key, c.extra, c.comment)
		}
		fmt.Println()
		idx, err := indexes(db, t)
		if err != nil {
			log.Fatalf("indexes %s: %v", t, err)
		}
		for _, i := range idx {
			fmt.Printf("  index: %s unique=%v cols=%v\n", i.name, i.unique, i.cols)
		}
		tt, err := tableOpts(db, t)
		if err != nil {
			log.Fatalf("opts %s: %v", t, err)
		}
		fmt.Printf("  opts: engine=%s collation=%s comment=%q\n", tt.engine, tt.collation, tt.comment)
		fmt.Println()
	}
	_ = os.Stdout.Sync()
}

// silence "imported and not used" if fmt stripped, defend against future edits.
var _ = fmt.Sprintf

type col struct {
	name, colType, collation, defVal, key, extra, comment sql.NullString
	nullable                                              bool
}
type idx struct {
	name    string
	unique  bool
	cols    []string
}
type tbl struct {
	engine, collation, comment string
}

func listTables(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT TABLE_NAME FROM information_schema.tables WHERE TABLE_SCHEMA = DATABASE() ORDER BY TABLE_NAME`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

func describeTable(db *sql.DB, t string) ([]col, error) {
	rows, err := db.Query(`SELECT COLUMN_NAME, COLUMN_TYPE, COLLATION_NAME, COLUMN_DEFAULT, IS_NULLABLE, COLUMN_KEY, EXTRA, COLUMN_COMMENT FROM information_schema.columns WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? ORDER BY ORDINAL_POSITION`, t)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []col
	for rows.Next() {
		var c col
		var nullable string
		if err := rows.Scan(&c.name, &c.colType, &c.collation, &c.defVal, &nullable, &c.key, &c.extra, &c.comment); err != nil {
			return nil, err
		}
		c.nullable = nullable == "YES"
		out = append(out, c)
	}
	return out, rows.Err()
}

func indexes(db *sql.DB, t string) ([]idx, error) {
	rows, err := db.Query(`SELECT INDEX_NAME, NON_UNIQUE, COLUMN_NAME FROM information_schema.statistics WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? ORDER BY INDEX_NAME, SEQ_IN_INDEX`, t)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byName := map[string]*idx{}
	order := []string{}
	for rows.Next() {
		var name, col string
		var nonUnique int
		if err := rows.Scan(&name, &nonUnique, &col); err != nil {
			return nil, err
		}
		k, ok := byName[name]
		if !ok {
			k = &idx{name: name, unique: nonUnique == 0}
			byName[name] = k
			order = append(order, name)
		}
		k.cols = append(k.cols, col)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]idx, 0, len(order))
	for _, n := range order {
		out = append(out, *byName[n])
	}
	return out, nil
}

func tableOpts(db *sql.DB, t string) (tbl, error) {
	var tt tbl
	var engine, collation, comment sql.NullString
	err := db.QueryRow(`SELECT ENGINE, TABLE_COLLATION, TABLE_COMMENT FROM information_schema.tables WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?`, t).Scan(&engine, &collation, &comment)
	if err != nil {
		return tt, err
	}
	if engine.Valid {
		tt.engine = engine.String
	}
	if collation.Valid {
		tt.collation = collation.String
	}
	if comment.Valid {
		tt.comment = comment.String
	}
	return tt, nil
}

var _ = os.Stdout
