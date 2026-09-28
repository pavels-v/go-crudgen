package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"

	domain "example.com/blogservice/internal/blog"
)

// tagRow is the database representation of domain.Tag.
type tagRow struct {
	Slug   string  `db:"slug"`
	Label  string  `db:"label"`
	Color  string  `db:"color"`
	Weight float64 `db:"weight"`
	Group  string  `db:"group"`
}

// newTagRow builds the row written by Create and Update.
func newTagRow(m *domain.Tag) tagRow {
	return tagRow{
		Slug:   m.Slug,
		Label:  m.Label,
		Color:  m.Color,
		Weight: m.Weight,
		Group:  m.Group,
	}
}

// toModel converts a scanned row back into the API model.
func (row tagRow) toModel() domain.Tag {
	return domain.Tag{
		Slug:   row.Slug,
		Label:  row.Label,
		Color:  row.Color,
		Weight: row.Weight,
		Group:  row.Group,
	}
}

// TagRepository is a PostgreSQL-backed domain.TagRepository.
type TagRepository struct {
	db *sqlx.DB
}

// NewTagRepository returns a repository backed by db.
func NewTagRepository(db *sqlx.DB) *TagRepository {
	return &TagRepository{db: db}
}

var _ domain.TagRepository = (*TagRepository)(nil)

func (r *TagRepository) Create(ctx context.Context, m *domain.Tag) error {
	row := newTagRow(m)

	if _, err := r.db.ExecContext(ctx, `INSERT INTO tags (slug, label, color, weight, "group") VALUES ($1, $2, $3, $4, $5)`, row.Slug, row.Label, row.Color, row.Weight, row.Group); err != nil {
		return fmt.Errorf("create tag: %w", mapWriteError(err))
	}

	return nil
}

func (r *TagRepository) Get(ctx context.Context, id string) (*domain.Tag, error) {
	var row tagRow
	if err := r.db.GetContext(ctx, &row, `SELECT slug, label, color, weight, "group" FROM tags WHERE slug = $1`, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}

		return nil, fmt.Errorf("get tag: %w", err)
	}

	m := row.toModel()

	return &m, nil
}

func (r *TagRepository) List(ctx context.Context, p domain.TagListParams) ([]domain.Tag, error) {
	order := `"group", slug`
	if p.Dir == domain.SortDesc {
		order = `"group" DESC, slug DESC`
	}

	var (
		where []string
		args  []any
	)

	if p.Group != nil {
		where = append(where, `"group" = ?`)
		args = append(args, *p.Group)
	}

	q := `SELECT slug, label, color, weight, "group" FROM tags`
	if len(where) > 0 {
		q += ` WHERE ` + strings.Join(where, ` AND `)
	}

	args = append(args, p.Limit, p.Offset)
	q += ` ORDER BY ` + order + ` LIMIT ? OFFSET ?`

	var rows []tagRow
	if err := r.db.SelectContext(ctx, &rows, r.db.Rebind(q), args...); err != nil {
		return nil, fmt.Errorf("list tag: %w", err)
	}

	out := make([]domain.Tag, len(rows))
	for i := range rows {
		out[i] = rows[i].toModel()
	}

	return out, nil
}

func (r *TagRepository) Update(ctx context.Context, m *domain.Tag) error {
	row := newTagRow(m)

	res, err := r.db.ExecContext(ctx, `UPDATE tags SET label = $1, color = $2, weight = $3, "group" = $4 WHERE slug = $5`, row.Label, row.Color, row.Weight, row.Group, row.Slug)
	if err != nil {
		return fmt.Errorf("update tag: %w", mapWriteError(err))
	}

	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update tag: %w", err)
	}

	if n == 0 {
		return domain.ErrNotFound
	}

	return nil
}

func (r *TagRepository) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM tags WHERE slug = $1`, id)
	if err != nil {
		return fmt.Errorf("delete tag: %w", mapDeleteError(err))
	}

	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete tag: %w", err)
	}

	if n == 0 {
		return domain.ErrNotFound
	}

	return nil
}
