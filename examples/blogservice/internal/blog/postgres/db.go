package postgres

import (
	"errors"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"

	"example.com/blogservice/internal/blog/domain"
)

// Connection pool limits applied by NewDB.
const (
	driverName = "pgx"

	maxOpenConns    = 25
	maxIdleConns    = 25
	connMaxLifetime = 5 * time.Minute
)

// NewDB opens a lazy connection pool through the configured driver.
func NewDB(dsn string) (*sqlx.DB, error) {
	db, err := sqlx.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
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
		return fmt.Errorf("%w: %v", domain.ErrAlreadyExists, err)
	case sqlStateForeignKeyViolation:
		return fmt.Errorf("%w: %v", domain.ErrReferenceNotFound, err)
	}

	return err
}

func mapDeleteError(err error) error {
	if sqlState(err) == sqlStateForeignKeyViolation {
		return fmt.Errorf("%w: %v", domain.ErrStillReferenced, err)
	}

	return err
}
