// Package e2e contains end-to-end tests that drive the go-crudgen binary
// against the committed example spec. The tests are guarded by the "e2e" build
// tag so they stay out of the default `go test ./...` run; invoke them with:
//
//	make e2e
//	# or: go test -tags e2e ./test/e2e
package e2e
