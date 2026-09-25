package postgres

import (
	"errors"
	"fmt"
	"time"

	"example.com/blogservice/internal/blog"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

// Connection-pool defaults. Tune these for your workload, or replace NewDB with
// your own constructor if you need them configurable.
const (
	driverName = "pgx"

	maxOpenConns    = 25
	maxIdleConns    = 25
	connMaxLifetime = 5 * time.Minute
)

// NewDB opens a connection pool through the configured driver and applies sane
// pool limits. Like sql.Open it is lazy: it validates the arguments but defers
// the first real connection to first use, so callers that need a startup health
// check should Ping the returned pool themselves. Pass it to the
// New<Entity>Repository constructors.
func NewDB(dsn string) (*sqlx.DB, error) {
	db, err := sqlx.Open(driverName, dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(connMaxLifetime)
	return db, nil
}

const (
	sqlStateUniqueViolation     = "23505"
	sqlStateForeignKeyViolation = "23503"
)

type sqlStater interface {
	SQLState() string
}

func sqlState(err error) string {
	var se sqlStater
	if errors.As(err, &se) {
		return se.SQLState()
	}
	return ""
}

func mapWriteError(err error) error {
	switch sqlState(err) {
	case sqlStateUniqueViolation:
		return fmt.Errorf("%w: %v", blog.ErrAlreadyExists, err)
	case sqlStateForeignKeyViolation:
		return fmt.Errorf("%w: %v", blog.ErrReferenceNotFound, err)
	}
	return err
}

func mapDeleteError(err error) error {
	if sqlState(err) == sqlStateForeignKeyViolation {
		return fmt.Errorf("%w: %v", blog.ErrStillReferenced, err)
	}
	return err
}
