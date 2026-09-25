# go-crudgen

Generates a RESTful Go service (models, DTOs, handlers, router, PostgreSQL repositories, goose migrations) from a YAML entity spec.

## Usage

```bash
go install ./cmd/go-crudgen                                         # from a clone of this repo
go-crudgen generate --spec ./api.yaml                               # preview on stdout
go-crudgen generate --spec ./api.yaml --out ./internal/api          # write files
go-crudgen generate --spec ./api.yaml --out ./internal/api --driver pq
cd ./internal/api && go mod tidy
```

```go
db, err := blog.NewDB(dsn)
if err != nil {
	log.Fatal(err)
}
router := blog.NewRouter(blog.Deps{
	Posts:   blog.NewPostgresPostRepository(db),
	Authors: blog.NewPostgresAuthorRepository(db),
})
http.ListenAndServe(":8080", router)
```

Developer tasks: `make help`.

## Output

- `<entity>.gen.go` - model and `Create`/`Update` request DTOs.
- `<entity>_handler.gen.go` - repository interface and CRUD handlers.
- `<entity>_repo.gen.go` - `sqlx` PostgreSQL repository.
- `http.gen.go` - `NewRouter`, `Deps`, JSON and pagination helpers.
- `db.gen.go` - `NewDB` with the driver blank-imported (`pgx` default, `pq` via `--driver`).
- `nulls.gen.go` - `sql.Null[T]` helpers, emitted when any column is nullable.
- `migrations/NNNNN_create_<table>.sql` - goose migrations, numbered in foreign-key order.

| Method   | Path             | Action                           |
| -------- | ---------------- | -------------------------------- |
| `POST`   | `/{plural}`      | Create                           |
| `GET`    | `/{plural}`      | List (`?limit`, `?offset`)       |
| `GET`    | `/{plural}/{id}` | Read                             |
| `PUT`    | `/{plural}/{id}` | Update                           |
| `DELETE` | `/{plural}/{id}` | Delete                           |

## Spec

Full example: [examples/blog.yaml](examples/blog.yaml), generated output: [examples/blog](examples/blog).

```yaml
package: blog
module: example.com/blog
entities:
  - name: Post
    plural: posts
    fields:
      - { name: id, type: uuid, primary: true }
      - { name: title, type: string, required: true, validate: "min=1,max=200" }
      - { name: author, type: references, target: Author }
    options:
      timestamps: true
      soft_delete: false
```

- Types: `string`, `text`, `int32`, `int64`, `float`, `decimal`, `bool`, `date`, `datetime`, `uuid`, `json`, `references`.
- Modifiers: `primary`, `required`, `unique`, `index`, `default`, `validate` (go-playground/validator rules).
- Exactly one `primary` field per entity, typed `string`, `text`, `int32`, `int64`, `uuid` or `references`.
- Fields that are neither `primary` nor `required` are nullable: `*T` in the model, `sql.Null[T]` in the repository.
- `references` becomes a column typed from the target's key with a `REFERENCES` constraint.

## Roadmap

- [x] YAML spec parser and validation
- [x] Model + struct-tag generation
- [x] CRUD handler generation (`net/http`)
- [x] PostgreSQL repository (`sqlx` / `database/sql`)
- [x] Migrations (goose SQL schema per entity)
- [x] List endpoint: pagination (`?limit` / `?offset`)
- [ ] List endpoint: filtering
- [ ] List endpoint: sorting
- [~] Relations (`belongs_to` / `has_many`)
  - [x] `belongs_to` via `references` fields
  - [ ] `has_many`, nested/relation routes, and JOIN-based loading
- [ ] OpenAPI 3 document export
- [ ] Pluggable storage backends (SQLite, in-memory)
- [ ] Auth/middleware hooks

## Known issues

Each line is removed when its fix lands.

- [ ] **[remove when fixed]** `PUT` returns zero `created_at`: Update SQL returns only `updated_at`; fix by `RETURNING created_at, updated_at`.
- [ ] **[remove when fixed]** `default` never applies to nullable fields: the repository inserts an explicit `NULL`; fix by making a field with `default` `NOT NULL` and filling the default in the DTO (alternatives: skip nil columns in `INSERT`, or `COALESCE`).
- [ ] **[remove when fixed]** Primary key is optional in Create: an omitted `id` is stored as the zero value and the next one fails with 500; fix by `required` on the key in the Create DTO, or DB-generated keys (`gen_random_uuid()` / `IDENTITY`) with `id` dropped from the DTO.
- [ ] **[remove when fixed]** Constraint violations return 500 with raw Postgres text: map `23505` (unique) and `23503` (foreign key) to dedicated sentinels with their own status (409 / 422), and stop exposing internal error text in 500 responses.
- [ ] **[remove when fixed]** `decodeJSON` accepts trailing data after the JSON body: fix by rejecting when `dec.More()` or a second `Decode` does not return `io.EOF`.
- [ ] **[remove when fixed]** Unknown spec keys are ignored silently (`requried`, `softdelete`): fix by decoding with `yaml.Decoder.KnownFields(true)`.
- [ ] **[remove when fixed]** Duplicate field names and fields colliding with option columns (`created_at`, `updated_at`, `deleted_at`) pass validation and produce uncompilable code: fix by rejecting duplicate snake_case column names in `Validate`.

## License

[MIT](LICENSE) © Pavel Sizikov
