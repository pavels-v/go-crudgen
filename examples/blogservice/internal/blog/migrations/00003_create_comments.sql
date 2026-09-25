-- +goose Up
-- +goose StatementBegin
CREATE TABLE comments (
    id BIGINT NOT NULL GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    post UUID NOT NULL REFERENCES posts (id),
    body TEXT NOT NULL,
    likes INTEGER NOT NULL DEFAULT 0,
    posted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_comments_post ON comments (post);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE comments;
-- +goose StatementEnd
