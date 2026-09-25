package blog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// tagRow is the database representation of Tag. Nullable
// columns use sql.Null[T] so a SQL NULL round-trips as an absent value, which
// newTagRow and toModel convert to and from the pointer fields on Tag.
type tagRow struct {
	Slug   string  `db:"slug"`
	Label  string  `db:"label"`
	Color  string  `db:"color"`
	Weight float64 `db:"weight"`
}

// newTagRow builds the row written by Create and Update. Option-managed
// columns (timestamps) are set by the SQL itself, so they are omitted here.
func newTagRow(m *Tag) tagRow {
	return tagRow{
		Slug:   m.Slug,
		Label:  m.Label,
		Color:  m.Color,
		Weight: m.Weight,
	}
}

// toModel converts a scanned row back into the API model.
func (row tagRow) toModel() Tag {
	return Tag{
		Slug:   row.Slug,
		Label:  row.Label,
		Color:  row.Color,
		Weight: row.Weight,
	}
}

// PostgresTagRepository is a PostgreSQL-backed TagRepository. It depends on sqlx rather
// than a concrete driver, so any database/sql-compatible Postgres driver
// (lib/pq, pgx's stdlib adapter, ...) can back it.
type PostgresTagRepository struct {
	db *sqlx.DB
}

// NewPostgresTagRepository returns a PostgresTagRepository backed by db.
func NewPostgresTagRepository(db *sqlx.DB) *PostgresTagRepository {
	return &PostgresTagRepository{db: db}
}

var _ TagRepository = (*PostgresTagRepository)(nil)

func (r *PostgresTagRepository) Create(ctx context.Context, m *Tag) error {
	row := newTagRow(m)
	if _, err := r.db.ExecContext(ctx, `INSERT INTO tags (slug, label, color, weight) VALUES ($1, $2, $3, $4)`, row.Slug, row.Label, row.Color, row.Weight); err != nil {
		return fmt.Errorf("create tag: %w", mapWriteError(err))
	}
	return nil
}

func (r *PostgresTagRepository) Get(ctx context.Context, id string) (*Tag, error) {
	var row tagRow
	if err := r.db.GetContext(ctx, &row, `SELECT slug, label, color, weight FROM tags WHERE slug = $1`, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get tag: %w", err)
	}
	m := row.toModel()
	return &m, nil
}

func (r *PostgresTagRepository) List(ctx context.Context, limit, offset int) ([]Tag, error) {
	rows := []tagRow{}
	if err := r.db.SelectContext(ctx, &rows, `SELECT slug, label, color, weight FROM tags ORDER BY slug LIMIT $1 OFFSET $2`, limit, offset); err != nil {
		return nil, fmt.Errorf("list tag: %w", err)
	}
	out := make([]Tag, len(rows))
	for i := range rows {
		out[i] = rows[i].toModel()
	}
	return out, nil
}

func (r *PostgresTagRepository) Update(ctx context.Context, m *Tag) error {
	row := newTagRow(m)
	res, err := r.db.ExecContext(ctx, `UPDATE tags SET label = $1, color = $2, weight = $3 WHERE slug = $4`, row.Label, row.Color, row.Weight, row.Slug)
	if err != nil {
		return fmt.Errorf("update tag: %w", mapWriteError(err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update tag: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresTagRepository) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM tags WHERE slug = $1`, id)
	if err != nil {
		return fmt.Errorf("delete tag: %w", mapDeleteError(err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete tag: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
