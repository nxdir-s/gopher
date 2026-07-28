package valobj

import "testing"

// BenchmarkNewNaming measures the derivation every naming aware template func
// performs. The pascal, camel, snake and plural funcs each rebuild the whole
// struct, so a template that names a type four ways pays this four times
func BenchmarkNewNaming(b *testing.B) {
	names := map[string]string{
		"word":       "Order",
		"words":      "payment gateway",
		"initialism": "http cache",
	}

	for label, name := range names {
		b.Run(label, func(b *testing.B) {
			for b.Loop() {
				NewNaming(name)
			}
		})
	}
}

// BenchmarkParseFields measures the field list parsing templateData runs twice,
// once for -field and once for -port
func BenchmarkParseFields(b *testing.B) {
	lists := map[string][]string{
		"one": {"ID:int"},
		"three": {
			"ID:int",
			"Total:float64",
			"Status:string:json:\"status\"",
		},
	}

	for label, values := range lists {
		b.Run(label, func(b *testing.B) {
			for b.Loop() {
				if _, err := ParseFields(values); err != nil {
					b.Fatalf("failed to parse fields: %s", err.Error())
				}
			}
		})
	}
}
