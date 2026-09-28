package postgres

import "database/sql"

// toNull wraps a pointer as a nullable SQL value: a nil pointer becomes SQL NULL.
func toNull[T any](p *T) sql.Null[T] {
	if p == nil {
		return sql.Null[T]{}
	}

	return sql.Null[T]{V: *p, Valid: true}
}

// fromNull unwraps a nullable SQL value into a pointer: SQL NULL becomes nil.
func fromNull[T any](n sql.Null[T]) *T {
	if !n.Valid {
		return nil
	}

	return &n.V
}
