package spec

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Load reads, parses, and validates a spec from a YAML file.
func Load(path string) (*Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading spec: %w", err)
	}

	var s Spec
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing spec %s: %w", path, err)
	}

	if err := s.Validate(); err != nil {
		return nil, fmt.Errorf("invalid spec %s: %w", path, err)
	}
	return &s, nil
}
