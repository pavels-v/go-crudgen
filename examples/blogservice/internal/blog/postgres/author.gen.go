package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"example.com/blogservice/internal/blog"
)

// authorRow is the database representation of blog.Author. Nullable
// columns use sql.Null[T] so a SQL NULL round-trips as an absent value, which
// newAuthorRow and toModel convert to and from the pointer fields on blog.Author.
type authorRow struct {
	ID     uuid.UUID           `db:"id"`
	Email  string              `db:"email"`
	Name   sql.Null[string]    `db:"name"`
	BornOn sql.Null[blog.Date] `db:"born_on"`
}

// newAuthorRow builds the row written by Create and Update. Generated
// columns are set by the SQL itself, so they are omitted here.
func newAuthorRow(m *blog.Author) authorRow {
	return authorRow{
		ID:     m.ID,
		Email:  m.Email,
		Name:   toNull(m.Name),
		BornOn: toNull(m.BornOn),
	}
}

// toModel converts a scanned row back into the API model.
func (row authorRow) toModel() blog.Author {
	return blog.Author{
		ID:     row.ID,
		Email:  row.Email,
		Name:   fromNull(row.Name),
		BornOn: fromNull(row.BornOn),
	}
}

// AuthorRepository is a PostgreSQL-backed blog.AuthorRepository. It depends on sqlx rather
// than a concrete driver, so any database/sql-compatible Postgres driver
// (lib/pq, pgx's stdlib adapter, ...) can back it.
type AuthorRepository struct {
	db *sqlx.DB
}

// NewAuthorRepository returns a repository backed by db.
func NewAuthorRepository(db *sqlx.DB) *AuthorRepository {
	return &AuthorRepository{db: db}
}

var _ blog.AuthorRepository = (*AuthorRepository)(nil)

func (r *AuthorRepository) Create(ctx context.Context, m *blog.Author) error {
	m.ID = uuid.New()
	row := newAuthorRow(m)
	if _, err := r.db.ExecContext(ctx, `INSERT INTO authors (id, email, name, born_on) VALUES ($1, $2, $3, $4)`, row.ID, row.Email, row.Name, row.BornOn); err != nil {
		return fmt.Errorf("create author: %w", mapWriteError(err))
	}
	return nil
}

func (r *AuthorRepository) Get(ctx context.Context, id uuid.UUID) (*blog.Author, error) {
	var row authorRow
	if err := r.db.GetContext(ctx, &row, `SELECT id, email, name, born_on FROM authors WHERE id = $1`, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, blog.ErrNotFound
		}
		return nil, fmt.Errorf("get author: %w", err)
	}
	m := row.toModel()
	return &m, nil
}

func (r *AuthorRepository) List(ctx context.Context, p blog.AuthorListParams) ([]blog.Author, error) {
	order := `id`
	if p.Dir == blog.SortDesc {
		order = `id DESC`
	}

	var (
		where []string
		args  []any
	)
	if p.Email != nil {
		where = append(where, `email = ?`)
		args = append(args, *p.Email)
	}

	q := `SELECT id, email, name, born_on FROM authors`
	if len(where) > 0 {
		q += ` WHERE ` + strings.Join(where, ` AND `)
	}
	q += ` ORDER BY ` + order + ` LIMIT ? OFFSET ?`
	args = append(args, p.Limit, p.Offset)

	var rows []authorRow
	if err := r.db.SelectContext(ctx, &rows, r.db.Rebind(q), args...); err != nil {
		return nil, fmt.Errorf("list author: %w", err)
	}
	out := make([]blog.Author, len(rows))
	for i := range rows {
		out[i] = rows[i].toModel()
	}
	return out, nil
}

func (r *AuthorRepository) Update(ctx context.Context, m *blog.Author) error {
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
		return blog.ErrNotFound
	}
	return nil
}

func (r *AuthorRepository) Delete(ctx context.Context, id uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM authors WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete author: %w", mapDeleteError(err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete author: %w", err)
	}
	if n == 0 {
		return blog.ErrNotFound
	}
	return nil
}
