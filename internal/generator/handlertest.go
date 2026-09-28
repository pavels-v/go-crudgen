package generator

import (
	"encoding/json/jsontext"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"

	"github.com/pavels-v/go-crudgen/internal/spec"
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
	exprNew         = "new(%s)"
	exprSeeded      = "seeded.%s"
	exprUUIDString  = "%s.String()"
	exprFormatInt   = "strconv.FormatInt(%s, 10)"
	exprNextInt     = "strconv.FormatInt(%s+1, 10)"
	exprToInt64     = "int64(%s)"
	exprMissingKey  = `"missing"`
	exprMissingID   = "uuid.NewString()"
	exprInt32       = "int32(%d)"
	exprInt64       = "int64(%d)"
	exprNewUUID     = "uuid.New()"
	exprDecimal     = "decimal.NewFromInt(1)"
	exprDate        = "domain.Date(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))"
	exprDatetime    = "time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)"
	exprJSON        = "jsontext.Value(`{}`)"
	sampleFiller    = "a"
	ruleOneOf       = "oneof"
	floatStep       = 0.5
	samplesPerParam = 3
	maxSampleLen    = 1024
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

		picked, ok := pickSample(v, samples(stored, f.Validate), f.Validate)
		if !ok {
			return nil, fmt.Errorf("entity %q field %q: no test sample passes validate %q: adjust the rule or pass --no-tests", e.Name, f.Name, f.Validate)
		}

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
	nums, words := ruleParams(validate)

	switch fieldType {
	case spec.TypeString, spec.TypeText:
		return stringSamples(nums, words)
	case spec.TypeInt32:
		return intSamples(nums, func(n int64) sample {
			return sample{expr: fmt.Sprintf(exprInt32, n), value: int32(n)}
		})
	case spec.TypeInt64:
		return intSamples(nums, func(n int64) sample {
			return sample{expr: fmt.Sprintf(exprInt64, n), value: n}
		})
	case spec.TypeFloat:
		return floatSamples(nums)
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

func ruleParams(validate string) (nums []float64, words []string) {
	for rule := range strings.SplitSeq(validate, ruleSep) {
		for alt := range strings.SplitSeq(rule, ruleOr) {
			name, param, ok := strings.Cut(alt, ruleParamSep)
			if !ok {
				continue
			}

			values := []string{param}
			if name == ruleOneOf {
				values = strings.Fields(param)
				words = append(words, values...)
			}

			for _, v := range values {
				if n, err := strconv.ParseFloat(v, 64); err == nil {
					nums = append(nums, n)
				}
			}
		}
	}

	return nums, words
}

func stringSamples(nums []float64, words []string) []sample {
	values := []string{"sample", "user@example.com", "https://example.com", "+14155552671", sampleFiller}
	values = append(values, words...)

	for _, n := range nums {
		if n > 0 && n <= maxSampleLen && n == float64(int(n)) {
			values = append(values, strings.Repeat(sampleFiller, int(n)))
		}
	}

	out := make([]sample, len(values))
	for i, s := range values {
		out[i] = sample{expr: strconv.Quote(s), value: s}
	}

	return out
}

func intSamples(nums []float64, mk func(int64) sample) []sample {
	values := make([]int64, 0, samplesPerParam*(len(nums)+1))
	values = append(values, 1, 0, 100)

	for _, n := range nums {
		values = append(values, int64(n), int64(n)+1, int64(n)-1)
	}

	out := make([]sample, len(values))
	for i, n := range values {
		out[i] = mk(n)
	}

	return out
}

func floatSamples(nums []float64) []sample {
	values := make([]float64, 0, samplesPerParam*(len(nums)+1))
	values = append(values, 1.5, 0.5, 100.5)

	for _, n := range nums {
		values = append(values, n, n+floatStep, n-floatStep)
	}

	out := make([]sample, len(values))
	for i, n := range values {
		out[i] = sample{expr: formatFloat(n), value: n}
	}

	return out
}

func pickSample(v *validator.Validate, candidates []sample, validate string) (sample, bool) {
	for _, c := range candidates {
		if validate == "" || c.value == nil || passes(v, c.value, validate) {
			return c, true
		}
	}

	return sample{}, false
}

func passes(v *validator.Validate, value any, tag string) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()

	return v.Var(value, tag) == nil
}
