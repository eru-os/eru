package derived

import (
	"fmt"
	"strings"
)

// SecuredSource returns the FROM-clause source for a hop target with the
// target's row-level security rule already applied - either the bare table name
// when it carries no rule, or a parenthesised select that applies it. denied
// reports a rule that refuses access, which has to fail the query rather than
// emit a false predicate and return a misleading zero.
//
// The compiler will not compile a hop without one.
type SecuredSource func(table string) (source string, denied bool, err error)

type hopContext struct {
	table string
	alias string
}

func (c *compiler) nextHopAlias() string {
	alias := fmt.Sprint("dc", c.hopSeq)
	c.hopSeq = c.hopSeq + 1
	return alias
}

func (c *compiler) recordHop(hop Hop) {
	for _, h := range c.hops {
		if h.TargetTable == hop.TargetTable {
			return
		}
	}
	c.hops = append(c.hops, hop)
}

func (c *compiler) outerAliasName() string {
	if c.outerAlias != "" {
		return c.outerAlias
	}
	return unqualify(c.table)
}

// hopSource asks the caller for the target's secured source. Every generated
// subquery reads through this, so a hop can never see rows the caller could not
// have selected directly.
func (c *compiler) hopSource(table string) (string, error) {
	if c.security == nil {
		if c.validateOnly {
			return table, nil
		}
		return "", fmt.Errorf("refusing to reach %s without its security rule", table)
	}
	source, denied, err := c.security(table)
	if err != nil {
		return "", err
	}
	if denied {
		return "", fmt.Errorf("access denied for table %s", table)
	}
	if strings.TrimSpace(source) == "" {
		return "", fmt.Errorf("no source returned for %s", table)
	}
	return source, nil
}

func (c *compiler) hopJoinCondition(hop Hop, alias string) (string, error) {
	if len(hop.OwnCols) == 0 || len(hop.OwnCols) != len(hop.TargetCols) {
		return "", fmt.Errorf("the join between %s and %s has mismatched keys", c.table, hop.TargetTable)
	}
	outer := c.outerAliasName()
	conds := make([]string, 0, len(hop.OwnCols))
	for i := range hop.OwnCols {
		conds = append(conds, fmt.Sprint(
			c.dialect.QuoteIdent(alias), ".", c.dialect.QuoteIdent(hop.TargetCols[i]),
			" = ",
			c.dialect.QuoteIdent(outer), ".", c.dialect.QuoteIdent(hop.OwnCols[i]),
		))
	}
	return strings.Join(conds, " AND "), nil
}

func (c *compiler) hopSubquery(inner string, hop Hop, alias string) (string, error) {
	source, err := c.hopSource(hop.TargetTable)
	if err != nil {
		return "", err
	}
	cond, err := c.hopJoinCondition(hop, alias)
	if err != nil {
		return "", err
	}
	return fmt.Sprint("(SELECT ", inner, " FROM ", source, " ", alias, " WHERE ", cond, ")"), nil
}

// emitHopRef renders a lookup reference - a hop whose target key is unique, so
// at most one row matches - as a correlated scalar subquery. A one-to-many
// reference never reaches here: it has to sit inside an aggregate, which builds
// its own subquery with the aggregate pushed down into it.
func (c *compiler) emitHopRef(ref resolvedRef) (emitted, error) {
	hop := *ref.hop
	if hop.Cardinality == CardinalityMany {
		return emitted{}, fmt.Errorf("{%s.%s} comes from a list of related records - wrap it in an aggregate: SUM({%s.%s}), COUNT(…), MIN/MAX/AVG(…)",
			unqualify(hop.TargetTable), ref.column, unqualify(hop.TargetTable), ref.column)
	}
	alias := c.nextHopAlias()
	inner := fmt.Sprint(c.dialect.QuoteIdent(alias), ".", c.dialect.QuoteIdent(ref.column))
	sql, err := c.hopSubquery(inner, hop, alias)
	if err != nil {
		return emitted{}, err
	}
	return emitted{sql: sql, valType: ref.valType, prec: precPrimary}, nil
}

// manyHopsIn returns the distinct one-to-many hops an aggregate's arguments
// reach. Resolution errors win over the "needs an aggregate" message so the
// caller sees the real problem.
func (c *compiler) manyHopsIn(n *Node) ([]Hop, error) {
	var hops []Hop
	seen := map[string]bool{}
	var firstErr error
	for _, arg := range n.Args {
		walkFields(arg, func(f *Node) {
			if !strings.Contains(f.Path, ".") {
				return
			}
			ref, err := c.resolveRef(f)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			if ref.hop == nil || ref.hop.Cardinality != CardinalityMany {
				return
			}
			if seen[ref.hop.TargetTable] {
				return
			}
			seen[ref.hop.TargetTable] = true
			hops = append(hops, *ref.hop)
		})
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return hops, nil
}

// hopCardinality is derived, never configured. A hop is one-to-one when the
// join's target-side columns cover a whole primary key or unique constraint.
func (c *compiler) hopCardinality(hop Hop) string {
	cols := c.schema.Tables[hop.TargetTable]
	if len(hop.TargetCols) == 0 || len(cols) == 0 {
		return CardinalityMany
	}
	keySet := map[string]bool{}
	for _, name := range hop.TargetCols {
		keySet[name] = true
	}

	named := map[string][]string{}
	var unnamedPk, unnamedUq []string
	for name, col := range cols {
		if col.PrimaryKey {
			if col.PkConstraintName != "" {
				named["pk:"+col.PkConstraintName] = append(named["pk:"+col.PkConstraintName], name)
			} else {
				unnamedPk = append(unnamedPk, name)
			}
		}
		if col.IsUnique {
			if col.UqConstraintName != "" {
				named["uq:"+col.UqConstraintName] = append(named["uq:"+col.UqConstraintName], name)
			} else {
				unnamedUq = append(unnamedUq, name)
			}
		}
	}
	// An unnamed set is treated as one composite constraint - the safe reading
	// when the metadata cannot tell us which columns belong together.
	if len(unnamedPk) > 0 {
		named["pk:"] = unnamedPk
	}
	if len(unnamedUq) > 0 {
		named["uq:"] = unnamedUq
	}

	for _, members := range named {
		covered := len(members) > 0
		for _, m := range members {
			if !keySet[m] {
				covered = false
				break
			}
		}
		if covered {
			return CardinalityOne
		}
	}
	return CardinalityMany
}

// warnUnindexedHop flags a hop key the database will have to scan. We already
// hold enough metadata to know.
func (c *compiler) warnUnindexedHop(hop Hop) {
	cols := c.schema.Tables[hop.TargetTable]
	for _, name := range hop.TargetCols {
		col, ok := cols[name]
		if ok && (col.PrimaryKey || col.IsUnique || col.FkConstraintName != "") {
			return
		}
	}
	c.warn(fmt.Sprint("the join key on ", hop.TargetTable, " has no primary key, unique or foreign key constraint - the generated subquery will scan the table for every row"))
}

// SourceSecuredDownstream is for a caller that applies row-level security to the
// finished SQL itself rather than per hop - the hand-written SQL entry point,
// where secureSQL parses the rewritten statement and wraps every table it finds,
// the hop targets this compiler introduced included.
//
// Passing it asserts that a later pass applies the target's rule. It is wrong
// anywhere that assertion does not hold - notably the GraphQL path, which
// secures the main query through its own clause map and never re-parses the SQL.
// Applying the rule in both places is not merely redundant: secureSQL would wrap
// the source this function returned a second time, aliasing it to the qualified
// table name, which is not valid SQL.
func SourceSecuredDownstream(table string) (source string, denied bool, err error) {
	return table, false, nil
}
