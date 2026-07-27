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
	parts := strings.Split(value, ":")
	if len(parts) < 2 {
		return Field{}, &ErrInvalidField{value}
	}

	name := strings.TrimSpace(parts[0])
	fieldType := strings.TrimSpace(parts[1])

	if len(name) == 0 || len(fieldType) == 0 {
		return Field{}, &ErrInvalidField{value}
	}

	field := Field{
		Name: NewNaming(name),
		Type: fieldType,
	}

	if len(parts) > 2 {
		field.Tag = strings.TrimSpace(strings.Join(parts[2:], ":"))
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
