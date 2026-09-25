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

const (
	createCommentSQL = `INSERT INTO comments (post, body, likes, posted_at) VALUES ($1, $2, $3, $4) RETURNING id`
	getCommentSQL    = `SELECT id, post, body, likes, posted_at, deleted_at FROM comments WHERE id = $1 AND deleted_at IS NULL`
	listCommentSQL   = `SELECT id, post, body, likes, posted_at, deleted_at FROM comments WHERE deleted_at IS NULL ORDER BY id LIMIT $1 OFFSET $2`
	updateCommentSQL = `UPDATE comments SET post = $1, body = $2, likes = $3, posted_at = $4 WHERE id = $5 AND deleted_at IS NULL`
	deleteCommentSQL = `UPDATE comments SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`
)

// commentRow is the database representation of Comment. Nullable
// columns use sql.Null[T] so a SQL NULL round-trips as an absent value, which
// newCommentRow and toModel convert to and from the pointer fields on Comment.
type commentRow struct {
	ID        int64               `db:"id"`
	Post      uuid.UUID           `db:"post"`
	Body      string              `db:"body"`
	Likes     int32               `db:"likes"`
	PostedAt  time.Time           `db:"posted_at"`
	DeletedAt sql.Null[time.Time] `db:"deleted_at"`
}

// newCommentRow builds the row written by Create and Update. Option-managed
// columns (timestamps) are set by the SQL itself, so they are omitted here.
func newCommentRow(m *Comment) commentRow {
	return commentRow{
		ID:       m.ID,
		Post:     m.Post,
		Body:     m.Body,
		Likes:    m.Likes,
		PostedAt: m.PostedAt,
	}
}

// toModel converts a scanned row back into the API model.
func (row commentRow) toModel() Comment {
	return Comment{
		ID:        row.ID,
		Post:      row.Post,
		Body:      row.Body,
		Likes:     row.Likes,
		PostedAt:  row.PostedAt,
		DeletedAt: fromNull(row.DeletedAt),
	}
}

// PostgresCommentRepository is a PostgreSQL-backed CommentRepository. It depends on sqlx rather
// than a concrete driver, so any database/sql-compatible Postgres driver
// (lib/pq, pgx's stdlib adapter, ...) can back it.
type PostgresCommentRepository struct {
	db *sqlx.DB
}

// NewPostgresCommentRepository returns a PostgresCommentRepository backed by db.
func NewPostgresCommentRepository(db *sqlx.DB) *PostgresCommentRepository {
	return &PostgresCommentRepository{db: db}
}

var _ CommentRepository = (*PostgresCommentRepository)(nil)

func (r *PostgresCommentRepository) Create(ctx context.Context, m *Comment) error {
	row := newCommentRow(m)
	if err := r.db.QueryRowContext(ctx, createCommentSQL, row.Post, row.Body, row.Likes, row.PostedAt).Scan(&m.ID); err != nil {
		return fmt.Errorf("create comment: %w", mapWriteError(err))
	}
	return nil
}

func (r *PostgresCommentRepository) Get(ctx context.Context, id int64) (*Comment, error) {
	var row commentRow
	if err := r.db.GetContext(ctx, &row, getCommentSQL, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get comment: %w", err)
	}
	m := row.toModel()
	return &m, nil
}

func (r *PostgresCommentRepository) List(ctx context.Context, limit, offset int) ([]Comment, error) {
	rows := []commentRow{}
	if err := r.db.SelectContext(ctx, &rows, listCommentSQL, limit, offset); err != nil {
		return nil, fmt.Errorf("list comment: %w", err)
	}
	out := make([]Comment, len(rows))
	for i := range rows {
		out[i] = rows[i].toModel()
	}
	return out, nil
}

func (r *PostgresCommentRepository) Update(ctx context.Context, m *Comment) error {
	row := newCommentRow(m)
	res, err := r.db.ExecContext(ctx, updateCommentSQL, row.Post, row.Body, row.Likes, row.PostedAt, row.ID)
	if err != nil {
		return fmt.Errorf("update comment: %w", mapWriteError(err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update comment: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresCommentRepository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, deleteCommentSQL, id)
	if err != nil {
		return fmt.Errorf("delete comment: %w", mapDeleteError(err))
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete comment: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
