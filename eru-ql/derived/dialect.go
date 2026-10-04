package derived

import (
	"fmt"
	"strings"
)

const (
	HopCorrelated = "correlated"
	HopJoined     = "joined"
)

// EmitFunc renders one call. args are already-emitted SQL fragments, argTypes
// their inferred value types.
type EmitFunc func(d *Dialect, args []string, argTypes []string) (string, error)

// Dialect is a set of small emitters and format strings. Its zero value is
// postgres-shaped, so a maker only overrides its deltas.
type Dialect struct {
	Name           string
	QuoteIdentFn   func(string) string
	QuoteStringFn  func(string) string
	NumericCastFmt string
	BoolNeedsWrap  bool
	HopStrategy    string
	Overrides      map[string]EmitFunc
}

func (d *Dialect) name() string {
	if d.Name == "" {
		return "postgres"
	}
	return d.Name
}

func (d *Dialect) QuoteIdent(ident string) string {
	if d.QuoteIdentFn != nil {
		return d.QuoteIdentFn(ident)
	}
	return fmt.Sprint("\"", strings.ReplaceAll(ident, "\"", "\"\""), "\"")
}

// QuoteString emits a text literal as a typed constant. No path in the emitter
// concatenates user text into SQL without going through here.
func (d *Dialect) QuoteString(s string) string {
	if d.QuoteStringFn != nil {
		return d.QuoteStringFn(s)
	}
	return fmt.Sprint("'", strings.ReplaceAll(s, "'", "''"), "'")
}

func (d *Dialect) NumericCast(expr string) string {
	fmtStr := d.NumericCastFmt
	if fmtStr == "" {
		fmtStr = "(%s)::numeric"
	}
	return fmt.Sprintf(fmtStr, expr)
}

// hopStrategy reports how a cross-table hop is emitted. Only HopCorrelated is
// implemented; the field exists so the joined shape can be added later without
// reworking the emitter, and Compile refuses any other value in the meantime.
func (d *Dialect) hopStrategy() string {
	if d.HopStrategy == "" {
		return HopCorrelated
	}
	return d.HopStrategy
}

func (d *Dialect) emitter(name string) EmitFunc {
	if d.Overrides != nil {
		if fn, ok := d.Overrides[name]; ok {
			return fn
		}
	}
	if spec, ok := catalog[name]; ok {
		return spec.Emit
	}
	return nil
}

// PostgresDialect is the baseline. Every other dialect is this plus overrides.
func PostgresDialect() Dialect {
	return Dialect{Name: "postgres", HopStrategy: HopCorrelated}
}
