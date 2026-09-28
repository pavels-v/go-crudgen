//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// repoRoot returns the module root (two levels up from this test package) and
// verifies a go.mod is there, so a wrong working directory fails loudly.
func repoRoot(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	require.NoError(t, err, "getwd")
	root, err := filepath.Abs(filepath.Join(wd, "..", ".."))
	require.NoError(t, err, "resolve repo root")
	require.FileExists(t, filepath.Join(root, "go.mod"), "go.mod at repo root")

	return root
}

// run executes name+args in dir, failing the test with the combined output when
// the command exits non-zero.
func run(t *testing.T, dir, name string, args ...string) {
	t.Helper()

	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "%s %v in %s failed:\n%s", name, args, dir, out)
	t.Logf("%s %v:\n%s", name, args, out)
}

// TestGenerateBlogExample drives the real CLI end to end: it regenerates the
// blog example from examples/blog.yaml into examples/blogservice/internal/blog,
// then builds and tests that (separate) module to prove the generated code
// compiles and behaves.
//
// It regenerates in place, so the committed examples stay the source of truth;
// `make verify-examples` checks git status afterwards to catch a stale
// commit where the generator output drifted from what's checked in.
func TestGenerateBlogExample(t *testing.T) {
	root := repoRoot(t)
	blogDir := filepath.Join(root, "examples", "blogservice")
	outDir := filepath.Join(blogDir, "internal", "blog")

	// Regenerate the example with the default (pgx) driver and NewRouter.
	run(t, root, "go", "run", "./cmd/go-crudgen", "generate",
		"--spec", "examples/blog.yaml", "--out", "examples/blogservice/internal/blog", "--router")

	for _, f := range []string{
		"post.gen.go", "author.gen.go", "comment.gen.go", "tag.gen.go",
		"errors.gen.go", "date.gen.go", "sort.gen.go",
		filepath.Join("restapi", "post.gen.go"),
		filepath.Join("restapi", "author.gen.go"),
		filepath.Join("restapi", "comment.gen.go"),
		filepath.Join("restapi", "tag.gen.go"),
		filepath.Join("restapi", "request.gen.go"),
		filepath.Join("restapi", "response.gen.go"),
		filepath.Join("restapi", "query.gen.go"),
		filepath.Join("restapi", "routes.gen.go"),
		filepath.Join("restapi", "router.gen.go"),
		filepath.Join("postgres", "post.gen.go"),
		filepath.Join("postgres", "author.gen.go"),
		filepath.Join("postgres", "comment.gen.go"),
		filepath.Join("postgres", "tag.gen.go"),
		filepath.Join("postgres", "db.gen.go"),
		filepath.Join("postgres", "nulls.gen.go"),
		filepath.Join("migrations", "00001_create_authors.sql"),
		filepath.Join("migrations", "00002_create_posts.sql"),
		filepath.Join("migrations", "00003_create_comments.sql"),
		filepath.Join("migrations", "00004_create_tags.sql"),
	} {
		require.FileExists(t, filepath.Join(outDir, f), "expected generated file")
	}

	// Prove the freshly generated module compiles and its handler tests pass.
	run(t, blogDir, "go", "test", "./...")
}
