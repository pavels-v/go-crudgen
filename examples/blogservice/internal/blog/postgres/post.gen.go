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

	"example.com/blogservice/internal/blog"
)

// postRow is the database representation of blog.Post. Nullable
// columns use sql.Null[T] so a SQL NULL round-trips as an absent value, which
// newPostRow and toModel convert to and from the pointer fields on blog.Post.
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

// newPostRow builds the row written by Create and Update. Generated
// columns are set by the SQL itself, so they are omitted here.
func newPostRow(m *blog.Post) postRow {
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
func (row postRow) toModel() blog.Post {
	return blog.Post{
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

// PostRepository is a PostgreSQL-backed blog.PostRepository. It depends on sqlx rather
// than a concrete driver, so any database/sql-compatible Postgres driver
// (lib/pq, pgx's stdlib adapter, ...) can back it.
type PostRepository struct {
	db *sqlx.DB
}

// NewPostRepository returns a repository backed by db.
func NewPostRepository(db *sqlx.DB) *PostRepository {
	return &PostRepository{db: db}
}

var _ blog.PostRepository = (*PostRepository)(nil)

func (r *PostRepository) Create(ctx context.Context, m *blog.Post) error {
	m.ID = uuid.New()
	row := newPostRow(m)
	if err := r.db.QueryRowContext(ctx, `INSERT INTO posts (id, title, body, published, views, metadata, author, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, now(), now()) RETURNING created_at, updated_at`, row.ID, row.Title, row.Body, row.Published, row.Views, row.Metadata, row.Author).Scan(&m.CreatedAt, &m.UpdatedAt); err != nil {
		return fmt.Errorf("create post: %w", mapWriteError(err))
	}
	return nil
}

func (r *PostRepository) Get(ctx context.Context, id uuid.UUID) (*blog.Post, error) {
	var row postRow
	if err := r.db.GetContext(ctx, &row, `SELECT id, title, body, published, views, metadata, author, created_at, updated_at FROM posts WHERE id = $1`, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, blog.ErrNotFound
		}
		return nil, fmt.Errorf("get post: %w", err)
	}
	m := row.toModel()
	return &m, nil
}

func (r *PostRepository) List(ctx context.Context, p blog.PostListParams) ([]blog.Post, error) {
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

	desc := p.Dir == blog.SortDesc
	var order string
	switch {
	case p.Sort == blog.PostSortTitle && desc:
		order = `title DESC, id DESC`
		if p.After != nil {
			where = append(where, `(title, id) < (?, ?)`)
			args = append(args, p.After.Title, p.After.ID)
		}
	case p.Sort == blog.PostSortTitle:
		order = `title, id`
		if p.After != nil {
			where = append(where, `(title, id) > (?, ?)`)
			args = append(args, p.After.Title, p.After.ID)
		}
	case p.Sort == blog.PostSortViews && desc:
		order = `views DESC, id DESC`
		if p.After != nil {
			where = append(where, `(views, id) < (?, ?)`)
			args = append(args, p.After.Views, p.After.ID)
		}
	case p.Sort == blog.PostSortViews:
		order = `views, id`
		if p.After != nil {
			where = append(where, `(views, id) > (?, ?)`)
			args = append(args, p.After.Views, p.After.ID)
		}
	case p.Sort == blog.PostSortCreatedAt && desc:
		order = `created_at DESC, id DESC`
		if p.After != nil {
			where = append(where, `(created_at, id) < (?, ?)`)
			args = append(args, p.After.CreatedAt, p.After.ID)
		}
	case p.Sort == blog.PostSortCreatedAt:
		order = `created_at, id`
		if p.After != nil {
			where = append(where, `(created_at, id) > (?, ?)`)
			args = append(args, p.After.CreatedAt, p.After.ID)
		}
	case desc:
		order = `id DESC`
		if p.After != nil {
			where = append(where, `id < ?`)
			args = append(args, p.After.ID)
		}
	default:
		order = `id`
		if p.After != nil {
			where = append(where, `id > ?`)
			args = append(args, p.After.ID)
		}
	}

	q := `SELECT id, title, body, published, views, metadata, author, created_at, updated_at FROM posts`
	if len(where) > 0 {
		q += ` WHERE ` + strings.Join(where, ` AND `)
	}
	q += ` ORDER BY ` + order + ` LIMIT ?`
	args = append(args, p.Limit)

	var rows []postRow
	if err := r.db.SelectContext(ctx, &rows, r.db.Rebind(q), args...); err != nil {
		return nil, fmt.Errorf("list post: %w", err)
	}
	out := make([]blog.Post, len(rows))
	for i := range rows {
		out[i] = rows[i].toModel()
	}
	return out, nil
}

func (r *PostRepository) Update(ctx context.Context, m *blog.Post) error {
	row := newPostRow(m)
	if err := r.db.QueryRowContext(ctx, `UPDATE posts SET title = $1, body = $2, published = $3, views = $4, metadata = $5, author = $6, updated_at = now() WHERE id = $7 RETURNING created_at, updated_at`, row.Title, row.Body, row.Published, row.Views, row.Metadata, row.Author, row.ID).Scan(&m.CreatedAt, &m.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return blog.ErrNotFound
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
		return blog.ErrNotFound
	}
	return nil
}
