package blog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

const (
	createAuthorSQL = `INSERT INTO authors (id, email, name) VALUES ($1, $2, $3)`
	getAuthorSQL    = `SELECT id, email, name FROM authors WHERE id = $1`
	listAuthorSQL   = `SELECT id, email, name FROM authors ORDER BY id LIMIT $1 OFFSET $2`
	updateAuthorSQL = `UPDATE authors SET email = $1, name = $2 WHERE id = $3`
	deleteAuthorSQL = `DELETE FROM authors WHERE id = $1`
)

// PostgresAuthorRepository is a PostgreSQL-backed AuthorRepository. It depends on sqlx rather
// than a concrete driver, so any database/sql-compatible Postgres driver
// (lib/pq, pgx's stdlib adapter, ...) can back it.
type PostgresAuthorRepository struct {
	db *sqlx.DB
}

// NewPostgresAuthorRepository returns a PostgresAuthorRepository backed by db.
func NewPostgresAuthorRepository(db *sqlx.DB) *PostgresAuthorRepository {
	return &PostgresAuthorRepository{db: db}
}

var _ AuthorRepository = (*PostgresAuthorRepository)(nil)

func (r *PostgresAuthorRepository) Create(ctx context.Context, m *Author) error {
	if _, err := r.db.ExecContext(ctx, createAuthorSQL, m.ID, m.Email, m.Name); err != nil {
		return fmt.Errorf("create author: %w", err)
	}
	return nil
}

func (r *PostgresAuthorRepository) Get(ctx context.Context, id uuid.UUID) (*Author, error) {
	var m Author
	err := r.db.GetContext(ctx, &m, getAuthorSQL, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get author: %w", err)
	}
	return &m, nil
}

func (r *PostgresAuthorRepository) List(ctx context.Context, limit, offset int) ([]Author, error) {
	out := []Author{}
	if err := r.db.SelectContext(ctx, &out, listAuthorSQL, limit, offset); err != nil {
		return nil, fmt.Errorf("list author: %w", err)
	}
	return out, nil
}

func (r *PostgresAuthorRepository) Update(ctx context.Context, m *Author) error {
	res, err := r.db.ExecContext(ctx, updateAuthorSQL, m.Email, m.Name, m.ID)
	if err != nil {
		return fmt.Errorf("update author: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update author: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresAuthorRepository) Delete(ctx context.Context, id uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, deleteAuthorSQL, id)
	if err != nil {
		return fmt.Errorf("delete author: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete author: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
