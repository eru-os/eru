package derived

import (
	"errors"
	"fmt"
	"strings"
)

// FuncSpec is one entry in the closed function registry. Nothing outside this
// map can be called, which is what makes emission injection-proof.
type FuncSpec struct {
	MinArgs     int
	MaxArgs     int // -1 for variadic
	ArgTypes    []string
	Result      string
	ResultFn    func(argTypes []string) (string, error)
	IsAggregate bool
	Emit        EmitFunc
}

// argType returns the expected value type for position i. The last entry
// repeats for variadic functions.
func (f FuncSpec) argType(i int) string {
	if len(f.ArgTypes) == 0 {
		return TypeAny
	}
	if i < len(f.ArgTypes) {
		return f.ArgTypes[i]
	}
	return f.ArgTypes[len(f.ArgTypes)-1]
}

const TypeAny = "any"

var validUnits = map[string]string{
	"day": "day", "days": "day",
	"month": "month", "months": "month",
	"year": "year", "years": "year",
}

func simple(sqlName string) EmitFunc {
	return func(d *Dialect, args []string, argTypes []string) (string, error) {
		return fmt.Sprint(sqlName, "(", strings.Join(args, ", "), ")"), nil
	}
}

func aggregate(sqlName string) EmitFunc {
	return func(d *Dialect, args []string, argTypes []string) (string, error) {
		return fmt.Sprint(sqlName, "(", args[0], ")"), nil
	}
}

func widestOf(positions ...int) func(argTypes []string) (string, error) {
	return func(argTypes []string) (string, error) {
		result := TypeNull
		for _, i := range positions {
			if i >= len(argTypes) {
				continue
			}
			widened, err := widen(result, argTypes[i])
			if err != nil {
				return "", err
			}
			result = widened
		}
		if result == TypeNull {
			return TypeNull, nil
		}
		return result, nil
	}
}

func widestOfAll(argTypes []string) (string, error) {
	result := TypeNull
	for _, t := range argTypes {
		widened, err := widen(result, t)
		if err != nil {
			return "", err
		}
		result = widened
	}
	return result, nil
}

var catalog map[string]FuncSpec

func init() {
	catalog = map[string]FuncSpec{
		"ABS":   {MinArgs: 1, MaxArgs: 1, ArgTypes: []string{TypeNumber}, Result: TypeNumber, Emit: simple("ABS")},
		"FLOOR": {MinArgs: 1, MaxArgs: 1, ArgTypes: []string{TypeNumber}, Result: TypeNumber, Emit: simple("FLOOR")},
		"CEIL":  {MinArgs: 1, MaxArgs: 1, ArgTypes: []string{TypeNumber}, Result: TypeNumber, Emit: simple("CEIL")},
		"SQRT":  {MinArgs: 1, MaxArgs: 1, ArgTypes: []string{TypeNumber}, Result: TypeNumber, Emit: simple("SQRT")},
		"POWER": {MinArgs: 2, MaxArgs: 2, ArgTypes: []string{TypeNumber, TypeNumber}, Result: TypeNumber, Emit: simple("POWER")},
		"MOD":   {MinArgs: 2, MaxArgs: 2, ArgTypes: []string{TypeNumber, TypeNumber}, Result: TypeNumber, Emit: simple("MOD")},

		"ROUND": {MinArgs: 1, MaxArgs: 2, ArgTypes: []string{TypeNumber, TypeNumber}, Result: TypeNumber,
			Emit: func(d *Dialect, args []string, argTypes []string) (string, error) {
				if len(args) == 1 {
					return fmt.Sprint("ROUND(", d.NumericCast(args[0]), ")"), nil
				}
				return fmt.Sprint("ROUND(", d.NumericCast(args[0]), ", ", args[1], ")"), nil
			}},

		"IF": {MinArgs: 3, MaxArgs: 3, ArgTypes: []string{TypeBoolean, TypeAny, TypeAny}, ResultFn: widestOf(1, 2),
			Emit: func(d *Dialect, args []string, argTypes []string) (string, error) {
				return fmt.Sprint("CASE WHEN ", args[0], " THEN ", args[1], " ELSE ", args[2], " END"), nil
			}},

		"CASE": {MinArgs: 3, MaxArgs: -1, ResultFn: caseResultType,
			Emit: func(d *Dialect, args []string, argTypes []string) (string, error) {
				if len(args)%2 == 0 {
					return "", errors.New("CASE needs alternating condition and value arguments followed by one default")
				}
				var sb strings.Builder
				sb.WriteString("CASE")
				for i := 0; i+1 < len(args); i = i + 2 {
					sb.WriteString(fmt.Sprint(" WHEN ", args[i], " THEN ", args[i+1]))
				}
				sb.WriteString(fmt.Sprint(" ELSE ", args[len(args)-1], " END"))
				return sb.String(), nil
			}},

		"COALESCE": {MinArgs: 2, MaxArgs: -1, ResultFn: widestOfAll, Emit: simple("COALESCE")},
		"NULLIF":   {MinArgs: 2, MaxArgs: 2, ResultFn: widestOf(0), Emit: simple("NULLIF")},

		"ISNULL": {MinArgs: 1, MaxArgs: 1, Result: TypeBoolean,
			Emit: func(d *Dialect, args []string, argTypes []string) (string, error) {
				return fmt.Sprint("(", args[0], ") IS NULL"), nil
			}},

		"SUM":   {MinArgs: 1, MaxArgs: 1, ArgTypes: []string{TypeNumber}, Result: TypeNumber, IsAggregate: true, Emit: aggregate("SUM")},
		"COUNT": {MinArgs: 1, MaxArgs: 1, ArgTypes: []string{TypeAny}, Result: TypeNumber, IsAggregate: true, Emit: aggregate("COUNT")},
		"MIN":   {MinArgs: 1, MaxArgs: 1, ResultFn: widestOf(0), IsAggregate: true, Emit: aggregate("MIN")},
		"MAX":   {MinArgs: 1, MaxArgs: 1, ResultFn: widestOf(0), IsAggregate: true, Emit: aggregate("MAX")},
		"AVG": {MinArgs: 1, MaxArgs: 1, ArgTypes: []string{TypeNumber}, Result: TypeNumber, IsAggregate: true,
			Emit: func(d *Dialect, args []string, argTypes []string) (string, error) {
				return fmt.Sprint("AVG(", d.NumericCast(args[0]), ")"), nil
			}},

		"CONCAT": {MinArgs: 2, MaxArgs: -1, ArgTypes: []string{TypeAny}, Result: TypeText, Emit: simple("CONCAT")},
		"UPPER":  {MinArgs: 1, MaxArgs: 1, ArgTypes: []string{TypeText}, Result: TypeText, Emit: simple("UPPER")},
		"LOWER":  {MinArgs: 1, MaxArgs: 1, ArgTypes: []string{TypeText}, Result: TypeText, Emit: simple("LOWER")},
		"TRIM":   {MinArgs: 1, MaxArgs: 1, ArgTypes: []string{TypeText}, Result: TypeText, Emit: simple("TRIM")},
		"LEN":    {MinArgs: 1, MaxArgs: 1, ArgTypes: []string{TypeText}, Result: TypeNumber, Emit: simple("LENGTH")},
		"LEFT":   {MinArgs: 2, MaxArgs: 2, ArgTypes: []string{TypeText, TypeNumber}, Result: TypeText, Emit: simple("LEFT")},
		"RIGHT":  {MinArgs: 2, MaxArgs: 2, ArgTypes: []string{TypeText, TypeNumber}, Result: TypeText, Emit: simple("RIGHT")},

		"SUBSTR": {MinArgs: 2, MaxArgs: 3, ArgTypes: []string{TypeText, TypeNumber, TypeNumber}, Result: TypeText,
			Emit: func(d *Dialect, args []string, argTypes []string) (string, error) {
				if len(args) == 2 {
					return fmt.Sprint("SUBSTRING(", args[0], " FROM ", args[1], ")"), nil
				}
				return fmt.Sprint("SUBSTRING(", args[0], " FROM ", args[1], " FOR ", args[2], ")"), nil
			}},

		"TODAY": {MinArgs: 0, MaxArgs: 0, Result: TypeDate,
			Emit: func(d *Dialect, args []string, argTypes []string) (string, error) {
				return "CURRENT_DATE", nil
			}},
		"NOW": {MinArgs: 0, MaxArgs: 0, Result: TypeDate,
			Emit: func(d *Dialect, args []string, argTypes []string) (string, error) {
				return "NOW()", nil
			}},

		"DATEDIFF": {MinArgs: 2, MaxArgs: 3, ArgTypes: []string{TypeDate, TypeDate, TypeText}, Result: TypeNumber, Emit: emitDateDiff},
		"DATEADD":  {MinArgs: 3, MaxArgs: 3, ArgTypes: []string{TypeDate, TypeNumber, TypeText}, Result: TypeDate, Emit: emitDateAdd},

		"YEAR":  {MinArgs: 1, MaxArgs: 1, ArgTypes: []string{TypeDate}, Result: TypeNumber, Emit: emitExtract("YEAR")},
		"MONTH": {MinArgs: 1, MaxArgs: 1, ArgTypes: []string{TypeDate}, Result: TypeNumber, Emit: emitExtract("MONTH")},
		"DAY":   {MinArgs: 1, MaxArgs: 1, ArgTypes: []string{TypeDate}, Result: TypeNumber, Emit: emitExtract("DAY")},
	}
}

func caseResultType(argTypes []string) (string, error) {
	if len(argTypes)%2 == 0 {
		return "", errors.New("CASE needs alternating condition and value arguments followed by one default")
	}
	var values []string
	for i := 1; i < len(argTypes); i = i + 2 {
		values = append(values, argTypes[i])
	}
	values = append(values, argTypes[len(argTypes)-1])
	return widestOfAll(values)
}

func emitExtract(part string) EmitFunc {
	return func(d *Dialect, args []string, argTypes []string) (string, error) {
		return fmt.Sprint("EXTRACT(", part, " FROM ", args[0], ")"), nil
	}
}

func emitDateDiff(d *Dialect, args []string, argTypes []string) (string, error) {
	unit := "day"
	if len(args) == 3 {
		u, err := unitFromLiteral(args[2])
		if err != nil {
			return "", err
		}
		unit = u
	}
	switch unit {
	case "day":
		return fmt.Sprint("((", args[0], ")::date - (", args[1], ")::date)"), nil
	case "month":
		return fmt.Sprint("((EXTRACT(YEAR FROM ", args[0], ") * 12 + EXTRACT(MONTH FROM ", args[0],
			")) - (EXTRACT(YEAR FROM ", args[1], ") * 12 + EXTRACT(MONTH FROM ", args[1], ")))"), nil
	default:
		return fmt.Sprint("(EXTRACT(YEAR FROM ", args[0], ") - EXTRACT(YEAR FROM ", args[1], "))"), nil
	}
}

func emitDateAdd(d *Dialect, args []string, argTypes []string) (string, error) {
	unit, err := unitFromLiteral(args[2])
	if err != nil {
		return "", err
	}
	return fmt.Sprint("(", args[0], " + ((", args[1], ") || ' ", unit, "')::interval)"), nil
}

// unitFromLiteral reads the unit back out of the already-emitted literal. The
// unit reaches SQL as a keyword in most dialects, so it is only ever one of
// three values from this map - never the caller's text.
func unitFromLiteral(emitted string) (string, error) {
	raw := strings.Trim(emitted, "'")
	unit, ok := validUnits[strings.ToLower(strings.TrimSpace(raw))]
	if !ok {
		return "", fmt.Errorf("unit %s is not supported - use 'day', 'month' or 'year'", emitted)
	}
	return unit, nil
}
