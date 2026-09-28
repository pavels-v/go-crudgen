package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	flagPrefix     = "--"
	specFile       = "api.yaml"
	unknownArg     = "bogus"
	missingSpec    = "missing.yaml"
	releaseVersion = "v1.2.3"
	validSpecSrc   = `package: blog
module: example.com/blog
entities:
  - name: Post
    fields:
      - { name: id, type: uuid, primary: true }
      - { name: title, type: string, required: true }
`
)

func flagArg(name string) string {
	return flagPrefix + name
}

func writeSpec(t *testing.T) string {
	t.Helper()

	p := filepath.Join(t.TempDir(), specFile)
	require.NoError(t, os.WriteFile(p, []byte(validSpecSrc), 0o600))

	return p
}

func TestRun(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{name: "no command", wantCode: exitUsage, wantStderr: usage},
		{name: "help", args: []string{cmdHelp}, wantCode: exitOK, wantStdout: usage},
		{name: "short help", args: []string{cmdHelpShort}, wantCode: exitOK, wantStdout: usage},
		{name: "long help", args: []string{cmdHelpLong}, wantCode: exitOK, wantStdout: usage},
		{name: "version", args: []string{cmdVersion}, wantCode: exitOK, wantStdout: "go-crudgen " + devVersion + "\n"},
		{name: "long version", args: []string{cmdVersionLong}, wantCode: exitOK, wantStdout: "go-crudgen " + devVersion + "\n"},
		{name: "short version", args: []string{cmdVersionShort}, wantCode: exitOK, wantStdout: "go-crudgen " + devVersion + "\n"},
		{name: "unknown command", args: []string{unknownArg}, wantCode: exitUsage, wantStderr: `unknown command "bogus"`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer

			code := Run(tc.args, &stdout, &stderr)
			require.Equal(t, tc.wantCode, code, stderr.String())
			require.Equal(t, tc.wantStdout, stdout.String())
			require.Contains(t, stderr.String(), tc.wantStderr)
		})
	}
}

func TestRunGenerate(t *testing.T) {
	t.Parallel()

	specPath := writeSpec(t)

	cases := []struct {
		name       string
		args       []string
		wantCode   int
		wantStderr string
	}{
		{name: "help", args: []string{cmdHelpShort}, wantCode: exitOK, wantStderr: flagSpec},
		{name: "unknown flag", args: []string{flagArg(unknownArg)}, wantCode: exitUsage, wantStderr: unknownArg},
		{name: "missing spec flag", wantCode: exitUsage, wantStderr: "-spec is required"},
		{
			name:       "main and no-main",
			args:       []string{flagArg(flagSpec), specPath, flagArg(flagMain), t.TempDir(), flagArg(flagNoMain)},
			wantCode:   exitUsage,
			wantStderr: "mutually exclusive",
		},
		{
			name:       "missing spec file",
			args:       []string{flagArg(flagSpec), filepath.Join(t.TempDir(), missingSpec)},
			wantCode:   exitError,
			wantStderr: "failed to load spec",
		},
		{
			name:       "unknown driver",
			args:       []string{flagArg(flagSpec), specPath, flagArg(flagDryRun), flagArg(flagNoMain), flagArg(flagDriver), unknownArg},
			wantCode:   exitError,
			wantStderr: "failed to generate",
		},
		{
			name:     "dry run",
			args:     []string{flagArg(flagSpec), specPath, flagArg(flagOut), t.TempDir(), flagArg(flagDryRun), flagArg(flagNoMain)},
			wantCode: exitOK,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var stdout, stderr bytes.Buffer

			code := Run(append([]string{cmdGenerate}, tc.args...), &stdout, &stderr)
			require.Equal(t, tc.wantCode, code, stderr.String())
			require.Contains(t, stderr.String(), tc.wantStderr)
		})
	}
}

func TestRunGenerate_InvalidSourceDateEpoch(t *testing.T) {
	t.Setenv(envSourceDateEpoch, unknownArg)

	var stdout, stderr bytes.Buffer

	code := Run([]string{cmdGenerate, flagArg(flagSpec), writeSpec(t), flagArg(flagNoMain)}, &stdout, &stderr)
	require.Equal(t, exitUsage, code, stderr.String())
	require.Contains(t, stderr.String(), envSourceDateEpoch)
}

func TestSourceDateEpoch(t *testing.T) {
	cases := []struct {
		name    string
		value   string
		set     bool
		want    time.Time
		wantErr bool
	}{
		{name: "unset"},
		{name: "unix seconds", value: "1767225600", set: true, want: time.Unix(1767225600, 0)},
		{name: "not a number", value: unknownArg, set: true, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set {
				t.Setenv(envSourceDateEpoch, tc.value)
			} else {
				t.Setenv(envSourceDateEpoch, "")
				require.NoError(t, os.Unsetenv(envSourceDateEpoch))
			}

			got, err := sourceDateEpoch()
			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.True(t, tc.want.Equal(got))
		})
	}
}

func TestResolveVersion(t *testing.T) {
	cases := []struct {
		name    string
		version string
		want    string
	}{
		{name: "development build", version: devVersion, want: devVersion},
		{name: "set by ldflags", version: releaseVersion, want: releaseVersion},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prev := version
			version = tc.version

			t.Cleanup(func() { version = prev })

			require.Equal(t, tc.want, resolveVersion())
		})
	}
}
