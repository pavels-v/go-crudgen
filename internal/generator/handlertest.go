package generator

import (
	"encoding/json/jsontext"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"

	"go-crudgen/internal/spec"
)

const (
	tmplHandlerTest = "handler_test.go.tmpl"
	tmplFakeTest    = "fake_test.go.tmpl"
	fileHandlerTest = pkgREST + "/%s_test.go"
	fileFakeTest    = pkgREST + "/fake_test.go"
)

const (
	importTesting  = "testing"
	importRequire  = "github.com/stretchr/testify/require"
	importHTTPTest = "net/http/httptest"
	importSync     = "sync"
)

const (
	exprNew        = "new(%s)"
	exprSeeded     = "seeded.%s"
	exprUUIDString = "%s.String()"
	exprFormatInt  = "strconv.FormatInt(%s, 10)"
	exprNextInt    = "strconv.FormatInt(%s+1, 10)"
	exprToInt64    = "int64(%s)"
	exprMissingKey = `"missing"`
	exprMissingID  = "uuid.NewString()"
	exprInt32      = "int32(%d)"
	exprInt64      = "int64(%d)"
	exprNewUUID    = "uuid.New()"
	exprDecimal    = "decimal.NewFromInt(1)"
	exprDate       = "domain.Date(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))"
	exprDatetime   = "time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)"
	exprJSON       = "jsontext.Value(`{}`)"
	sampleFiller   = "a"
	maxSampleLen   = 1024
)

type handlerTestData struct {
	Package       string
	Imports       []string
	Struct        string
	Model         string
	ListParams    string
	Plural        string
	KeyType       string
	PKGoName      string
	GenInt        bool
	GenUUID       bool
	ParsedKey     bool
	HasRequired   bool
	SeedKey       string
	SeededID      string
	MissingID     string
	CreateFixture []assign
	UpdateFixture []assign
}

type sample struct {
	expr  string
	value any
	imp   string
}

func renderFakeTest(s *spec.Spec) ([]byte, error) {
	imports := map[string]struct{}{
		importContext:  {},
		importNetHTTP:  {},
		importHTTPTest: {},
		importStrings:  {},
		importSync:     {},
		importTesting:  {},
		s.Module:       {},
	}

	return renderTemplate(tmplFakeTest, restData{Package: pkgREST, Imports: groupImports(imports, s.Module)})
}

func renderHandlerTest(s *spec.Spec, e *spec.Entity, byName map[string]*spec.Entity, v *validator.Validate) ([]byte, error) {
	pk, gt, err := serveKey(e, byName)
	if err != nil {
		return nil, err
	}

	name := pascalCase(e.Name)
	pkGoName := pascalCase(pk.Name)
	seeded := fmt.Sprintf(exprSeeded, pkGoName)
	_, generated := keyGenerator(pk)

	data := handlerTestData{
		Package:    pkgREST,
		Struct:     name,
		Model:      qualified(name),
		ListParams: qualified(fmt.Sprintf(nameListParams, name)),
		Plural:     plural(e.Name, e.Plural),
		KeyType:    gt.outside(s).expr,
		PKGoName:   pkGoName,
		GenInt:     generated && gt.expr != goUUID,
		GenUUID:    generated && gt.expr == goUUID,
		ParsedKey:  gt.expr != goString,
	}

	imports := map[string]struct{}{
		importJSONv2:  {},
		importJSON:    {},
		importNetHTTP: {},
		importTesting: {},
		importRequire: {},
		s.Module:      {},
	}

	switch gt.expr {
	case goUUID:
		data.SeededID = fmt.Sprintf(exprUUIDString, seeded)
		data.MissingID = exprMissingID
		imports[importUUID] = struct{}{}
	case goString:
		data.SeededID = seeded
		data.MissingID = exprMissingKey
	default:
		id := seeded
		if gt.expr == goInt32 {
			id = fmt.Sprintf(exprToInt64, seeded)
		}

		data.SeededID = fmt.Sprintf(exprFormatInt, id)
		data.MissingID = fmt.Sprintf(exprNextInt, id)
		imports[importStrconv] = struct{}{}
	}

	if !generated {
		keyType, err := storedType(pk, byName)
		if err != nil {
			return nil, err
		}

		data.SeedKey = samples(keyType, "")[0].expr
	}

	for _, f := range e.Fields {
		if f.Generate != "" || (f.Primary && generated) || (!f.Required && !f.Primary) {
			continue
		}

		stored, err := storedType(f, byName)
		if err != nil {
			return nil, err
		}

		ft, err := fieldType(f, byName)
		if err != nil {
			return nil, err
		}

		picked := pickSample(v, samples(stored, f.Validate), f.Validate)
		if picked.imp != "" {
			imports[picked.imp] = struct{}{}
		}

		req := f
		req.Required = true

		expr := picked.expr
		if requiresPresence(req, ft) {
			expr = fmt.Sprintf(exprNew, expr)
		}

		a := assign{Field: pascalCase(f.Name), Expr: expr}
		data.CreateFixture = append(data.CreateFixture, a)

		if !f.Primary {
			data.UpdateFixture = append(data.UpdateFixture, a)
		}
	}

	data.HasRequired = len(data.CreateFixture) > 0
	data.Imports = groupImports(imports, s.Module)

	return renderTemplate(tmplHandlerTest, data)
}

func samples(fieldType, validate string) []sample {
	switch fieldType {
	case spec.TypeString, spec.TypeText:
		return stringSamples(validate)
	case spec.TypeInt32:
		return []sample{
			{expr: fmt.Sprintf(exprInt32, 1), value: int32(1)},
			{expr: fmt.Sprintf(exprInt32, 0), value: int32(0)},
			{expr: fmt.Sprintf(exprInt32, 100), value: int32(100)},
		}
	case spec.TypeInt64:
		return []sample{
			{expr: fmt.Sprintf(exprInt64, 1), value: int64(1)},
			{expr: fmt.Sprintf(exprInt64, 0), value: int64(0)},
			{expr: fmt.Sprintf(exprInt64, 100), value: int64(100)},
		}
	case spec.TypeFloat:
		return []sample{
			{expr: formatFloat(1.5), value: 1.5},
			{expr: formatFloat(0.5), value: 0.5},
			{expr: formatFloat(100.5), value: 100.5},
		}
	case spec.TypeBool:
		return []sample{{expr: strconv.FormatBool(true), value: true}, {expr: strconv.FormatBool(false), value: false}}
	case spec.TypeDecimal:
		return []sample{{expr: exprDecimal, imp: importDecimal}}
	case spec.TypeDate:
		return []sample{{expr: exprDate, imp: importTime}}
	case spec.TypeDatetime:
		return []sample{{expr: exprDatetime, imp: importTime}}
	case spec.TypeUUID:
		return []sample{{expr: exprNewUUID, imp: importUUID}}
	case spec.TypeJSON:
		return []sample{{expr: exprJSON, value: jsontext.Value("{}"), imp: importJSON}}
	}

	return nil
}

func stringSamples(validate string) []sample {
	values := []string{"sample", "user@example.com", "https://example.com", sampleFiller}

	for rule := range strings.SplitSeq(validate, ruleSep) {
		for alt := range strings.SplitSeq(rule, ruleOr) {
			_, param, ok := strings.Cut(alt, ruleParamSep)
			if !ok {
				continue
			}

			n, err := strconv.Atoi(param)
			if err == nil && n > 0 && n <= maxSampleLen {
				values = append(values, strings.Repeat(sampleFiller, n))
			}
		}
	}

	out := make([]sample, len(values))
	for i, s := range values {
		out[i] = sample{expr: strconv.Quote(s), value: s}
	}

	return out
}

func pickSample(v *validator.Validate, candidates []sample, validate string) sample {
	for _, c := range candidates {
		if validate == "" || c.value == nil || passes(v, c.value, validate) {
			return c
		}
	}

	return candidates[0]
}

func passes(v *validator.Validate, value any, tag string) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()

	return v.Var(value, tag) == nil
}
