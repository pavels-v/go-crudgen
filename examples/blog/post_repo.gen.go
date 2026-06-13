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
	createPostSQL = `INSERT INTO posts (id, title, body, published, author, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, now(), now()) RETURNING created_at, updated_at`
	getPostSQL    = `SELECT id, title, body, published, author, created_at, updated_at FROM posts WHERE id = $1`
	listPostSQL   = `SELECT id, title, body, published, author, created_at, updated_at FROM posts ORDER BY id LIMIT $1 OFFSET $2`
	updatePostSQL = `UPDATE posts SET title = $1, body = $2, published = $3, author = $4, updated_at = now() WHERE id = $5 RETURNING updated_at`
	deletePostSQL = `DELETE FROM posts WHERE id = $1`
)

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
	if err := r.db.QueryRowContext(ctx, createPostSQL, m.ID, m.Title, m.Body, m.Published, m.Author).Scan(&m.CreatedAt, &m.UpdatedAt); err != nil {
		return fmt.Errorf("create post: %w", err)
	}
	return nil
}

func (r *PostgresPostRepository) Get(ctx context.Context, id uuid.UUID) (*Post, error) {
	var m Post
	err := r.db.GetContext(ctx, &m, getPostSQL, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get post: %w", err)
	}
	return &m, nil
}

func (r *PostgresPostRepository) List(ctx context.Context, limit, offset int) ([]Post, error) {
	out := []Post{}
	if err := r.db.SelectContext(ctx, &out, listPostSQL, limit, offset); err != nil {
		return nil, fmt.Errorf("list post: %w", err)
	}
	return out, nil
}

func (r *PostgresPostRepository) Update(ctx context.Context, m *Post) error {
	err := r.db.QueryRowContext(ctx, updatePostSQL, m.Title, m.Body, m.Published, m.Author, m.ID).Scan(&m.UpdatedAt)
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
