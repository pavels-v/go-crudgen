package blog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// authorRow is the database representation of Author. Nullable
// columns use sql.Null[T] so a SQL NULL round-trips as an absent value, which
// newAuthorRow and toModel convert to and from the pointer fields on Author.
type authorRow struct {
	ID     uuid.UUID           `db:"id"`
	Email  string              `db:"email"`
	Name   sql.Null[string]    `db:"name"`
	BornOn sql.Null[time.Time] `db:"born_on"`
}

// newAuthorRow builds the row written by Create and Update. Option-managed
// columns (timestamps) are set by the SQL itself, so they are omitted here.
func newAuthorRow(m *Author) authorRow {
	return authorRow{
		ID:     m.ID,
		Email:  m.Email,
		Name:   toNull(m.Name),
		BornOn: toNull(m.BornOn),
	}
}

// toModel converts a scanned row back into the API model.
func (row authorRow) toModel() Author {
	return Author{
		ID:     row.ID,
		Email:  row.Email,
		Name:   fromNull(row.Name),
		BornOn: fromNull(row.BornOn),
	}
}

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
	m.ID = uuid.New()
	row := newAuthorRow(m)
	if _, err := r.db.ExecContext(ctx, `INSERT INTO authors (id, email, name, born_on) VALUES ($1, $2, $3, $4)`, row.ID, row.Email, row.Name, row.BornOn); err != nil {
		return fmt.Errorf("create author: %w", mapWriteError(err))
	}
	return nil
}

func (r *PostgresAuthorRepository) Get(ctx context.Context, id uuid.UUID) (*Author, error) {
	var row authorRow
	if err := r.db.GetContext(ctx, &row, `SELECT id, email, name, born_on FROM authors WHERE id = $1`, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get author: %w", err)
	}
	m := row.toModel()
	return &m, nil
}

func (r *PostgresAuthorRepository) List(ctx context.Context, limit, offset int) ([]Author, error) {
	rows := []authorRow{}
	if err := r.db.SelectContext(ctx, &rows, `SELECT id, email, name, born_on FROM authors ORDER BY id LIMIT $1 OFFSET $2`, limit, offset); err != nil {
		return nil, fmt.Errorf("list author: %w", err)
	}
	out := make([]Author, len(rows))
	for i := range rows {
		out[i] = rows[i].toModel()
	}
	return out, nil
}

func (r *PostgresAuthorRepository) Update(ctx context.Context, m *Author) error {
	row := newAuthorRow(m)
	res, err := r.db.ExecContext(ctx, `UPDATE authors SET email = $1, name = $2, born_on = $3 WHERE id = $4`, row.Email, row.Name, row.BornOn, row.ID)
	if err != nil {
		return fmt.Errorf("update author: %w", mapWriteError(err))
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
	res, err := r.db.ExecContext(ctx, `DELETE FROM authors WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete author: %w", mapDeleteError(err))
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
