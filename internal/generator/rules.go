package generator

import (
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"

	"github.com/pavels-v/go-crudgen/internal/spec"
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

		stored, err := storedType(f, byName)
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

		for _, rule := range tags[1:] {
			name, _, _ := strings.Cut(rule, ruleParamSep)
			if !ruleApplies(stored, name) {
				return fmt.Errorf("entity %q field %q has validate rule %q, which does not apply to %s fields", e.Name, f.Name, rule, stored)
			}
		}
	}

	return nil
}

func ruleApplies(fieldType, rule string) bool {
	if isPresenceRule(rule) {
		return true
	}

	switch fieldType {
	case spec.TypeDate, spec.TypeDatetime, spec.TypeDecimal:
		return false
	case spec.TypeUUID:
		return strings.HasPrefix(rule, ruleUUID)
	}

	return true
}

func isPresenceRule(rule string) bool {
	switch rule {
	case ruleOmitEmpty, ruleOmitNil, ruleOmitZero:
		return true
	}

	return strings.HasPrefix(rule, ruleRequired) || strings.HasPrefix(rule, ruleExcluded)
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
