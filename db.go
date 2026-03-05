package main

import (
	"database/sql"
	"fmt"
	"path/filepath"

	_ "modernc.org/sqlite"
)

var (
	HistoryLimit   = &struct{}{}
	ModelID        = int64(0)
	DBPath         = filepath.Join(ConfigDir, "sqlite.db")
	tableSchemas   = []string{}
	indexSchemas   = []string{}
	upgradeSchemas = []string{}
)

func RegisterTableSchema(ss ...string) {
	tableSchemas = append(tableSchemas, ss...)
}

func RegisterIndexSchema(ss ...string) {
	indexSchemas = append(indexSchemas, ss...)
}

func RegisterUpgradeSchema(ss ...string) {
	upgradeSchemas = append(upgradeSchemas, ss...)
}

func init() {
	db, err := OpenDB()
	if err != nil {
		panic(err)
	}
	defer db.Close()

	// tables
	for _, query := range tableSchemas {
		if _, err := db.Exec(query); err != nil {
			panic(err)
		}
	}

	// index
	for _, query := range indexSchemas {
		if _, err := db.Exec(query); err != nil {
			panic(err)
		}
	}

	// upgrade scripts(ignore error)
	for _, query := range upgradeSchemas {
		if _, err := db.Exec(query); err == nil {
			fmt.Printf("migrate %s done", query)
		}
	}
}

func OpenDB(elem ...string) (db *sql.DB, err error) {
	dbPath := DBPath
	if len(elem) != 0 {
		dbPath = filepath.Join(elem...)
	}
	return sql.Open("sqlite", dbPath)
}
