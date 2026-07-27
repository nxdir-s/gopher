package valobj

import (
	"strings"
	"unicode"
)

// initialisms are words that stay fully capitalized in Go identifiers
var initialisms = map[string]struct{}{
	"ACL":   {},
	"API":   {},
	"ASCII": {},
	"CPU":   {},
	"CSS":   {},
	"DB":    {},
	"DNS":   {},
	"EOF":   {},
	"GRPC":  {},
	"HTML":  {},
	"HTTP":  {},
	"HTTPS": {},
	"ID":    {},
	"IP":    {},
	"JSON":  {},
	"RPC":   {},
	"SLA":   {},
	"SQL":   {},
	"SSH":   {},
	"TCP":   {},
	"TLS":   {},
	"TTL":   {},
	"UDP":   {},
	"UI":    {},
	"URI":   {},
	"URL":   {},
	"UTF8":  {},
	"UUID":  {},
	"XML":   {},
}

// Naming holds the case variations derived from a single input name
type Naming struct {
	Pascal string `json:"pascal"`
	Camel  string `json:"camel"`
	Snake  string `json:"snake"`
	Kebab  string `json:"kebab"`
	Lower  string `json:"lower"`
	Upper  string `json:"upper"`
	Plural string `json:"plural"`
	Words  string `json:"words"`
}

// NewNaming splits the supplied name into words and derives its case variations
func NewNaming(name string) Naming {
	words := splitWords(name)
	lower := lowerWords(words)
	pascal := pascalCase(words)

	return Naming{
		Pascal: pascal,
		Camel:  camelCase(words),
		Snake:  strings.Join(lower, "_"),
		Kebab:  strings.Join(lower, "-"),
		Lower:  strings.Join(lower, ""),
		Upper:  strings.ToUpper(strings.Join(lower, "_")),
		Plural: plural(pascal),
		Words:  strings.Join(lower, " "),
	}
}

// String returns the pascal case variation
func (n Naming) String() string {
	return n.Pascal
}

// splitWords breaks a name on separators, case transitions, and acronym boundaries
func splitWords(name string) []string {
	runes := []rune(name)
	words := make([]string, 0, 4)
	current := make([]rune, 0, len(runes))

	for i := range runes {
		if !unicode.IsLetter(runes[i]) && !unicode.IsDigit(runes[i]) {
			if len(current) > 0 {
				words = append(words, string(current))
				current = current[:0]
			}

			continue
		}

		if unicode.IsUpper(runes[i]) && len(current) > 0 {
			prev := current[len(current)-1]

			var next rune
			if i+1 < len(runes) {
				next = runes[i+1]
			}

			if !unicode.IsUpper(prev) || unicode.IsLower(next) {
				words = append(words, string(current))
				current = current[:0]
			}
		}

		current = append(current, runes[i])
	}

	if len(current) > 0 {
		words = append(words, string(current))
	}

	return words
}

// lowerWords lowercases every word
func lowerWords(words []string) []string {
	lower := make([]string, 0, len(words))
	for i := range words {
		lower = append(lower, strings.ToLower(words[i]))
	}

	return lower
}

// pascalCase joins the words with each one capitalized
func pascalCase(words []string) string {
	var b strings.Builder

	for i := range words {
		b.WriteString(capitalize(words[i]))
	}

	return b.String()
}

// camelCase joins the words with everything but the first one capitalized
func camelCase(words []string) string {
	if len(words) == 0 {
		return ""
	}

	var b strings.Builder

	b.WriteString(strings.ToLower(words[0]))

	for i := 1; i < len(words); i++ {
		b.WriteString(capitalize(words[i]))
	}

	return b.String()
}

// capitalize uppercases the first rune, keeping known initialisms fully capitalized
func capitalize(word string) string {
	if len(word) == 0 {
		return word
	}

	upper := strings.ToUpper(word)
	if _, ok := initialisms[upper]; ok {
		return upper
	}

	runes := []rune(strings.ToLower(word))
	runes[0] = unicode.ToUpper(runes[0])

	return string(runes)
}

// plural returns the plural form of the supplied word
func plural(word string) string {
	if len(word) == 0 {
		return word
	}

	lower := strings.ToLower(word)

	switch {
	case strings.HasSuffix(lower, "s"),
		strings.HasSuffix(lower, "x"),
		strings.HasSuffix(lower, "z"),
		strings.HasSuffix(lower, "ch"),
		strings.HasSuffix(lower, "sh"):
		return word + "es"
	case strings.HasSuffix(lower, "y") && len(word) > 1 && !isVowel(rune(lower[len(lower)-2])):
		return word[:len(word)-1] + "ies"
	default:
		return word + "s"
	}
}

// isVowel reports whether the supplied rune is an english vowel
func isVowel(r rune) bool {
	switch r {
	case 'a', 'e', 'i', 'o', 'u':
		return true
	default:
		return false
	}
}
