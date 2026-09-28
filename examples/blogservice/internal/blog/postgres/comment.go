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

	domain "example.com/blogservice/internal/blog"
)

// commentRow is the database representation of domain.Comment.
type commentRow struct {
	ID       int64     `db:"id"`
	Post     uuid.UUID `db:"post"`
	Body     string    `db:"body"`
	Likes    int32     `db:"likes"`
	PostedAt time.Time `db:"posted_at"`
}

// newCommentRow builds the row written by Create and Update.
func newCommentRow(m *domain.Comment) commentRow {
	return commentRow{
		ID:       m.ID,
		Post:     m.Post,
		Body:     m.Body,
		Likes:    m.Likes,
		PostedAt: m.PostedAt,
	}
}

// toModel converts a scanned row back into the API model.
func (row commentRow) toModel() domain.Comment {
	return domain.Comment{
		ID:       row.ID,
		Post:     row.Post,
		Body:     row.Body,
		Likes:    row.Likes,
		PostedAt: row.PostedAt,
	}
}

// CommentRepository is a PostgreSQL-backed domain.CommentRepository.
type CommentRepository struct {
	db *sqlx.DB
}

// NewCommentRepository returns a repository backed by db.
func NewCommentRepository(db *sqlx.DB) *CommentRepository {
	return &CommentRepository{db: db}
}

var _ domain.CommentRepository = (*CommentRepository)(nil)

func (r *CommentRepository) Create(ctx context.Context, m *domain.Comment) error {
	row := newCommentRow(m)

	err := r.db.QueryRowContext(ctx,
		`INSERT INTO comments (post, body, likes, posted_at) VALUES ($1, $2, $3, $4) RETURNING id`,
		row.Post, row.Body, row.Likes, row.PostedAt,
	).Scan(&m.ID)
	if err != nil {
		return fmt.Errorf("create comment: %w", mapWriteError(err))
	}

	return nil
}

func (r *CommentRepository) Get(ctx context.Context, id int64) (*domain.Comment, error) {
	var row commentRow

	err := r.db.GetContext(ctx, &row,
		`SELECT id, post, body, likes, posted_at FROM comments WHERE id = $1`,
		id,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}

		return nil, fmt.Errorf("get comment: %w", err)
	}

	m := row.toModel()

	return &m, nil
}

func (r *CommentRepository) List(ctx context.Context, p domain.CommentListParams) ([]domain.Comment, error) {
	order := `posted_at, id`
	if p.Dir == domain.SortDesc {
		order = `posted_at DESC, id DESC`
	}

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

	args = append(args, p.Limit, p.Offset)
	q += ` ORDER BY ` + order + ` LIMIT ? OFFSET ?`

	var rows []commentRow

	err := r.db.SelectContext(ctx, &rows,
		r.db.Rebind(q),
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("list comment: %w", err)
	}

	out := make([]domain.Comment, len(rows))
	for i := range rows {
		out[i] = rows[i].toModel()
	}

	return out, nil
}

func (r *CommentRepository) Update(ctx context.Context, m *domain.Comment) error {
	row := newCommentRow(m)

	res, err := r.db.ExecContext(ctx,
		`UPDATE comments SET post = $1, body = $2, likes = $3, posted_at = $4 WHERE id = $5`,
		row.Post, row.Body, row.Likes, row.PostedAt, row.ID,
	)
	if err != nil {
		return fmt.Errorf("update comment: %w", mapWriteError(err))
	}

	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update comment: %w", err)
	}

	if n == 0 {
		return domain.ErrNotFound
	}

	return nil
}

func (r *CommentRepository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM comments WHERE id = $1`,
		id,
	)
	if err != nil {
		return fmt.Errorf("delete comment: %w", mapDeleteError(err))
	}

	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete comment: %w", err)
	}

	if n == 0 {
		return domain.ErrNotFound
	}

	return nil
}
