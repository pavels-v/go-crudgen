# go-crudgen

Generates a RESTful Go service (models, DTOs, handlers, router, PostgreSQL repositories, goose migrations) from a YAML entity spec.

## Why

Every backend re-implements the same layer: entity, table, five CRUD handlers, validation, serialization. Other ecosystems generate it:

- Ruby: [Rails scaffold](https://rails.devcamp.com/trails/learn-ruby-on-rails-from-scratch/campsites/building-your-first-rails-application/guides/rails-scaffold-generator-guide) (`rails g scaffold Post title:string`).
- Python: Django REST Framework.
- JavaScript: [NestJS](https://docs.nestjs.com/recipes/crud-generator) (`nest generate resource <name>`) scaffolds module, controller, service, DTOs and entity as editable code.

**Scope:** `go-crudgen` takes the routine out of writing plain CRUD for a set of entities. It is not a replacement for writing code: routes, handlers, the data layer and any service layer are yours to change after generation.

### Why a new spec format

Existing Go tools each cover part of the layer:

- OpenAPI ([oapi-codegen](https://github.com/oapi-codegen/oapi-codegen), [ogen](https://github.com/ogen-go/ogen)): API-shaped and verbose; server stubs only, no storage, migrations or relations.
- Go-code schema ([Ent](https://entgo.io/blog/2021/07/29/generate-a-fully-working-go-crud-http-api-with-ent/) + [ogent](https://github.com/ariga/ogent)): full CRUD, but generated code depends on the Ent ORM and the ogen router.
- API DSL + SQL DDL ([go-zero goctl](https://github.com/zeromicro/go-zero)): two separate inputs, the logic layer is left as stubs, code is tied to go-zero.
- Database introspection ([gofromdb](https://dev.to/alzhi_f93e67fa45b972/i-got-tired-of-writing-crud-by-hand-so-i-built-a-go-code-generator-from-a-database-1h52), [sqlboiler](https://github.com/aarondl/sqlboiler), [xo/dbtpl](https://github.com/xo/dbtpl)): needs an existing database; no migrations or input validation.
- SQL-first ([sqlc](https://github.com/sqlc-dev/sqlc)): queries only, no HTTP layer.
- Framework scaffold ([Buffalo](https://github.com/gobuffalo/buffalo) `generate resource`): full scaffold with migrations, tied to Buffalo and the Pop ORM; archived in February 2024.

Among the tools we found, none generates every layer (validation, handlers, routes, SQL repository, migrations) from one declarative entity spec without a framework or ORM dependency. The `go-crudgen` spec does:

- One YAML file drives model, DTOs, validation, handlers, routes, repository SQL and migrations; no second source of truth.
- Input validation rules (go-playground/validator) sit next to the field they check.
- `references` becomes a foreign key typed from the target's key; migrations are numbered in dependency order.
- Routes use the standard `net/http.ServeMux` (Go 1.22+ patterns), no framework.
- Generated code depends on neither the generator nor an ORM: plain Go plus `sqlx`.
- Repository SQL is written inline at each call, readable and reviewable.
- Storage is driver-agnostic via `database/sql`; `pgx` or `pq` is a flag.
- Nullability flows from the spec: `*T` in the model, `sql.Null[T]` in the repository, no `NOT NULL` in the migration.
- Migrations are goose SQL files, applied with standard tooling.
- Server-set timestamps via `generate: on_create | on_write`.
- Spec-first: works for greenfield projects; the spec is versioned and diff-friendly.
- Plain YAML syntax familiar from OpenAPI and Kubernetes; the generator validates its own rules on top (required fields, known types, exactly one primary key, existing reference targets).
- Handlers depend on a repository interface, so storage can be swapped or faked in tests.

## Usage

```bash
go install ./cmd/go-crudgen                                         # from a clone of this repo
go-crudgen generate --spec ./api.yaml                               # preview on stdout
go-crudgen generate --spec ./api.yaml --out ./internal/api          # write files
go-crudgen generate --spec ./api.yaml --out ./internal/api --driver pq
cd ./internal/api && go mod tidy
```

```go
db, err := postgres.NewDB(dsn)
if err != nil {
	log.Fatal(err)
}
router := restapi.NewRouter(restapi.Deps{
	Posts:   postgres.NewPostRepository(db),
	Authors: postgres.NewAuthorRepository(db),
})
http.ListenAndServe(":8080", router)
```

Developer tasks: `make help`.

## Output

- `<entity>.gen.go` - model and repository interface, in the root package named by `package`.
- `errors.gen.go` - sentinel errors shared by all layers.
- `sort.gen.go` - `SortDir` for List ordering.
- `date.gen.go` - `Date` type, `YYYY-MM-DD` in JSON, emitted when any field is `date`.
- `restapi/<entity>.gen.go` - `Create`/`Update` request DTOs and CRUD handlers.
- `restapi/router.gen.go` - `NewRouter`, `Deps`, JSON and pagination helpers.
- `postgres/<entity>.gen.go` - `sqlx` PostgreSQL repository.
- `postgres/db.gen.go` - `NewDB` with the driver blank-imported (`pgx` default, `pq` via `--driver`).
- `postgres/nulls.gen.go` - `sql.Null[T]` helpers, emitted when any column is nullable.
- `migrations/NNNNN_create_<table>.sql` - goose migrations, numbered in foreign-key order.
- Generation fails when an entity name collides with a generated declaration or file.

| Method   | Path             | Action                                       |
| -------- | ---------------- | -------------------------------------------- |
| `POST`   | `/{plural}`      | Create                                       |
| `GET`    | `/{plural}`      | List (`?limit`, `?offset` or `?cursor`, `?dir`, filters) |
| `GET`    | `/{plural}/{id}` | Read                                         |
| `PUT`    | `/{plural}/{id}` | Update                                       |
| `DELETE` | `/{plural}/{id}` | Delete                                       |

Responses: `{"body": ...}` on success (List: `{"items", "limit", "offset", "has_more"}`, or `{"items", "next_cursor"}` under cursor pagination), `{"error": {"code", "message", "details"}}` on failure; `DELETE` returns `204` with no body.

Errors: `400` malformed body, id or query, `404` not found, `405` method not allowed, `409` unique violation or deleting a referenced row, `413` body over 1 MiB, `422` failed validation or unknown reference, `500` without internal details.

## Spec

Full example: [examples/blog.yaml](examples/blog.yaml), generated output: [examples/blogservice/internal/blog](examples/blogservice/internal/blog).

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
      - { name: created_at, type: datetime, generate: on_create }
      - { name: updated_at, type: datetime, generate: on_write }
```

- `order: <field>` sets the List order (default: primary key, ties broken by it); clients pick `?dir=asc|desc` (default `asc`); the field must be NOT NULL.
- `pagination: cursor` pages List by an opaque keyset `?cursor=` instead of `?offset=`.
- `package` names the root package; `module` is the import path of the `--out` directory.
- Types: `string`, `text`, `int32`, `int64`, `float`, `decimal`, `bool`, `date`, `datetime`, `uuid`, `json`, `references`.
- Modifiers: `primary`, `required`, `unique`, `index`, `default`, `validate` (go-playground/validator rules), `on_delete: cascade` (references only), `filter`, `generate`.
- `required` fields must be present in the request body; `false` and `0` are accepted, an empty `string` or `text` is not.
- `validate` rules are checked against the field's Go type at generation; on optional fields they apply only when the value is present.
- `generate: on_create` sets a `datetime` to `now()` on insert, `generate: on_write` on insert and every update; both are read-only in the API.
- `filter: true` makes List accept `?<name>=<value>` (equality); unknown or repeated query parameters are rejected; pair filters and `order` with `index: true`.
- Exactly one `primary` field per entity, typed `string`, `text`, `int32`, `int64`, `uuid` or `references`.
- `uuid` keys are generated in Go (v4) with `gen_random_uuid()` as the column default; `int32` and `int64` keys use `IDENTITY`; other keys are required in Create.
- `default` takes a literal of the field's type (`string`, `text`, `int32`, `int64`, `float`, `bool`) or `now` for `datetime`; the column is `NOT NULL DEFAULT` and the handler fills an omitted value.
- Fields with none of `primary`, `required`, `default` are nullable: `*T` in the model, `sql.Null[T]` in the repository.
- `references` becomes a column typed from the target's key with a `REFERENCES` constraint.

## Roadmap

- [x] YAML spec parser and validation
- [x] Model + struct-tag generation
- [x] CRUD handler generation (`net/http`)
- [x] PostgreSQL repository (`sqlx` / `database/sql`)
- [x] Migrations (goose SQL schema per entity)
- [x] List endpoint: pagination (`?limit` / `?offset`)
- [x] List endpoint: cursor pagination (`?cursor`)
- [x] List endpoint: filtering (equality)
- [x] List endpoint: ordering (`order` + `?dir`)
- [x] Relations (`belongs_to` / `has_many`)
  - [x] `belongs_to` via `references` fields
  - [x] `has_many` via `filter: true` on the reference field
- [ ] Pluggable storage backends (SQLite, in-memory)
- [ ] Auth/middleware hooks

## License

[MIT](LICENSE) © Pavel Sizikov
