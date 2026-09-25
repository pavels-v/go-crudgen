-- +goose Up
-- +goose StatementBegin
CREATE TABLE tags (
    slug TEXT NOT NULL PRIMARY KEY,
    label TEXT NOT NULL,
    color TEXT NOT NULL DEFAULT 'gray',
    weight DOUBLE PRECISION NOT NULL DEFAULT 1
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE tags;
-- +goose StatementEnd
