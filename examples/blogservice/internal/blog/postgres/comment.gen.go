package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"example.com/blogservice/internal/blog"
)

// commentRow is the database representation of blog.Comment. Nullable
// columns use sql.Null[T] so a SQL NULL round-trips as an absent value, which
// newCommentRow and toModel convert to and from the pointer fields on blog.Comment.
type commentRow struct {
	ID       int64     `db:"id"`
	Post     uuid.UUID `db:"post"`
	Body     string    `db:"body"`
	Likes    int32     `db:"likes"`
	PostedAt time.Time `db:"posted_at"`
}

// newCommentRow builds the row written by Create and Update. Option-managed
// columns (timestamps) are set by the SQL itself, so they are omitted here.
func newCommentRow(m *blog.Comment) commentRow {
	return commentRow{
		ID:       m.ID,
		Post:     m.Post,
		Body:     m.Body,
		Likes:    m.Likes,
		PostedAt: m.PostedAt,
	}
}

// toModel converts a scanned row back into the API model.
func (row commentRow) toModel() blog.Comment {
	return blog.Comment{
		ID:       row.ID,
		Post:     row.Post,
		Body:     row.Body,
		Likes:    row.Likes,
		PostedAt: row.PostedAt,
	}
}

// CommentRepository is a PostgreSQL-backed blog.CommentRepository. It depends on sqlx rather
// than a concrete driver, so any database/sql-compatible Postgres driver
// (lib/pq, pgx's stdlib adapter, ...) can back it.
type CommentRepository struct {
	db *sqlx.DB
}

// NewCommentRepository returns a repository backed by db.
func NewCommentRepository(db *sqlx.DB) *CommentRepository {
	return &CommentRepository{db: db}
}

var _ blog.CommentRepository = (*CommentRepository)(nil)

func (r *CommentRepository) Create(ctx context.Context, m *blog.Comment) error {
	row := newCommentRow(m)
	if err := r.db.QueryRowContext(ctx, `INSERT INTO comments (post, body, likes, posted_at) VALUES ($1, $2, $3, $4) RETURNING id`, row.Post, row.Body, row.Likes, row.PostedAt).Scan(&m.ID); err != nil {
		return fmt.Errorf("create comment: %w", mapWriteError(err))
	}
	return nil
}

func (r *CommentRepository) Get(ctx context.Context, id int64) (*blog.Comment, error) {
	var row commentRow
	if err := r.db.GetContext(ctx, &row, `SELECT id, post, body, likes, posted_at FROM comments WHERE id = $1`, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, blog.ErrNotFound
		}
		return nil, fmt.Errorf("get comment: %w", err)
	}
	m := row.toModel()
	return &m, nil
}

func (r *CommentRepository) List(ctx context.Context, p blog.CommentListParams) ([]blog.Comment, error) {
	var (
		where []string
		args  []any
	)
	if p.Post != nil {
		where = append(where, `post = ?`)
		args = append(args, *p.Post)
	}

	q := `SELECT id, post, body, likes, posted_at FROM comments`
	if len(where) > 0 {
		q += ` WHERE ` + strings.Join(where, ` AND `)
	}
	switch p.Sort {
	case blog.CommentSortPostedAt:
		q += ` ORDER BY posted_at, id`
	case blog.CommentSortPostedAtDesc:
		q += ` ORDER BY posted_at DESC, id`
	default:
		q += ` ORDER BY id`
	}
	q += ` LIMIT ? OFFSET ?`
	args = append(args, p.Limit, p.Offset)

	var rows []commentRow
	if err := r.db.SelectContext(ctx, &rows, r.db.Rebind(q), args...); err != nil {
		return nil, fmt.Errorf("list comment: %w", err)
	}
	out := make([]blog.Comment, len(rows))
	for i := range rows {
		out[i] = rows[i].toModel()
	}
	return out, nil
}

func (r *CommentRepository) Update(ctx context.Context, m *blog.Comment) error {
	row := newCommentRow(m)
	res, err := r.db.ExecContext(ctx, `UPDATE comments SET post = $1, body = $2, likes = $3, posted_at = $4 WHERE id = $5`, row.Post, row.Body, row.Likes, row.PostedAt, row.ID)
	if err != nil {
		return fmt.Errorf("update comment: %w", mapWriteError(err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update comment: %w", err)
	}
	if n == 0 {
		return blog.ErrNotFound
	}
	return nil
}

func (r *CommentRepository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM comments WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete comment: %w", mapDeleteError(err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete comment: %w", err)
	}
	if n == 0 {
		return blog.ErrNotFound
	}
	return nil
}
