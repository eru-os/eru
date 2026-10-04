package derived

import (
	"fmt"
	"strings"
)

// MysqlDialect is the postgres baseline plus the constructs that genuinely
// differ. Everything not listed here - the arithmetic and comparison operators,
// CASE, COALESCE, NULLIF, ABS, FLOOR, POWER, SQRT, MOD, UPPER, LOWER, TRIM,
// CONCAT, LEFT, RIGHT and the aggregates - is identical across all four.
func MysqlDialect() Dialect {
	return Dialect{
		Name:           "mysql",
		QuoteIdentFn:   func(ident string) string { return fmt.Sprint("`", strings.ReplaceAll(ident, "`", "``"), "`") },
		NumericCastFmt: "CAST(%s AS DECIMAL(38,10))",
		HopStrategy:    HopCorrelated,
		Overrides: map[string]EmitFunc{
			"LEN":    simple("CHAR_LENGTH"),
			"SUBSTR": positionalSubstr,
			"TODAY":  constant("CURDATE()"),
			"NOW":    constant("NOW()"),
			"DATEDIFF": unitEmitter(func(d *Dialect, args []string, unit string) string {
				return fmt.Sprint("TIMESTAMPDIFF(", strings.ToUpper(unit), ", ", args[1], ", ", args[0], ")")
			}, 2),
			"DATEADD": unitEmitter(func(d *Dialect, args []string, unit string) string {
				return fmt.Sprint("DATE_ADD(", args[0], ", INTERVAL ", args[1], " ", strings.ToUpper(unit), ")")
			}, 2),
			"YEAR":  simple("YEAR"),
			"MONTH": simple("MONTH"),
			"DAY":   simple("DAY"),
		},
	}
}

// MssqlDialect. MSSQL has no boolean type, so a boolean-typed top-level
// expression has to be wrapped as a value - see BoolNeedsWrap.
func MssqlDialect() Dialect {
	return Dialect{
		Name:           "mssql",
		QuoteIdentFn:   func(ident string) string { return fmt.Sprint("[", strings.ReplaceAll(ident, "]", "]]"), "]") },
		NumericCastFmt: "CAST(%s AS DECIMAL(38,10))",
		BoolNeedsWrap:  true,
		HopStrategy:    HopCorrelated,
		Overrides: map[string]EmitFunc{
			"CEIL": simple("CEILING"),
			"LEN":  simple("LEN"),
			"MOD": func(d *Dialect, args []string, argTypes []string) (string, error) {
				return fmt.Sprint("(", args[0], " % ", args[1], ")"), nil
			},
			"ROUND": func(d *Dialect, args []string, argTypes []string) (string, error) {
				decimals := "0"
				if len(args) == 2 {
					decimals = args[1]
				}
				return fmt.Sprint("ROUND(", d.NumericCast(args[0]), ", ", decimals, ")"), nil
			},
			"SUBSTR": func(d *Dialect, args []string, argTypes []string) (string, error) {
				// MSSQL has no two-argument form
				length := fmt.Sprint("LEN(", args[0], ")")
				if len(args) == 3 {
					length = args[2]
				}
				return fmt.Sprint("SUBSTRING(", args[0], ", ", args[1], ", ", length, ")"), nil
			},
			"TODAY": constant("CAST(GETDATE() AS date)"),
			"NOW":   constant("SYSDATETIME()"),
			"DATEDIFF": unitEmitter(func(d *Dialect, args []string, unit string) string {
				return fmt.Sprint("DATEDIFF(", unit, ", ", args[1], ", ", args[0], ")")
			}, 2),
			"DATEADD": unitEmitter(func(d *Dialect, args []string, unit string) string {
				return fmt.Sprint("DATEADD(", unit, ", ", args[1], ", ", args[0], ")")
			}, 2),
			"YEAR":  simple("YEAR"),
			"MONTH": simple("MONTH"),
			"DAY":   simple("DAY"),
		},
	}
}

// IcebergDialect covers Trino/Presto SQL, which has no LEFT or RIGHT.
func IcebergDialect() Dialect {
	return Dialect{
		Name:           "iceberg",
		NumericCastFmt: "CAST(%s AS DECIMAL(38,10))",
		HopStrategy:    HopCorrelated,
		Overrides: map[string]EmitFunc{
			"LEN":    simple("LENGTH"),
			"SUBSTR": simple("SUBSTR"),
			"LEFT": func(d *Dialect, args []string, argTypes []string) (string, error) {
				return fmt.Sprint("SUBSTR(", args[0], ", 1, ", args[1], ")"), nil
			},
			"RIGHT": func(d *Dialect, args []string, argTypes []string) (string, error) {
				return fmt.Sprint("SUBSTR(", args[0], ", -(", args[1], "))"), nil
			},
			"TODAY": constant("CURRENT_DATE"),
			"NOW":   constant("CURRENT_TIMESTAMP"),
			"DATEDIFF": unitEmitter(func(d *Dialect, args []string, unit string) string {
				return fmt.Sprint("DATE_DIFF('", unit, "', ", args[1], ", ", args[0], ")")
			}, 2),
			"DATEADD": unitEmitter(func(d *Dialect, args []string, unit string) string {
				// Trino needs a literal interval, so scale a one-unit interval
				return fmt.Sprint("(", args[0], " + (", args[1], ") * INTERVAL '1' ", strings.ToUpper(unit), ")")
			}, 2),
			"YEAR":  simple("YEAR"),
			"MONTH": simple("MONTH"),
			"DAY":   simple("DAY"),
		},
	}
}

func constant(sql string) EmitFunc {
	return func(d *Dialect, args []string, argTypes []string) (string, error) {
		return sql, nil
	}
}

// positionalSubstr is the SUBSTRING(x, s, l) form, as opposed to the
// SUBSTRING(x FROM s FOR l) form postgres uses.
func positionalSubstr(d *Dialect, args []string, argTypes []string) (string, error) {
	return fmt.Sprint("SUBSTRING(", strings.Join(args, ", "), ")"), nil
}

// unitEmitter reads the unit out of the emitted literal at position unitArg -
// always one of three keywords from the catalogue's map, never caller text - and
// hands it to the dialect's own shape.
func unitEmitter(emit func(d *Dialect, args []string, unit string) string, unitArg int) EmitFunc {
	return func(d *Dialect, args []string, argTypes []string) (string, error) {
		unit := "day"
		if len(args) > unitArg {
			u, err := unitFromLiteral(args[unitArg])
			if err != nil {
				return "", err
			}
			unit = u
		}
		return emit(d, args, unit), nil
	}
}
