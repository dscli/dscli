// Package sqlite - provide sqlite integration
package sqlite

import (
	"database/sql"

	"github.com/dscli/dscli/internal/context"
	"github.com/nanjj/clog"
	_ "modernc.org/sqlite"
)

func Open(ctx context.Context, dbPath string) (*sql.DB, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "Open")
	defer span.Finish()
	return sql.Open("sqlite", dbPath+"?_journal=WAL&_timeout=5000&_fk=1&_txlock=immediate")
}
