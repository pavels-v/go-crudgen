package spec

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

// Load reads, parses, and validates a spec from a YAML file.
func Load(path string) (*Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read spec: %w", err)
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var s Spec
	err = dec.Decode(&s)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parse spec %s: %w", path, err)
	}

	if err := s.Validate(); err != nil {
		return nil, fmt.Errorf("validate spec %s: %w", path, err)
	}
	return &s, nil
}
