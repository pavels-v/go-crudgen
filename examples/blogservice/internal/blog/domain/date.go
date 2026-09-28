package domain

import (
	"database/sql"
	"database/sql/driver"
	"encoding"
	"fmt"
	"time"
)

var (
	_ encoding.TextMarshaler   = Date{}
	_ encoding.TextUnmarshaler = (*Date)(nil)
	_ driver.Valuer            = Date{}
	_ sql.Scanner              = (*Date)(nil)
)

type Date time.Time //nolint:recvcheck // decoders need a pointer receiver

func (d Date) MarshalText() ([]byte, error) {
	return []byte(time.Time(d).Format(time.DateOnly)), nil
}

func (d *Date) UnmarshalText(b []byte) error {
	t, err := time.Parse(time.DateOnly, string(b))
	if err != nil {
		return fmt.Errorf("parse date: %w", err)
	}

	*d = Date(t)

	return nil
}

func (d Date) Value() (driver.Value, error) {
	return time.Time(d).Format(time.DateOnly), nil
}

func (d *Date) Scan(src any) error {
	t, ok := src.(time.Time)
	if !ok {
		return fmt.Errorf("scan date: unsupported type %T", src)
	}

	*d = Date(t)

	return nil
}
