package valobj

import "strings"

type ErrInvalidField struct {
	value string
}

func (e *ErrInvalidField) Error() string {
	return "invalid field '" + e.value + "', expected Name:Type[:tag]"
}

// Field represents a single struct field on a generated type
type Field struct {
	Name Naming `json:"name"`
	Type string `json:"type"`
	Tag  string `json:"tag,omitempty"`
}

// ParseField builds a Field from a Name:Type[:tag] declaration
func ParseField(value string) (Field, error) {
	// cut twice rather than splitting: only the first two segments are read
	// separately, and the tag is whatever is left joined back together
	name, rest, ok := strings.Cut(value, ":")
	if !ok {
		return Field{}, &ErrInvalidField{value}
	}

	fieldType, tag, tagged := strings.Cut(rest, ":")

	name = strings.TrimSpace(name)
	fieldType = strings.TrimSpace(fieldType)

	if len(name) == 0 || len(fieldType) == 0 {
		return Field{}, &ErrInvalidField{value}
	}

	field := Field{
		Name: NewNaming(name),
		Type: fieldType,
	}

	if tagged {
		field.Tag = strings.TrimSpace(tag)
	}

	return field, nil
}

// ParseFields builds Fields from a slice of Name:Type[:tag] declarations
func ParseFields(values []string) ([]Field, error) {
	fields := make([]Field, 0, len(values))

	for i := range values {
		field, err := ParseField(values[i])
		if err != nil {
			return nil, err
		}

		fields = append(fields, field)
	}

	return fields, nil
}
