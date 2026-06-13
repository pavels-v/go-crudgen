# go-crudgen

A code generator that turns a declarative **entity model specification** into a
fully-working, RESTful Go HTTP service — handlers, routing, data models,
validation, and persistence — so you spend your time on business logic instead
of CRUD boilerplate.

> Status: in progress. Models, DTOs, CRUD handlers, `net/http` router wiring,
> PostgreSQL repositories (via `sqlx`/`database/sql`), and goose SQL migrations
> are generated today; OpenAPI export is still on the roadmap.

## Why

Every backend project re-implements the same layer: an entity, a table, five
HTTP handlers (`Create`, `Read`, `List`, `Update`, `Delete`), request
validation, and serialization. Frameworks in other ecosystems automate this —
Rails scaffolding, Django REST Framework, and in JavaScript:

- **[NestJS](https://docs.nestjs.com/recipes/crud-generator)** —
  `nest generate resource <name>` scaffolds a module, a controller (or a
  resolver for GraphQL), a service, DTOs, and an entity with CRUD stubs as
  editable code — the same generate-and-own model `go-crudgen` uses.

Go has no single, idiomatic equivalent. `go-crudgen` aims to be that: describe your
entities once, generate idiomatic Go you own and can edit.

## What it generates

From a single spec describing one or more entities, `go-crudgen` produces:

- **Models** — Go structs with field tags (JSON, validation).
- **Request/response DTOs** — `Create`/`Update` bodies with validation derived
  from the spec.
- **HTTP handlers** — full CRUD per entity following REST conventions.
- **Router wiring** — routes registered on a standard `net/http` mux.
- **Repository interfaces + a PostgreSQL implementation** — the storage seam
  each handler depends on, plus a generated `sqlx`-backed repository that
  satisfies it. The repository talks to `database/sql`, so it binds to no
  specific driver.
- **Database connection constructor** — a `NewDB` helper that opens the pool
  through your chosen driver (`pgx` by default, `pq` via `--driver`), with that
  driver blank-imported for you.
- **Migrations** — a [goose](https://github.com/pressly/goose) SQL migration per
  entity (`migrations/NNNNN_create_<table>.sql`) with `-- +goose Up`/`Down`
  sections, written for every entity (including composite-key ones that get no
  handler).

Still planned:

- **OpenAPI document** — generated from the same spec for client tooling.

### REST surface per entity

| Method   | Path             | Action           |
| -------- | ---------------- | ---------------- |
| `POST`   | `/{plural}`      | Create           |
| `GET`    | `/{plural}`      | List (paginated) |
| `GET`    | `/{plural}/{id}` | Read one         |
| `PUT`    | `/{plural}/{id}` | Update           |
| `DELETE` | `/{plural}/{id}` | Delete           |

## Specification format

The spec format is the heart of the tool. We surveyed common approaches:

- **Rails** uses a terse CLI shorthand — `rails g scaffold Post title:string
  body:text published:boolean post:references`. Fast to type, but it lives in
  shell history rather than a checked-in, reviewable file, and struggles to
  express rich constraints.
- **Prisma** uses a dedicated schema DSL with `model` blocks and field
  attributes (`@id`, `@unique`, `@default`, `@relation`). Very expressive, but
  requires learning (and us writing) a custom parser.
- **OpenAPI** uses YAML/JSON and is the industry standard for describing REST
  APIs; the Go tool `oapi-codegen` generates server and client code directly
  from an OpenAPI document. Portable, but verbose and API-shaped rather than
  entity-shaped.

**Decision: the format is YAML.** It is declarative, diff-friendly,
checked into the repo, needs no custom parser, and reads naturally to anyone
who has touched OpenAPI, Prisma, or Kubernetes manifests. OpenAPI export is on
the roadmap for interop.

### Example: YAML spec

```yaml
version: 1
package: blog            # Go package name for generated code
module: example.com/blog # import path

entities:
  - name: Post
    plural: posts         # optional; defaults to a naive pluralization
    fields:
      - name: id
        type: uuid
        primary: true     # every entity needs at least one primary field
      - name: title
        type: string
        required: true
        validate: "min=1,max=200"
      - name: body
        type: text
      - name: published
        type: bool
        default: false
      - name: author
        type: references   # belongs-to relation
        target: Author
    options:
      timestamps: true     # adds created_at / updated_at
      soft_delete: false

  - name: Author
    fields:
      - name: id
        type: uuid
        primary: true
      - name: email
        type: string
        required: true
        unique: true
        validate: "email"
      - name: name
        type: string
```

### Supported field types (initial)

`string`, `text`, `int`, `int64`, `float`, `decimal`, `bool`, `date`,
`datetime`, `uuid`, `json`, and `references` (relations). Per-field modifiers:
`primary` (marks a primary-key field; entities may have a composite key),
`required`, `unique`, `default`, `index`, and `validate` (validation tag rules).

A field that is neither `primary` nor `required` is **nullable**. Nullable fields
become pointers (`*T`, with `json:",omitempty"`) in the model and request DTOs,
and `sql.Null[T]` columns in the generated repository's row type, so a SQL `NULL`
round-trips as a `nil` pointer rather than a zero value.

> Note: entities with a composite primary key currently generate a model but no
> HTTP handlers (a single `/{id}` path can't address a composite key yet).

## Planned usage

```bash
# build the binary (named go-crudgen) from the CLI entrypoint
go build -o go-crudgen ./cmd/cli

# preview generated code on stdout (default when --out is omitted)
go-crudgen generate --spec ./api.yaml

# generate a service from a spec into a directory
go-crudgen generate --spec ./api.yaml --out ./internal/api

# the generated code imports third-party packages (sqlx, the driver, uuid, ...),
# so resolve them in the output module afterwards
cd ./internal/api && go mod tidy
```

By default the generator emits a `NewDB` constructor wired to the **pgx**
driver. Pass `--driver pq` to use `lib/pq` instead:

```bash
go-crudgen generate --spec ./api.yaml --out ./internal/api --driver pq
```

The driver only affects the generated `NewDB` constructor; the repositories
themselves stay driver-agnostic (they depend on `*sqlx.DB`, not a driver).
Re-run `go mod tidy` in the output module after switching drivers so the new
driver dependency is fetched. `NewDB` applies sane connection-pool defaults
(max-open/idle conns, conn lifetime) you can tune in the generated file.

### Wiring the generated service

Handlers depend on the repository *interface*; the generated Postgres
repository implements it. The generated `NewDB` opens the pool (the chosen
driver is blank-imported for you); you inject the repositories into the router:

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

## Design principles

- **Idiomatic, readable Go** — output looks hand-written, not magic.
- **You own the code** — generate once into your repo; edit freely. No runtime
  framework lock-in.
- **Standard library first** — `net/http` routing, minimal dependencies.
- **Spec is the source of truth** — re-running the generator is safe and
  predictable.
- **Pluggable storage** — repository interfaces keep the DB swappable, and the
  generated Postgres repository targets `database/sql`, so any driver works.

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
  - [x] `belongs_to`: `references` fields generate a foreign-key column typed
    from the target's primary key
  - [ ] `has_many`, nested/relation routes, and JOIN-based loading
- [ ] OpenAPI 3 document export
- [ ] Pluggable storage backends (SQLite, in-memory)
- [ ] Auth/middleware hooks

## References

- [Rails Scaffold Generator Guide](https://rails.devcamp.com/trails/learn-ruby-on-rails-from-scratch/campsites/building-your-first-rails-application/guides/rails-scaffold-generator-guide)
- [NestJS CRUD generator](https://docs.nestjs.com/recipes/crud-generator)
- [Prisma schema documentation](https://www.prisma.io/docs/orm/prisma-schema/overview)
- [OpenAPI code generation in Go with oapi-codegen](https://dev.to/nikita_rykhlov/go-tools-code-generation-from-openapi-specs-in-go-with-oapi-codegen-3jc1)
- [Generate a Go CRUD HTTP API with Ent + elk](https://entgo.io/blog/2021/07/29/generate-a-fully-working-go-crudgen-http-api-with-ent/)

## License

[MIT](LICENSE) © Pavel Sizikov
