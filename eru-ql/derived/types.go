package derived

import (
	"fmt"
	"strings"
)

// ValueTypeOf maps a declared type onto one of the four value types the formula
// language works in. It accepts both database column types and the
// presentation-level types a consumer is more likely to send, so a caller only
// ever has to state the type it already knows.
func ValueTypeOf(dbDataType string) string {
	t := strings.ToLower(strings.TrimSpace(dbDataType))
	if i := strings.IndexAny(t, "( "); i > 0 {
		t = t[:i]
	}
	switch t {
	case "smallint", "integer", "int", "int2", "int4", "int8", "bigint", "decimal", "numeric",
		"real", "double", "float", "float4", "float8", "money", "serial", "bigserial",
		"number", "currency", "percentage", "percent", "rating", "progress":
		return TypeNumber
	case "date", "timestamp", "timestamptz", "datetime", "datetime2", "time", "timetz",
		"smalldatetime", "timestamp with time zone", "timestamp without time zone":
		return TypeDate
	case "boolean", "bool", "bit", "checkbox":
		return TypeBoolean
	default:
		return TypeText
	}
}

// widen combines two value types. NULL takes on the other side's type; anything
// else has to agree. Strictness here is deliberate - a silent number/text mix is
// how a formula starts returning plausible nonsense.
func widen(a string, b string) (string, error) {
	if a == TypeNull || a == "" {
		return b, nil
	}
	if b == TypeNull || b == "" {
		return a, nil
	}
	if a == TypeAny {
		return b, nil
	}
	if b == TypeAny {
		return a, nil
	}
	if a == b {
		return a, nil
	}
	return "", fmt.Errorf("cannot combine %s and %s in one expression", a, b)
}

// acceptsType reports whether a value of type actual can be passed where want
// is expected.
func acceptsType(want string, actual string) bool {
	if want == TypeAny || want == "" || actual == TypeNull || actual == TypeAny {
		return true
	}
	return want == actual
}
