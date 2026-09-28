//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

const exampleEpoch = "1767225600"

func clean(t *testing.T, dir string) {
	t.Helper()

	for _, pattern := range []string{"*.go", "restapi/*.go", "postgres/*.go"} {
		matches, err := filepath.Glob(filepath.Join(dir, filepath.FromSlash(pattern)))
		require.NoError(t, err, "glob %s", pattern)

		for _, m := range matches {
			if strings.HasSuffix(m, "_test.go") {
				continue
			}

			require.NoError(t, os.Remove(m), "remove %s", m)
		}
	}

	require.NoError(t, os.RemoveAll(filepath.Join(dir, "migrations")), "remove migrations")
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

	clean(t, outDir)
	t.Setenv("SOURCE_DATE_EPOCH", exampleEpoch)

	// Regenerate the example with the default (pgx) driver and NewRouter.
	run(t, root, "go", "run", "./cmd/go-crudgen", "generate",
		"--spec", "examples/blog.yaml", "--out", "examples/blogservice/internal/blog", "--router")

	for _, f := range []string{
		"post.go", "author.go", "comment.go", "tag.go",
		"errors.go", "date.go", "sort.go",
		filepath.Join("restapi", "post.go"),
		filepath.Join("restapi", "author.go"),
		filepath.Join("restapi", "comment.go"),
		filepath.Join("restapi", "tag.go"),
		filepath.Join("restapi", "request.go"),
		filepath.Join("restapi", "response.go"),
		filepath.Join("restapi", "query.go"),
		filepath.Join("restapi", "routes.go"),
		filepath.Join("restapi", "router.go"),
		filepath.Join("postgres", "post.go"),
		filepath.Join("postgres", "author.go"),
		filepath.Join("postgres", "comment.go"),
		filepath.Join("postgres", "tag.go"),
		filepath.Join("postgres", "db.go"),
		filepath.Join("postgres", "nulls.go"),
		filepath.Join("migrations", "20260101000000_create_authors.sql"),
		filepath.Join("migrations", "20260101000001_create_posts.sql"),
		filepath.Join("migrations", "20260101000002_create_comments.sql"),
		filepath.Join("migrations", "20260101000003_create_tags.sql"),
	} {
		require.FileExists(t, filepath.Join(outDir, f), "expected generated file")
	}

	again := exec.Command("go", "run", "./cmd/go-crudgen", "generate",
		"--spec", "examples/blog.yaml", "--out", "examples/blogservice/internal/blog", "--router")
	again.Dir = root
	out, err := again.CombinedOutput()
	require.Errorf(t, err, "second generation should fail:\n%s", out)
	require.Contains(t, string(out), "already has generated code")

	// Prove the freshly generated module compiles and its handler tests pass.
	run(t, blogDir, "go", "test", "./...")
}
