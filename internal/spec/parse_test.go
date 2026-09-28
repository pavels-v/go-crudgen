package spec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const validYAML = `package: blog
module: example.com/blog
entities:
  - name: Author
    fields:
      - name: id
        type: uuid
        primary: true
      - name: created_at
        type: datetime
        generate: on_create
`

func TestLoad(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		yaml    string
		wantErr bool
	}{
		{"valid", validYAML, false},
		{"empty file", "", true},
		{"unknown top-level key", validYAML + "modul: x\n", true},
		{"unknown field key", `package: blog
module: example.com/blog
entities:
  - name: Author
    fields:
      - name: id
        type: uuid
        primary: true
        optional: true
`, true},
		{"removed options section", `package: blog
module: example.com/blog
entities:
  - name: Author
    fields:
      - name: id
        type: uuid
        primary: true
    options:
      timestamps: true
`, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "spec.yaml")
			require.NoError(t, os.WriteFile(path, []byte(tc.yaml), 0o600))

			_, err := Load(path)
			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
		})
	}
}
