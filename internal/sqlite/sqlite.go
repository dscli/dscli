// Package sqlite - provide sqlite integration
package sqlite

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

func Open(dbPath string) (*sql.DB, error) {
	return sql.Open("sqlite", dbPath+"?_journal=WAL&_timeout=5000&_fk=1&_txlock=immediate")
}
