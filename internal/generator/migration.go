package generator

import (
	"fmt"
	"strconv"
	"strings"

	"go-crudgen/internal/spec"
)

// migrationData is the template input for one entity's goose migration file. The
// DDL is built here rather than in the template (mirroring repo.go) so the column
// math (types, constraints, primary-key placement) stays testable. Each
// statement is rendered as its own goose StatementBegin/StatementEnd block.
type migrationData struct {
	Up   []string // CREATE TABLE, then any CREATE INDEX statements
	Down []string // DROP TABLE statement
}

// sqlType maps a spec field to its PostgreSQL column type. A reference resolves
// to the target entity's primary-key type; validation guarantees the target
// exists with a single primary key, and fieldType already rejects a target whose
// key is itself a reference, so the same case is an error here.
func sqlType(f spec.Field, byName map[string]*spec.Entity) (string, error) {
	t := f.Type
	if t == spec.TypeReferences {
		pk := byName[f.Target].PrimaryKey()[0]
		if pk.Type == spec.TypeReferences {
			return "", fmt.Errorf("reference to %q whose primary key %q is itself a reference (not supported)", f.Target, pk.Name)
		}
		t = pk.Type
	}
	ti, ok := scalarTypes[t]
	if !ok {
		return "", fmt.Errorf("no SQL type for %q", t)
	}
	return ti.sqlType, nil
}

// sqlDefault formats a field's `default` value as a SQL literal, returning "" when
// no default is set. YAML scalars decode to bool, int, float64, or string.
func sqlDefault(v any) (string, error) {
	switch d := v.(type) {
	case nil:
		return "", nil
	case bool:
		if d {
			return "TRUE", nil
		}
		return "FALSE", nil
	case int:
		return strconv.Itoa(d), nil
	case int64:
		return strconv.FormatInt(d, 10), nil
	case float64:
		return strconv.FormatFloat(d, 'g', -1, 64), nil
	case string:
		return "'" + strings.ReplaceAll(d, "'", "''") + "'", nil
	}
	return "", fmt.Errorf("unsupported default value %v (%T)", v, v)
}

// migrationOrder returns the entities ordered so that every entity follows the
// entities it references. goose applies migrations in sequence, so a referenced
// table must be created (numbered) before the table whose foreign key points at
// it. A self-reference is fine inline (the table exists within its own CREATE
// TABLE) and is ignored here; a reference cycle between distinct entities cannot
// be expressed with inline foreign keys and is reported as an error.
func migrationOrder(entities []spec.Entity, byName map[string]*spec.Entity) ([]*spec.Entity, error) {
	const (
		unvisited = iota
		visiting
		done
	)
	state := make(map[string]int, len(entities))
	var order []*spec.Entity
	var visit func(e *spec.Entity) error
	visit = func(e *spec.Entity) error {
		switch state[e.Name] {
		case done:
			return nil
		case visiting:
			return fmt.Errorf("reference cycle involving entity %q (inline foreign keys cannot express it)", e.Name)
		}
		state[e.Name] = visiting
		for _, f := range e.Fields {
			if f.Type != spec.TypeReferences || f.Target == e.Name {
				continue
			}
			if err := visit(byName[f.Target]); err != nil {
				return err
			}
		}
		state[e.Name] = done
		order = append(order, e)
		return nil
	}
	for i := range entities {
		if err := visit(&entities[i]); err != nil {
			return nil, err
		}
	}
	return order, nil
}

// migrationInfo builds the goose migration for an entity. Unlike handlers and
// repositories, a migration is generated for every entity, even one whose
// primary-key type is not HTTP-serveable still needs its table.
func migrationInfo(e *spec.Entity, byName map[string]*spec.Entity) (migrationData, error) {
	table := plural(e.Name, e.Plural)

	var lines, indexes []string
	for _, f := range e.Fields {
		st, err := sqlType(f, byName)
		if err != nil {
			return migrationData{}, fmt.Errorf("entity %q field %q: %w", e.Name, f.Name, err)
		}
		def := sqlNow
		if !isNowDefault(f) {
			def, err = sqlDefault(f.Default)
			if err != nil {
				return migrationData{}, fmt.Errorf("entity %q field %q: %w", e.Name, f.Name, err)
			}
		}

		col := snakeCase(f.Name)
		parts := []string{col, st}
		if !isNullable(f) {
			parts = append(parts, "NOT NULL")
		}
		if gen, ok := keyGenerator(f); ok {
			parts = append(parts, gen)
		}
		if def != "" {
			parts = append(parts, "DEFAULT "+def)
		}
		if f.Primary {
			parts = append(parts, "PRIMARY KEY")
		}
		if f.Unique {
			parts = append(parts, "UNIQUE")
		}
		if f.Type == spec.TypeReferences {
			target := byName[f.Target]
			parts = append(parts, fmt.Sprintf("REFERENCES %s (%s)",
				plural(target.Name, target.Plural), snakeCase(target.PrimaryKey()[0].Name)))
		}
		lines = append(lines, "    "+strings.Join(parts, " "))

		if f.Index && !f.Primary {
			indexes = append(indexes, col)
		}
	}

	// Option-injected columns reuse optionColumns' ordering so the schema and the
	// generated model never disagree on which columns exist. Timestamps are
	// NOT NULL DEFAULT now(); the soft-delete marker is nullable.
	for _, oc := range optionColumns(e.Options) {
		switch oc.Column {
		case colCreatedAt, colUpdatedAt:
			lines = append(lines, fmt.Sprintf("    %s %s NOT NULL DEFAULT now()", oc.Column, sqlTimestamptz))
		case colDeletedAt:
			lines = append(lines, fmt.Sprintf("    %s %s", oc.Column, sqlTimestamptz))
		}
	}

	up := []string{fmt.Sprintf("CREATE TABLE %s (\n%s\n);", table, strings.Join(lines, ",\n"))}
	for _, idx := range indexes {
		up = append(up, fmt.Sprintf("CREATE INDEX idx_%s_%s ON %s (%s);", table, idx, table, idx))
	}

	return migrationData{
		Up:   up,
		Down: []string{fmt.Sprintf("DROP TABLE %s;", table)},
	}, nil
}
