# go-crudgen

[![Go Reference](https://pkg.go.dev/badge/github.com/pavels-v/go-crudgen.svg)](https://pkg.go.dev/github.com/pavels-v/go-crudgen)
[![Go Report Card](https://goreportcard.com/badge/github.com/pavels-v/go-crudgen)](https://goreportcard.com/report/github.com/pavels-v/go-crudgen)
[![Release](https://img.shields.io/github/v/release/pavels-v/go-crudgen)](https://github.com/pavels-v/go-crudgen/releases)
[![Go Version](https://img.shields.io/github/go-mod/go-version/pavels-v/go-crudgen)](go.mod)
[![License](https://img.shields.io/github/license/pavels-v/go-crudgen)](LICENSE)

Generates a RESTful Go service (models, DTOs, handlers, router, PostgreSQL repositories, goose migrations) from a YAML entity spec.

## Why

Every backend re-implements the same layer: entity, table, five CRUD handlers, validation, serialization. Other ecosystems generate it:

- Ruby: [Rails scaffold](https://rails.devcamp.com/trails/learn-ruby-on-rails-from-scratch/campsites/building-your-first-rails-application/guides/rails-scaffold-generator-guide) (`rails g scaffold Post title:string`).
- Python: Django REST Framework.
- JavaScript: [NestJS](https://docs.nestjs.com/recipes/crud-generator) (`nest generate resource <name>`) scaffolds module, controller, service, DTOs and entity as editable code.

**Scope:** `go-crudgen` takes the routine out of writing plain CRUD for a set of entities. It is not a replacement for writing code: routes, handlers, the data layer and any service layer are yours to change after generation. Generation is one-shot: a second run into the same `--out` fails instead of overwriting code or migrations, and the project evolves by hand from there.

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
- `references` becomes a foreign key typed from the target's key; migrations are timestamped in dependency order.
- Routes use the standard `net/http.ServeMux`, no framework; generated code needs Go 1.27+ (`encoding/json/v2`).
- Generated code depends on neither the generator nor an ORM: plain Go plus `sqlx`.
- Repository SQL is written inline at each call, readable and reviewable.
- Storage is driver-agnostic via `database/sql`; `pgx` or `pq` is a flag.
- Nullability flows from the spec: `*T` in the model, `sql.Null[T]` in the repository, no `NOT NULL` in the migration.
- Column and table names that are reserved SQL words are quoted, so fields like `user`, `order` or `group` work.
- Migrations are goose SQL files, applied with standard tooling.
- Server-set timestamps via `generate: on_create | on_write`.
- Spec-first: works for greenfield projects; the spec is versioned and diff-friendly.
- Plain YAML syntax familiar from OpenAPI and Kubernetes; the generator validates its own rules on top (required fields, known types, exactly one primary key, existing reference targets).
- Handlers depend on a repository interface, so storage can be swapped or faked in tests.

## Install

```bash
go install github.com/pavels-v/go-crudgen/cmd/go-crudgen@latest
```

Requires Go 1.27+ for the generator and the generated code; the service needs PostgreSQL; `make integration` needs Docker.

## Quick start

```bash
mkdir blog && cd blog && go mod init example.com/blog
go-crudgen generate --spec api.yaml --out ./internal/blog   # api.yaml: the spec below
go mod tidy
DATABASE_URL='postgres://user:pass@localhost:5432/blog?sslmode=disable' go run ./cmd/blog
```

```bash
$ curl -s -X POST localhost:8080/authors -d '{"email":"ada@example.com"}'
{"body":{"id":"358fbc76-73d5-4053-8eb4-bfe4c8681d07","email":"ada@example.com"}}
$ curl -s localhost:8080/authors
{"body":{"items":[{"id":"358fbc76-73d5-4053-8eb4-bfe4c8681d07","email":"ada@example.com"}],"limit":50,"offset":0,"has_more":false}}
$ curl -s -X POST localhost:8080/authors -d '{"email":"nope"}'
{"error":{"code":"validation_failed","message":"request body failed validation","details":[{"field":"/email","reason":"email"}]}}
```

## Usage

```bash
go-crudgen generate --spec ./api.yaml                                            # preview on stdout
go-crudgen generate --spec ./api.yaml --out ./internal/api                       # write files
go-crudgen generate --spec ./api.yaml --out ./internal/api --driver pq
go-crudgen generate --spec ./api.yaml --out ./internal/api --main ./cmd/api      # main.go elsewhere than cmd/<package>
go-crudgen generate --spec ./api.yaml --out ./internal/api --no-router --no-main # wire the service yourself
go-crudgen generate --spec ./api.yaml --out ./internal/api --no-tests            # skip the handler tests
go-crudgen generate --spec ./api.yaml --out ./internal/api --dry-run             # run every check, list the files, write nothing
cd ./internal/api && go mod tidy
```

`--out` must hold none of the files to be written, no code but tests in `domain/`, `restapi/` and `postgres/`, and nothing in `migrations/`; to start over, delete them and generate again.

With `--no-router --no-main` the wiring is yours:

```go
db, err := postgres.NewDB(dsn)
if err != nil {
    log.Fatal(err)
}

mux := http.NewServeMux()
restapi.NewPostHandler(postgres.NewPostRepository(db)).RegisterRoutes(mux)
restapi.NewAuthorHandler(postgres.NewAuthorRepository(db)).RegisterRoutes(mux)
http.ListenAndServe(":8080", restapi.WithRouteErrors(mux))
```

Developer tasks: `make help`.

## Output

- `domain/<entity>.go` - model and List params (plus the cursor type under cursor pagination).
- `domain/errors.go` - sentinel errors shared by all layers.
- `domain/sort.go` - `SortDir` for List ordering.
- `domain/date.go` - `Date` type, `YYYY-MM-DD` in JSON, emitted when any field is `date`.
- `restapi/<entity>.go` - repository interface, `Create`/`Update` request DTOs, CRUD handlers and `RegisterRoutes`.
- `restapi/request.go`, `response.go`, `query.go` - request decoding, response envelope, List query parsing.
- `restapi/routes.go` - `WithRouteErrors`: the JSON envelope for unmatched routes.
- `restapi/router.go` - `NewRouter` and `Deps` wiring every entity, unless `--no-router`.
- `restapi/<entity>_test.go` and `restapi/fake_test.go`, unless `--no-tests`: table-driven handler tests over an in-memory fake repository, with request fixtures picked to pass each field's `validate` rules.
- `postgres/<entity>.go` - `sqlx` PostgreSQL repository.
- `postgres/db.go` - `NewDB` with the driver blank-imported (`pgx` default, `pq` via `--driver`).
- `postgres/nulls.go` - `sql.Null[T]` helpers, emitted when any column is nullable.
- `migrations/<timestamp>_create_<table>.sql` - goose migrations, a second apart in foreign-key order; `SOURCE_DATE_EPOCH` pins the first timestamp.
- `migrations/embed.go` and `cmd/<package>/main.go` next to the nearest `go.mod` (or `--main <dir>`; with no `go.mod` above `--out`, pass `--main` or `--no-main`), unless `--no-main`: a service that reads `DATABASE_URL` and `HTTP_ADDR` (default `:8080`), applies the embedded migrations, serves the API and shuts down gracefully.
- Generation fails when an entity name collides with a generated declaration or file.

| Method   | Path             | Action                                                   |
| -------- | ---------------- | -------------------------------------------------------- |
| `POST`   | `/{plural}`      | Create                                                   |
| `GET`    | `/{plural}`      | List (`?limit`, `?offset` or `?cursor`, `?dir`, filters) |
| `GET`    | `/{plural}/{id}` | Read                                                     |
| `PUT`    | `/{plural}/{id}` | Update                                                   |
| `DELETE` | `/{plural}/{id}` | Delete                                                   |

`?limit` defaults to 50 and is capped at 200; a cursor only continues the `?dir` it was issued for.

Responses: `{"body": ...}` on success (List: `{"items", "limit", "offset", "has_more"}`, or `{"items", "next_cursor"}` under cursor pagination), `{"error": {"code", "message", "details"}}` on failure; `DELETE` returns `204` with no body.

Errors: `400` malformed body, id or query, `404` not found, `405` method not allowed, `409` unique violation or deleting a referenced row, `413` body over 1 MiB, `422` failed validation or unknown reference, `500` without internal details.

## Spec

Full example: [examples/blog.yaml](examples/blog.yaml), generated output: [examples/blogservice/internal/blog](examples/blogservice/internal/blog).

```yaml
package: blog
entities:
  - name: Post
    plural: posts
    fields:
      - { name: id, type: uuid, primary: true }
      - { name: title, type: string, required: true, validate: "min=1,max=200" }
      - { name: author, type: references, target: Author }
      - { name: created_at, type: datetime, generate: on_create }
      - { name: updated_at, type: datetime, generate: on_write }
  - name: Author
    fields:
      - { name: id, type: uuid, primary: true }
      - { name: email, type: string, required: true, unique: true, validate: "email" }
```

- `order: <field>` sets the List order (default: primary key, ties broken by it); clients pick `?dir=asc|desc` (default `asc`); the field must be NOT NULL and not `bool` or `json`.
- `pagination: cursor` pages List by an opaque keyset `?cursor=` instead of `?offset=`.
- `plural` sets the route segment (letters, digits, `-`, `_`) and, in snake_case, the table name; the default is a naive English plural of the snake_case name; two entities cannot share a table.
- `package` names the service: `main.go` goes to `cmd/<package>`.
- `module` is the import path of the `--out` directory, derived from the nearest `go.mod` when omitted and checked against it when set; required without `--out` or a `go.mod`.
- Types: `string`, `text`, `int32`, `int64`, `float`, `decimal`, `bool`, `date`, `datetime`, `uuid`, `json`, `references`.
- Modifiers: `primary`, `required`, `unique`, `index`, `default`, `validate` (go-playground/validator rules), `on_delete: cascade` (references only), `filter`, `generate`.
- `required` fields must be present in the request body; `false` and `0` are accepted, an empty `string` or `text` is not.
- `validate` rules are checked against the field's Go type at generation; on optional fields they apply only when the value is present.
- `date`, `datetime` and `decimal` take only presence rules (`required*`, `excluded*`, `omitempty`, `omitnil`, `omitzero`); `uuid` also takes the `uuid*` rules.
- `generate: on_create` sets a `datetime` to `now()` on insert, `generate: on_write` on insert and every update; both are read-only in the API and exclude `primary`, `required`, `default` and `validate`.
- `filter: true` makes List accept `?<name>=<value>` (equality); not on the primary key, `float` or `json`, nor on names that clash with `limit`, `offset`, `dir`, `cursor` or the List params; unknown or repeated query parameters are rejected; pair filters and `order` with `index: true`.
- Exactly one `primary` field per entity, typed `string`, `text`, `int32`, `int64`, `uuid` or `references` (to an entity whose key is not itself a reference); no `default`.
- `uuid` keys are generated in Go (v4) with `gen_random_uuid()` as the column default; `int32` and `int64` keys use `IDENTITY`; other keys are required in Create.
- `default` takes a literal of the field's type (`string`, `text`, `int32` in range, `int64`, finite `float`, `bool`) or `now` for `datetime`; the column is `NOT NULL DEFAULT` and the handler fills an omitted value.
- Fields with none of `primary`, `required`, `default`, `generate` are nullable: `*T` in the model, `sql.Null[T]` in the repository.
- Entity and field names must map to Go identifiers (`2fa` does not).
- `references` becomes a column typed from the target's key with a `REFERENCES` constraint; self-references work, reference cycles between entities are rejected.

## License

[MIT](LICENSE) © Pavel Sizikov
