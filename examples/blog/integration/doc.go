// Package integration holds Docker-backed integration tests that drive the
// generated example service (example.com/blog) against a real PostgreSQL
// instance via testcontainers, applying the generated goose migrations.
//
// It is a separate Go module so the heavy testcontainers/goose dependency tree
// stays out of the example's go.mod. Run it with a Docker daemon available:
//
//	go test -count=1 -v ./...
package integration
