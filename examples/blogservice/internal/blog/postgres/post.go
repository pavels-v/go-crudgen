package postgres

import (
	"context"
	"database/sql"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	domain "example.com/blogservice/internal/blog"
)

// postRow is the database representation of domain.Post.
type postRow struct {
	ID        uuid.UUID                `db:"id"`
	Title     string                   `db:"title"`
	Body      sql.Null[string]         `db:"body"`
	Published bool                     `db:"published"`
	Views     int64                    `db:"views"`
	Metadata  sql.Null[jsontext.Value] `db:"metadata"`
	Author    sql.Null[uuid.UUID]      `db:"author"`
	CreatedAt time.Time                `db:"created_at"`
	UpdatedAt time.Time                `db:"updated_at"`
}

// newPostRow builds the row written by Create and Update.
func newPostRow(m *domain.Post) postRow {
	return postRow{
		ID:        m.ID,
		Title:     m.Title,
		Body:      toNull(m.Body),
		Published: m.Published,
		Views:     m.Views,
		Metadata:  toNull(m.Metadata),
		Author:    toNull(m.Author),
	}
}

// toModel converts a scanned row back into the API model.
func (row postRow) toModel() domain.Post {
	return domain.Post{
		ID:        row.ID,
		Title:     row.Title,
		Body:      fromNull(row.Body),
		Published: row.Published,
		Views:     row.Views,
		Metadata:  fromNull(row.Metadata),
		Author:    fromNull(row.Author),
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

// PostRepository is a PostgreSQL-backed domain.PostRepository.
type PostRepository struct {
	db *sqlx.DB
}

// NewPostRepository returns a repository backed by db.
func NewPostRepository(db *sqlx.DB) *PostRepository {
	return &PostRepository{db: db}
}

var _ domain.PostRepository = (*PostRepository)(nil)

func (r *PostRepository) Create(ctx context.Context, m *domain.Post) error {
	m.ID = uuid.New()
	row := newPostRow(m)

	if err := r.db.QueryRowContext(ctx, `INSERT INTO posts (id, title, body, published, views, metadata, author, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, now(), now()) RETURNING created_at, updated_at`, row.ID, row.Title, row.Body, row.Published, row.Views, row.Metadata, row.Author).Scan(&m.CreatedAt, &m.UpdatedAt); err != nil {
		return fmt.Errorf("create post: %w", mapWriteError(err))
	}

	return nil
}

func (r *PostRepository) Get(ctx context.Context, id uuid.UUID) (*domain.Post, error) {
	var row postRow
	if err := r.db.GetContext(ctx, &row, `SELECT id, title, body, published, views, metadata, author, created_at, updated_at FROM posts WHERE id = $1`, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}

		return nil, fmt.Errorf("get post: %w", err)
	}

	m := row.toModel()

	return &m, nil
}

func (r *PostRepository) List(ctx context.Context, p domain.PostListParams) ([]domain.Post, error) {
	order, after := `created_at, id`, `(created_at, id) > (?, ?)`
	if p.Dir == domain.SortDesc {
		order, after = `created_at DESC, id DESC`, `(created_at, id) < (?, ?)`
	}

	var (
		where []string
		args  []any
	)

	if p.Published != nil {
		where = append(where, `published = ?`)
		args = append(args, *p.Published)
	}

	if p.Author != nil {
		where = append(where, `author = ?`)
		args = append(args, *p.Author)
	}

	if p.After != nil {
		where = append(where, after)
		args = append(args, p.After.CreatedAt, p.After.ID)
	}

	q := `SELECT id, title, body, published, views, metadata, author, created_at, updated_at FROM posts`
	if len(where) > 0 {
		q += ` WHERE ` + strings.Join(where, ` AND `)
	}

	args = append(args, p.Limit)
	q += ` ORDER BY ` + order + ` LIMIT ?`

	var rows []postRow
	if err := r.db.SelectContext(ctx, &rows, r.db.Rebind(q), args...); err != nil {
		return nil, fmt.Errorf("list post: %w", err)
	}

	out := make([]domain.Post, len(rows))
	for i := range rows {
		out[i] = rows[i].toModel()
	}

	return out, nil
}

func (r *PostRepository) Update(ctx context.Context, m *domain.Post) error {
	row := newPostRow(m)

	if err := r.db.QueryRowContext(ctx, `UPDATE posts SET title = $1, body = $2, published = $3, views = $4, metadata = $5, author = $6, updated_at = now() WHERE id = $7 RETURNING created_at, updated_at`, row.Title, row.Body, row.Published, row.Views, row.Metadata, row.Author, row.ID).Scan(&m.CreatedAt, &m.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ErrNotFound
		}

		return fmt.Errorf("update post: %w", mapWriteError(err))
	}

	return nil
}

func (r *PostRepository) Delete(ctx context.Context, id uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM posts WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete post: %w", mapDeleteError(err))
	}

	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete post: %w", err)
	}

	if n == 0 {
		return domain.ErrNotFound
	}

	return nil
}
