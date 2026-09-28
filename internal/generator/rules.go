package generator

import (
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"

	"go-crudgen/internal/spec"
)

func checkRules(e *spec.Entity, byName map[string]*spec.Entity) error {
	v := validator.New()
	for _, f := range e.Fields {
		if f.Validate == "" {
			continue
		}

		gt, err := fieldType(f, byName)
		if err != nil {
			return fmt.Errorf("entity %q field %q: %w", e.Name, f.Name, err)
		}

		tags := []string{f.Validate}
		for rule := range strings.SplitSeq(f.Validate, ruleSep) {
			tags = append(tags, strings.Split(rule, ruleOr)...)
		}

		for _, tag := range tags {
			if err := tryRule(v, gt.sample, tag); err != nil {
				return fmt.Errorf("entity %q field %q has invalid validate %q: %w", e.Name, f.Name, f.Validate, err)
			}
		}
	}

	return nil
}

func tryRule(v *validator.Validate, sample any, tag string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()

	_ = v.Var(sample, tag)

	return nil
}
