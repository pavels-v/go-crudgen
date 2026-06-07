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
