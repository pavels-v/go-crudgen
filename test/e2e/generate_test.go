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
// blog example from examples/blog.yaml into examples/blog, then builds and tests
// that (separate) module to prove the generated code compiles and behaves.
//
// It regenerates in place, so the committed examples stay the source of truth;
// CI can run `git diff --exit-code examples/blog` afterwards to catch a stale
// commit where the generator output drifted from what's checked in.
func TestGenerateBlogExample(t *testing.T) {
	root := repoRoot(t)
	blogDir := filepath.Join(root, "examples", "blog")

	// Regenerate the example with the default (pgx) driver.
	run(t, root, "go", "run", "./cmd/go-crudgen", "generate",
		"--spec", "examples/blog.yaml", "--out", "examples/blog")

	for _, f := range []string{
		"post.gen.go", "post_handler.gen.go", "post_repo.gen.go",
		"author.gen.go", "author_handler.gen.go", "author_repo.gen.go",
		"http.gen.go", "db.gen.go", "nulls.gen.go",
		filepath.Join("migrations", "00001_create_authors.sql"),
		filepath.Join("migrations", "00002_create_posts.sql"),
	} {
		require.FileExists(t, filepath.Join(blogDir, f), "expected generated file")
	}

	// Prove the freshly generated module compiles and its handler tests pass.
	run(t, blogDir, "go", "test", "./...")
}
