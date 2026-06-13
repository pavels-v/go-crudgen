package generator

import "strings"

// initialisms get fully upper-cased in Go identifiers for idiomatic output.
var initialisms = map[string]bool{
	"id":   true,
	"uuid": true,
	"url":  true,
	"api":  true,
	"http": true,
	"json": true,
	"sql":  true,
}

// splitWords breaks an identifier on underscores, hyphens, and spaces.
func splitWords(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == '-' || r == ' '
	})
}

// pascalCase converts a spec name (e.g. "created_at") to an exported Go
// identifier (e.g. "CreatedAt"), upper-casing known initialisms.
func pascalCase(s string) string {
	var b strings.Builder
	for _, w := range splitWords(s) {
		if initialisms[strings.ToLower(w)] {
			b.WriteString(strings.ToUpper(w))
			continue
		}
		b.WriteString(strings.ToUpper(w[:1]))
		b.WriteString(w[1:])
	}
	return b.String()
}

// unexport lower-cases the first letter of an identifier, e.g. "BlogPost" ->
// "blogPost", for naming package-internal types like the repository row struct.
func unexport(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

// plural returns the route path segment for an entity: the explicit override
// when set, otherwise a naive pluralization of the snake_cased name.
func plural(name, override string) string {
	if strings.TrimSpace(override) != "" {
		return override
	}
	return pluralize(snakeCase(name))
}

// pluralize applies naive English rules to a lower-case word. Irregulars are
// out of scope; the spec's `plural` override is the escape hatch.
func pluralize(s string) string {
	switch {
	case strings.HasSuffix(s, "s"), strings.HasSuffix(s, "x"), strings.HasSuffix(s, "z"),
		strings.HasSuffix(s, "ch"), strings.HasSuffix(s, "sh"):
		return s + "es"
	case strings.HasSuffix(s, "y") && len(s) >= 2 && !isVowel(s[len(s)-2]):
		return s[:len(s)-1] + "ies"
	default:
		return s + "s"
	}
}

func isVowel(b byte) bool {
	switch b {
	case 'a', 'e', 'i', 'o', 'u':
		return true
	}
	return false
}

// snakeCase lower-cases and joins an identifier's words with underscores; used
// for generated file names (e.g. "BlogPost" -> "blog_post").
func snakeCase(s string) string {
	var b strings.Builder
	prevLower := false
	for i, r := range s {
		switch {
		case r == '_' || r == '-' || r == ' ':
			b.WriteByte('_')
			prevLower = false
		case r >= 'A' && r <= 'Z':
			if i > 0 && prevLower {
				b.WriteByte('_')
			}
			b.WriteRune(r - 'A' + 'a')
			prevLower = false
		default:
			b.WriteRune(r)
			prevLower = r >= 'a' && r <= 'z'
		}
	}
	return b.String()
}
