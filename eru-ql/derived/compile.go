package derived

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"

	common_types "github.com/eru-os/eru/eru-ql/common_types"
)

type CompileRequest struct {
	Schema     Schema
	Table      string
	Field      common_types.DerivedFieldMetaData
	Dialect    Dialect
	OuterAlias string
	AllowHops  bool
	Security   SecuredSource
	// ValidateOnly compiles hops without a security source, for save-time and
	// preview validation. The expression it produces carries no row-level
	// security rule and must never be executed.
	ValidateOnly bool
	// MaxInlineDepth caps how deep a formula may build on other derived fields.
	// Zero means the default.
	MaxInlineDepth int
	SkipCache      bool
}

// CompiledField is a self-contained scalar expression plus what the caller needs
// to know about it. The compiler never emits a JOIN.
type CompiledField struct {
	Expr          string
	ResultType    string
	Deps          []string
	Refs          []string
	RefTables     []string
	HasAggregate  bool
	Hops          []Hop
	AggregateHops int
	Warnings      []string
}

const DefaultMaxInlineDepth = 3

func maxInlineDepth(requested int) int {
	if requested <= 0 {
		return DefaultMaxInlineDepth
	}
	return requested
}

var (
	memoMu sync.RWMutex
	memo   = map[string]CompiledField{}
)

// memoKey covers everything a hop-free compilation depends on: the formula, the
// dialect, where the expression is anchored, and a fingerprint of the owning
// table's columns, so a dropped column or a changed type invalidates the entry.
func memoKey(req CompileRequest) string {
	h := sha256.New()
	h.Write([]byte(req.Field.Formula))
	h.Write([]byte{0})

	cols := req.Schema.Tables[req.Table]
	names := make([]string, 0, len(cols))
	for name := range cols {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		col := cols[name]
		h.Write([]byte(name))
		h.Write([]byte{1})
		h.Write([]byte(pickType(col)))
		h.Write([]byte{1})
		h.Write([]byte(col.ColumnMasking.MaskingType))
		h.Write([]byte{0})
	}

	return strings.Join([]string{
		req.Dialect.name(),
		req.Table,
		req.Field.ColName,
		req.OuterAlias,
		fmt.Sprint(req.AllowHops),
		hex.EncodeToString(h.Sum(nil)[:12]),
	}, "|")
}

// Compile turns a stored formula into a dialect-correct SQL expression. Both the
// GraphQL and the SQL entry point go through here.
func Compile(req CompileRequest) (CompiledField, error) {
	if req.Field.Invalid {
		reason := req.Field.InvalidReason
		if reason == "" {
			reason = "a dependency no longer exists"
		}
		return CompiledField{}, fmt.Errorf("derived field %s on %s cannot be used: %s", req.Field.ColName, req.Table, reason)
	}
	if req.Field.CalcMode != "" && req.Field.CalcMode != common_types.CalcModeVirtual {
		return CompiledField{}, fmt.Errorf("calc_mode %s is not supported - use %s", req.Field.CalcMode, common_types.CalcModeVirtual)
	}
	// HopStrategy is a seam, not a setting: only the correlated shape is
	// implemented. Refuse anything else rather than silently emitting the
	// correlated form and letting the caller believe otherwise.
	if strategy := req.Dialect.hopStrategy(); strategy != HopCorrelated {
		return CompiledField{}, fmt.Errorf("hop strategy %s is not implemented - only %s is", strategy, HopCorrelated)
	}
	if req.AllowHops && req.Security == nil && !req.ValidateOnly {
		return CompiledField{}, fmt.Errorf("cross-table references need the target table's security rule - refusing to compile %s without it", req.Field.ColName)
	}

	key := memoKey(req)
	if !req.SkipCache {
		memoMu.RLock()
		cached, ok := memo[key]
		memoMu.RUnlock()
		if ok {
			return cached, nil
		}
	}

	node, err := Parse(req.Field.Formula)
	if err != nil {
		return CompiledField{}, fmt.Errorf("derived field %s on %s: %s", req.Field.ColName, req.Table, err.Error())
	}

	c := &compiler{
		schema:       req.Schema,
		dialect:      req.Dialect,
		table:        req.Table,
		field:        req.Field.ColName,
		outerAlias:   req.OuterAlias,
		allowHops:    req.AllowHops,
		security:     req.Security,
		validateOnly: req.ValidateOnly,
		deps:         map[string]bool{},
		refTables:    map[string]bool{},
		refs:         map[string]bool{},
		maxInline:    maxInlineDepth(req.MaxInlineDepth),
	}

	root, err := c.emit(node)
	if err != nil {
		return CompiledField{}, fmt.Errorf("derived field %s on %s: %s", req.Field.ColName, req.Table, err.Error())
	}

	expr := root.sql
	resultType := root.valType
	if resultType == TypeBoolean && c.dialect.BoolNeedsWrap {
		expr = fmt.Sprint("CASE WHEN ", expr, " THEN 1 ELSE 0 END")
	} else if root.prec < precPrimary {
		expr = fmt.Sprint("(", expr, ")")
	}

	// The caller declares data_type - the type it expects the formula to
	// produce. We map that to a value type ourselves rather than asking for our
	// own vocabulary. calc_result_type is an output, not an input: whatever
	// arrives in it is replaced by what the formula actually produces.
	if declared := strings.TrimSpace(req.Field.DataType); declared != "" && resultType != TypeNull {
		if want := ValueTypeOf(declared); want != resultType {
			return CompiledField{}, fmt.Errorf(
				"derived field %s on %s produces a %s value, but data_type %q is a %s type",
				req.Field.ColName, req.Table, resultType, declared, want)
		}
	}

	compiled := CompiledField{
		Expr:          expr,
		ResultType:    resultType,
		Deps:          sortedKeys(c.deps),
		Refs:          sortedKeys(c.refs),
		RefTables:     sortedKeys(c.refTables),
		HasAggregate:  c.hasAgg,
		Hops:          c.hops,
		AggregateHops: c.aggHops,
		Warnings:      c.warnings,
	}

	// Only self-contained expressions are cacheable. Anything that reaches
	// another table has the target's security predicate baked into it, and that
	// predicate is built from the caller's own variables - caching it would
	// serve one caller's rules to the next; join configuration is not part of
	// the key either, so a deactivated join must not hit a stale entry. Anything
	// that inlines another derived field depends on that field's formula, which
	// the key does not cover.
	if len(compiled.RefTables) == 0 && len(compiled.Refs) == 0 {
		memoMu.Lock()
		memo[key] = compiled
		memoMu.Unlock()
	}
	return compiled, nil
}

func sortedKeys(m map[string]bool) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
