package derived

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	precOr         = 1
	precAnd        = 2
	precNot        = 3
	precComparison = 4
	precAdditive   = 5
	precMultiply   = 6
	precUnary      = 7
	precPrimary    = 8
)

type compiler struct {
	schema       Schema
	dialect      Dialect
	table        string
	field        string
	outerAlias   string
	allowHops    bool
	security     SecuredSource
	validateOnly bool

	deps      map[string]bool
	refTables map[string]bool
	warnings  []string
	aggDepth  int
	hasAgg    bool
	hopSeq    int
	hopCtx    *hopContext
	refs      map[string]bool
	inlining  []string
	maxInline int
	hops      []Hop
	aggHops   int
}

type emitted struct {
	sql     string
	valType string
	prec    int
}

// wrapped parenthesises a child expression when its precedence is lower than
// the parent's. parenAtSamePrec says whether an equal-precedence child also
// needs parentheses: false for the left operand of a left-associative operator,
// true for the right operand, where a - (b - c) differs from (a - b) - c.
func (e emitted) wrapped(parentPrec int, parenAtSamePrec bool) string {
	if e.prec > parentPrec {
		return e.sql
	}
	if e.prec == parentPrec && !parenAtSamePrec {
		return e.sql
	}
	return fmt.Sprint("(", e.sql, ")")
}

func (c *compiler) warn(msg string) {
	for _, w := range c.warnings {
		if w == msg {
			return
		}
	}
	c.warnings = append(c.warnings, msg)
}

func (c *compiler) emit(n *Node) (emitted, error) {
	if n == nil {
		return emitted{}, errors.New("empty expression")
	}
	switch n.Kind {
	case NodeNumber:
		v, ok := numberValue(n)
		if !ok {
			return emitted{}, errors.New("malformed number literal")
		}
		return emitted{sql: strconv.FormatFloat(v, 'f', -1, 64), valType: TypeNumber, prec: precPrimary}, nil
	case NodeString:
		s, ok := stringValue(n)
		if !ok {
			return emitted{}, errors.New("malformed text literal")
		}
		return emitted{sql: c.dialect.QuoteString(s), valType: TypeText, prec: precPrimary}, nil
	case NodeBoolean:
		b, _ := n.Value.(bool)
		if b {
			return emitted{sql: "TRUE", valType: TypeBoolean, prec: precPrimary}, nil
		}
		return emitted{sql: "FALSE", valType: TypeBoolean, prec: precPrimary}, nil
	case NodeNull:
		return emitted{sql: "NULL", valType: TypeNull, prec: precPrimary}, nil
	case NodeField:
		return c.emitField(n)
	case NodeUnary:
		return c.emitUnary(n)
	case NodeBinary:
		return c.emitBinary(n)
	case NodeFunc:
		return c.emitCall(n)
	}
	return emitted{}, fmt.Errorf("unsupported node kind %q", n.Kind)
}

func (c *compiler) emitField(n *Node) (emitted, error) {
	ref, err := c.resolveRef(n)
	if err != nil {
		return emitted{}, err
	}
	if ref.hop != nil && c.hopCtx != nil && c.hopCtx.table == ref.hop.TargetTable {
		// inside the subquery this hop built - reference the alias directly
		return emitted{
			sql:     fmt.Sprint(c.dialect.QuoteIdent(c.hopCtx.alias), ".", c.dialect.QuoteIdent(ref.column)),
			valType: ref.valType,
			prec:    precPrimary,
		}, nil
	}
	if ref.derivedRef != "" {
		return c.emitDerivedRef(ref.derivedRef)
	}
	if ref.hop == nil {
		c.deps[ref.column] = true
		alias := c.outerAlias
		if alias == "" {
			alias = unqualify(c.table)
		}
		return emitted{
			sql:     fmt.Sprint(c.dialect.QuoteIdent(alias), ".", c.dialect.QuoteIdent(ref.column)),
			valType: ref.valType,
			prec:    precPrimary,
		}, nil
	}
	return c.emitHopRef(ref)
}

func (c *compiler) emitUnary(n *Node) (emitted, error) {
	operand, err := c.emit(n.Operand)
	if err != nil {
		return emitted{}, err
	}
	switch n.Op {
	case "-":
		if !acceptsType(TypeNumber, operand.valType) {
			return emitted{}, fmt.Errorf("cannot negate a %s value", operand.valType)
		}
		return emitted{sql: fmt.Sprint("-", operand.wrapped(precUnary, true)), valType: TypeNumber, prec: precUnary}, nil
	case "NOT":
		if !acceptsType(TypeBoolean, operand.valType) {
			return emitted{}, fmt.Errorf("NOT needs a true/false value, got %s", operand.valType)
		}
		return emitted{sql: fmt.Sprint("NOT ", operand.wrapped(precNot, true)), valType: TypeBoolean, prec: precNot}, nil
	}
	return emitted{}, fmt.Errorf("unsupported unary operator %q", n.Op)
}

func (c *compiler) emitBinary(n *Node) (emitted, error) {
	left, err := c.emit(n.Left)
	if err != nil {
		return emitted{}, err
	}
	right, err := c.emit(n.Right)
	if err != nil {
		return emitted{}, err
	}

	switch n.Op {
	case "AND", "OR":
		prec := precAnd
		if n.Op == "OR" {
			prec = precOr
		}
		if !acceptsType(TypeBoolean, left.valType) || !acceptsType(TypeBoolean, right.valType) {
			return emitted{}, fmt.Errorf("%s needs true/false values on both sides", n.Op)
		}
		return emitted{
			sql:     fmt.Sprint(left.wrapped(prec, false), " ", n.Op, " ", right.wrapped(prec, true)),
			valType: TypeBoolean,
			prec:    prec,
		}, nil

	case "=", "!=", "<", "<=", ">", ">=":
		if _, werr := widen(left.valType, right.valType); werr != nil {
			return emitted{}, fmt.Errorf("cannot compare %s with %s", left.valType, right.valType)
		}
		return emitted{
			sql:     fmt.Sprint(left.wrapped(precComparison, false), " ", n.Op, " ", right.wrapped(precComparison, true)),
			valType: TypeBoolean,
			prec:    precComparison,
		}, nil

	case "+", "-", "*", "/", "%":
		return c.emitArithmetic(n, left, right)
	}
	return emitted{}, fmt.Errorf("unsupported operator %q", n.Op)
}

func (c *compiler) emitArithmetic(n *Node, left emitted, right emitted) (emitted, error) {
	prec := precAdditive
	if n.Op == "*" || n.Op == "/" || n.Op == "%" {
		prec = precMultiply
	}

	if n.Op == "+" && (left.valType == TypeText || right.valType == TypeText) {
		return emitted{}, errors.New("+ does not join text - use CONCAT(a, b)")
	}
	if n.Op == "-" && left.valType == TypeDate && right.valType == TypeDate {
		c.warn("subtracting two dates returns a number of days - use DATEDIFF(a, b, 'day') to state the unit")
		return emitted{
			sql:     fmt.Sprint(left.wrapped(prec, false), " - ", right.wrapped(prec, true)),
			valType: TypeNumber,
			prec:    prec,
		}, nil
	}
	if !acceptsType(TypeNumber, left.valType) || !acceptsType(TypeNumber, right.valType) {
		return emitted{}, fmt.Errorf("%s needs numbers on both sides, got %s and %s", n.Op, left.valType, right.valType)
	}
	if n.Op == "/" || n.Op == "%" {
		if v, ok := numberValue(n.Right); ok && v == 0 {
			return emitted{}, errors.New("division by zero")
		}
		if n.Right.Kind != NodeNumber && n.Right.Name != "NULLIF" {
			c.warn("an unguarded divisor breaks on any row where it is 0 or NULL - wrap it in NULLIF(divisor, 0)")
		}
	}
	return emitted{
		sql:     fmt.Sprint(left.wrapped(prec, false), " ", n.Op, " ", right.wrapped(prec, true)),
		valType: TypeNumber,
		prec:    prec,
	}, nil
}

func (c *compiler) emitCall(n *Node) (emitted, error) {
	spec, ok := catalog[n.Name]
	if !ok {
		return emitted{}, fmt.Errorf("%s is not a known function", n.Name)
	}
	if err := checkArity(n.Name, spec, len(n.Args)); err != nil {
		return emitted{}, err
	}
	if spec.IsAggregate {
		return c.emitAggregate(n, spec)
	}

	args, argTypes, err := c.emitArgs(n, spec)
	if err != nil {
		return emitted{}, err
	}
	return c.finishCall(n, spec, args, argTypes, "")
}

func (c *compiler) emitArgs(n *Node, spec FuncSpec) (args []string, argTypes []string, err error) {
	args = make([]string, 0, len(n.Args))
	argTypes = make([]string, 0, len(n.Args))
	for i, argNode := range n.Args {
		arg, aerr := c.emit(argNode)
		if aerr != nil {
			return nil, nil, aerr
		}
		want := spec.argType(i)
		if !acceptsType(want, arg.valType) {
			return nil, nil, fmt.Errorf("%s argument %d needs a %s value, got %s", n.Name, i+1, want, arg.valType)
		}
		args = append(args, arg.sql)
		argTypes = append(argTypes, arg.valType)
	}
	return args, argTypes, nil
}

func (c *compiler) finishCall(n *Node, spec FuncSpec, args []string, argTypes []string, forceType string) (emitted, error) {
	resultType := spec.Result
	if spec.ResultFn != nil {
		t, err := spec.ResultFn(argTypes)
		if err != nil {
			return emitted{}, fmt.Errorf("%s: %s", n.Name, err.Error())
		}
		resultType = t
	}
	if forceType != "" {
		resultType = forceType
	}

	emitFn := c.dialect.emitter(n.Name)
	if emitFn == nil {
		return emitted{}, fmt.Errorf("%s is not supported on %s", n.Name, c.dialect.name())
	}
	sql, err := emitFn(&c.dialect, args, argTypes)
	if err != nil {
		return emitted{}, err
	}
	prec := precPrimary
	if n.Name == "ISNULL" {
		prec = precComparison
	}
	return emitted{sql: sql, valType: resultType, prec: prec}, nil
}

// emitAggregate builds the whole subquery, with the aggregate inside it. The
// hop context makes references to the target table resolve to the subquery
// alias while own-table references stay correlated to the outer query.
func (c *compiler) emitAggregate(n *Node, spec FuncSpec) (emitted, error) {
	if c.aggDepth > 0 {
		return emitted{}, fmt.Errorf("%s cannot be nested inside another aggregate", n.Name)
	}
	for _, arg := range n.Args {
		if containsAggregate(arg) {
			return emitted{}, fmt.Errorf("%s cannot be nested inside another aggregate", n.Name)
		}
	}
	hops, err := c.manyHopsIn(n)
	if err != nil {
		return emitted{}, err
	}
	if len(hops) == 0 {
		return emitted{}, fmt.Errorf("%s has to wrap a reference reached through a one-to-many relationship, for example SUM({child.amount})", n.Name)
	}
	if len(hops) > 1 {
		return emitted{}, fmt.Errorf("%s reaches %d related tables - an aggregate can only reach one", n.Name, len(hops))
	}
	hop := hops[0]

	alias := c.nextHopAlias()
	prevCtx := c.hopCtx
	c.hopCtx = &hopContext{table: hop.TargetTable, alias: alias}
	c.aggDepth = c.aggDepth + 1
	args, argTypes, aerr := c.emitArgs(n, spec)
	c.aggDepth = c.aggDepth - 1
	c.hopCtx = prevCtx
	if aerr != nil {
		return emitted{}, aerr
	}

	inner, err := c.finishCall(n, spec, args, argTypes, "")
	if err != nil {
		return emitted{}, err
	}
	sql, err := c.hopSubquery(inner.sql, hop, alias)
	if err != nil {
		return emitted{}, err
	}

	// An empty child set yields NULL, which would poison whatever the
	// expression does next. Zero is the right identity for SUM and COUNT;
	// for MIN, MAX and AVG empty genuinely means unknown.
	if n.Name == "SUM" || n.Name == "COUNT" {
		sql = fmt.Sprint("COALESCE(", sql, ", 0)")
	}

	c.hasAgg = true
	c.aggHops = c.aggHops + 1
	c.warnUnindexedHop(hop)
	return emitted{sql: sql, valType: inner.valType, prec: precPrimary}, nil
}

// emitDerivedRef inlines another derived field on the same table. The chain is
// tracked so a cycle is caught here rather than looping forever, which also
// makes cycle detection at save time free - saving runs the same compile.
func (c *compiler) emitDerivedRef(name string) (emitted, error) {
	chain := append([]string{c.field}, c.inlining...)
	for _, inFlight := range chain {
		if inFlight == name {
			return emitted{}, fmt.Errorf("derived field %s is part of a cycle: %s -> %s", name, strings.Join(chain, " -> "), name)
		}
	}
	if len(c.inlining) >= c.maxInline {
		return emitted{}, fmt.Errorf("derived field %s builds on %d other derived fields, the limit is %d", name, len(c.inlining)+1, c.maxInline)
	}

	df, ok := c.schema.Derived[c.table][name]
	if !ok {
		return emitted{}, fmt.Errorf("derived field %s does not exist on %s", name, c.table)
	}
	if df.Invalid {
		reason := df.InvalidReason
		if reason == "" {
			reason = "a dependency no longer exists"
		}
		return emitted{}, fmt.Errorf("derived field %s cannot be used: %s", name, reason)
	}

	// re-parse: the emitter only ever walks nodes our own parser created
	node, err := Parse(df.Formula)
	if err != nil {
		return emitted{}, fmt.Errorf("derived field %s: %s", name, err.Error())
	}

	c.refs[name] = true
	c.inlining = append(c.inlining, name)
	inlined, err := c.emit(node)
	c.inlining = c.inlining[:len(c.inlining)-1]
	if err != nil {
		return emitted{}, fmt.Errorf("derived field %s: %s", name, err.Error())
	}
	return inlined, nil
}

func containsAggregate(n *Node) bool {
	if n == nil {
		return false
	}
	if n.Kind == NodeFunc {
		if spec, ok := catalog[n.Name]; ok && spec.IsAggregate {
			return true
		}
	}
	for _, child := range append([]*Node{n.Operand, n.Left, n.Right}, n.Args...) {
		if containsAggregate(child) {
			return true
		}
	}
	return false
}

func checkArity(name string, spec FuncSpec, count int) error {
	if count < spec.MinArgs {
		return fmt.Errorf("%s needs at least %d argument(s), got %d", name, spec.MinArgs, count)
	}
	if spec.MaxArgs >= 0 && count > spec.MaxArgs {
		return fmt.Errorf("%s takes at most %d argument(s), got %d", name, spec.MaxArgs, count)
	}
	return nil
}

func unqualify(table string) string {
	if i := strings.LastIndex(table, "."); i >= 0 {
		return table[i+1:]
	}
	return table
}
