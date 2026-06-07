# go-crud

A code generator that turns a declarative **entity model specification** into a
fully-working, RESTful Go HTTP service — handlers, routing, data models,
validation, and persistence — so you spend your time on business logic instead
of CRUD boilerplate.

> Status: early design. Nothing is generated yet; this document describes the
> service to be built and the decisions behind it.

## Why

Every backend project re-implements the same layer: an entity, a table, five
HTTP handlers (`Create`, `Read`, `List`, `Update`, `Delete`), request
validation, and serialization. Frameworks in other ecosystems automate this —
Rails scaffolding, Prisma + a REST layer, Django REST Framework, NestJS CRUD.
Go has no single, idiomatic equivalent. `go-crud` aims to be that: describe your
entities once, generate idiomatic Go you own and can edit.

## What it generates

From a single spec describing one or more entities, `go-crud` produces:

- **Models** — Go structs with field tags (JSON, validation, ORM).
- **Storage layer** — repository interfaces plus a concrete implementation
  (initial target: PostgreSQL via SQL/`pgx`; pluggable backends planned).
- **HTTP handlers** — full CRUD per entity following REST conventions.
- **Router wiring** — routes registered on a standard `net/http` mux.
- **Request/response DTOs** — with validation derived from the spec.
- **Migrations** — SQL schema for each entity (planned).
- **OpenAPI document** — generated from the same spec for client tooling (planned).

### REST surface per entity

| Method   | Path             | Action          |
| -------- | ---------------- | --------------- |
| `POST`   | `/{plural}`      | Create          |
| `GET`    | `/{plural}`      | List (paginated)|
| `GET`    | `/{plural}/{id}` | Read one        |
| `PUT`    | `/{plural}/{id}` | Update          |
| `DELETE` | `/{plural}/{id}` | Delete          |

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
  APIs; Go tools like `oapi-codegen` and Ent's `elk` already generate from it.
  Portable, but verbose and API-shaped rather than entity-shaped.

**Decision: the primary format is YAML.** It is declarative, diff-friendly,
checked into the repo, needs no custom parser, and reads naturally to anyone
who has touched OpenAPI, Prisma, or Kubernetes manifests. A Rails-style
one-line shorthand is offered as a convenience for quick scaffolding, and
OpenAPI export is on the roadmap for interop.

### Example: YAML spec

```yaml
version: 1
package: blog            # Go package name for generated code
module: example.com/blog # import path

entities:
  - name: Post
    plural: posts         # optional; defaults to a naive pluralization
    fields:
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
      - name: email
        type: string
        required: true
        unique: true
        validate: "email"
      - name: name
        type: string
```

### Example: Rails-style shorthand (convenience)

```bash
go-crud scaffold Post title:string body:text published:bool author:references
```

This expands to the equivalent YAML before generation.

### Supported field types (initial)

`string`, `text`, `int`, `int64`, `float`, `decimal`, `bool`, `date`,
`datetime`, `uuid`, `json`, and `references` (relations). Per-field modifiers:
`required`, `unique`, `default`, `index`, and `validate` (validation tag rules).

## Planned usage

```bash
# install
go install example.com/go-crud/cmd/go-crud@latest

# generate a service from a spec
go-crud generate --spec ./api.yaml --out ./internal/api

# quick scaffold via shorthand
go-crud scaffold Post title:string body:text published:bool
```

## Design principles

- **Idiomatic, readable Go** — output looks hand-written, not magic.
- **You own the code** — generate once into your repo; edit freely. No runtime
  framework lock-in.
- **Standard library first** — `net/http` routing, minimal dependencies.
- **Spec is the source of truth** — re-running the generator is safe and
  predictable.
- **Pluggable storage** — repository interfaces keep the DB swappable.

## Roadmap

- [ ] YAML spec parser and validation
- [ ] Model + struct-tag generation
- [ ] CRUD handler generation (`net/http`)
- [ ] PostgreSQL repository + migrations
- [ ] List endpoint: pagination, filtering, sorting
- [ ] Rails-style shorthand → YAML expansion
- [ ] Relations (`belongs_to` / `has_many`)
- [ ] OpenAPI 3 document export
- [ ] Pluggable storage backends (SQLite, in-memory)
- [ ] Auth/middleware hooks

## References

- [Rails Scaffold Generator Guide](https://rails.devcamp.com/trails/learn-ruby-on-rails-from-scratch/campsites/building-your-first-rails-application/guides/rails-scaffold-generator-guide)
- [Prisma schema documentation](https://www.prisma.io/docs/orm/prisma-schema/overview)
- [OpenAPI code generation in Go with oapi-codegen](https://dev.to/nikita_rykhlov/go-tools-code-generation-from-openapi-specs-in-go-with-oapi-codegen-3jc1)
- [Generate a Go CRUD HTTP API with Ent + elk](https://entgo.io/blog/2021/07/29/generate-a-fully-working-go-crud-http-api-with-ent/)
