package blog

import (
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

// driverName is the database/sql driver the generated connection uses. It is
// selected at generation time via the --driver flag; the repositories
// themselves stay driver-agnostic (they only need a *sqlx.DB).
const driverName = "pgx"

// NewDB opens a connection pool through the configured driver. Like sql.Open it
// is lazy: it validates the arguments but defers the first real connection to
// first use, so callers that need a startup health check should Ping the
// returned pool themselves. Pass it to the New<Entity>Repository constructors.
func NewDB(dsn string) (*sqlx.DB, error) {
	return sqlx.Open(driverName, dsn)
}
