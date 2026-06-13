package blog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"time"
)

const (
	createPostSQL = `INSERT INTO posts (id, title, body, published, author, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, now(), now()) RETURNING created_at, updated_at`
	getPostSQL    = `SELECT id, title, body, published, author, created_at, updated_at FROM posts WHERE id = $1`
	listPostSQL   = `SELECT id, title, body, published, author, created_at, updated_at FROM posts ORDER BY id LIMIT $1 OFFSET $2`
	updatePostSQL = `UPDATE posts SET title = $1, body = $2, published = $3, author = $4, updated_at = now() WHERE id = $5 RETURNING updated_at`
	deletePostSQL = `DELETE FROM posts WHERE id = $1`
)

// postRow is the database representation of Post. Nullable
// columns use sql.Null[T] so a SQL NULL round-trips as an absent value, which
// newPostRow and toModel convert to and from the pointer fields on Post.
type postRow struct {
	ID        uuid.UUID           `db:"id"`
	Title     string              `db:"title"`
	Body      sql.Null[string]    `db:"body"`
	Published sql.Null[bool]      `db:"published"`
	Author    sql.Null[uuid.UUID] `db:"author"`
	CreatedAt time.Time           `db:"created_at"`
	UpdatedAt time.Time           `db:"updated_at"`
}

// newPostRow builds the row written by Create and Update. Option-managed
// columns (timestamps) are set by the SQL itself, so they are omitted here.
func newPostRow(m *Post) postRow {
	return postRow{
		ID:        m.ID,
		Title:     m.Title,
		Body:      toNull(m.Body),
		Published: toNull(m.Published),
		Author:    toNull(m.Author),
	}
}

// toModel converts a scanned row back into the API model.
func (row postRow) toModel() Post {
	return Post{
		ID:        row.ID,
		Title:     row.Title,
		Body:      fromNull(row.Body),
		Published: fromNull(row.Published),
		Author:    fromNull(row.Author),
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

// PostgresPostRepository is a PostgreSQL-backed PostRepository. It depends on sqlx rather
// than a concrete driver, so any database/sql-compatible Postgres driver
// (lib/pq, pgx's stdlib adapter, ...) can back it.
type PostgresPostRepository struct {
	db *sqlx.DB
}

// NewPostgresPostRepository returns a PostgresPostRepository backed by db.
func NewPostgresPostRepository(db *sqlx.DB) *PostgresPostRepository {
	return &PostgresPostRepository{db: db}
}

var _ PostRepository = (*PostgresPostRepository)(nil)

func (r *PostgresPostRepository) Create(ctx context.Context, m *Post) error {
	row := newPostRow(m)
	if err := r.db.QueryRowContext(ctx, createPostSQL, row.ID, row.Title, row.Body, row.Published, row.Author).Scan(&m.CreatedAt, &m.UpdatedAt); err != nil {
		return fmt.Errorf("create post: %w", err)
	}
	return nil
}

func (r *PostgresPostRepository) Get(ctx context.Context, id uuid.UUID) (*Post, error) {
	var row postRow
	err := r.db.GetContext(ctx, &row, getPostSQL, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get post: %w", err)
	}
	m := row.toModel()
	return &m, nil
}

func (r *PostgresPostRepository) List(ctx context.Context, limit, offset int) ([]Post, error) {
	rows := []postRow{}
	if err := r.db.SelectContext(ctx, &rows, listPostSQL, limit, offset); err != nil {
		return nil, fmt.Errorf("list post: %w", err)
	}
	out := make([]Post, len(rows))
	for i := range rows {
		out[i] = rows[i].toModel()
	}
	return out, nil
}

func (r *PostgresPostRepository) Update(ctx context.Context, m *Post) error {
	row := newPostRow(m)
	err := r.db.QueryRowContext(ctx, updatePostSQL, row.Title, row.Body, row.Published, row.Author, row.ID).Scan(&m.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("update post: %w", err)
	}
	return nil
}

func (r *PostgresPostRepository) Delete(ctx context.Context, id uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, deletePostSQL, id)
	if err != nil {
		return fmt.Errorf("delete post: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete post: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
