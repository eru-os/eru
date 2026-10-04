package derived

import (
	"fmt"
	"strings"
)

type parser struct {
	tokens []token
	pos    int
}

// Parse turns formula text into our own AST. A calc_ast supplied by a client is
// never used - the emitter only ever walks nodes this parser produced.
func Parse(formula string) (*Node, error) {
	if strings.TrimSpace(formula) == "" {
		return nil, fmt.Errorf("formula is empty")
	}
	if len(formula) > MaxFormulaLen {
		return nil, fmt.Errorf("formula is %d characters, the limit is %d", len(formula), MaxFormulaLen)
	}
	tokens, err := lex(formula)
	if err != nil {
		return nil, err
	}
	p := &parser{tokens: tokens}
	node, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.peek().kind != tokEOF {
		return nil, fmt.Errorf("unexpected %s at position %d", p.describe(p.peek()), p.peek().start)
	}
	if c := nodeCount(node); c > MaxNodeCount {
		return nil, fmt.Errorf("formula has %d nodes, the limit is %d", c, MaxNodeCount)
	}
	if d := nodeDepth(node); d > MaxNodeDepth {
		return nil, fmt.Errorf("formula nests %d levels deep, the limit is %d", d, MaxNodeDepth)
	}
	return node, nil
}

func (p *parser) peek() token { return p.tokens[p.pos] }

func (p *parser) next() token {
	t := p.tokens[p.pos]
	if p.pos < len(p.tokens)-1 {
		p.pos = p.pos + 1
	}
	return t
}

func (p *parser) describe(t token) string {
	switch t.kind {
	case tokEOF:
		return "end of formula"
	case tokField:
		return fmt.Sprint("field reference {", t.text, "}")
	case tokString:
		return "text literal"
	case tokNumber:
		return fmt.Sprint("number ", t.text)
	default:
		return fmt.Sprint("\"", t.text, "\"")
	}
}

func (p *parser) isKeyword(t token, word string) bool {
	return t.kind == tokIdent && strings.EqualFold(t.text, word)
}

func (p *parser) parseOr() (*Node, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.isKeyword(p.peek(), "OR") {
		p.next()
		right, rerr := p.parseAnd()
		if rerr != nil {
			return nil, rerr
		}
		left = &Node{Kind: NodeBinary, Op: "OR", Left: left, Right: right, StartPos: left.StartPos, EndPos: right.EndPos}
	}
	return left, nil
}

func (p *parser) parseAnd() (*Node, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	for p.isKeyword(p.peek(), "AND") {
		p.next()
		right, rerr := p.parseNot()
		if rerr != nil {
			return nil, rerr
		}
		left = &Node{Kind: NodeBinary, Op: "AND", Left: left, Right: right, StartPos: left.StartPos, EndPos: right.EndPos}
	}
	return left, nil
}

func (p *parser) parseNot() (*Node, error) {
	if p.isKeyword(p.peek(), "NOT") {
		t := p.next()
		operand, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return &Node{Kind: NodeUnary, Op: "NOT", Operand: operand, StartPos: t.start, EndPos: operand.EndPos}, nil
	}
	return p.parseComparison()
}

var comparisonOps = map[string]bool{"=": true, "!=": true, "<": true, "<=": true, ">": true, ">=": true}

func (p *parser) parseComparison() (*Node, error) {
	left, err := p.parseAdditive()
	if err != nil {
		return nil, err
	}
	t := p.peek()
	if t.kind == tokOp && comparisonOps[t.text] {
		p.next()
		right, rerr := p.parseAdditive()
		if rerr != nil {
			return nil, rerr
		}
		return &Node{Kind: NodeBinary, Op: t.text, Left: left, Right: right, StartPos: left.StartPos, EndPos: right.EndPos}, nil
	}
	return left, nil
}

func (p *parser) parseAdditive() (*Node, error) {
	left, err := p.parseMultiplicative()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		if t.kind != tokOp || (t.text != "+" && t.text != "-") {
			return left, nil
		}
		p.next()
		right, rerr := p.parseMultiplicative()
		if rerr != nil {
			return nil, rerr
		}
		left = &Node{Kind: NodeBinary, Op: t.text, Left: left, Right: right, StartPos: left.StartPos, EndPos: right.EndPos}
	}
}

func (p *parser) parseMultiplicative() (*Node, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		if t.kind != tokOp || (t.text != "*" && t.text != "/" && t.text != "%") {
			return left, nil
		}
		p.next()
		right, rerr := p.parseUnary()
		if rerr != nil {
			return nil, rerr
		}
		left = &Node{Kind: NodeBinary, Op: t.text, Left: left, Right: right, StartPos: left.StartPos, EndPos: right.EndPos}
	}
}

func (p *parser) parseUnary() (*Node, error) {
	t := p.peek()
	if t.kind == tokOp && (t.text == "-" || t.text == "+") {
		p.next()
		operand, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		if t.text == "+" {
			return operand, nil
		}
		return &Node{Kind: NodeUnary, Op: "-", Operand: operand, StartPos: t.start, EndPos: operand.EndPos}, nil
	}
	return p.parsePrimary()
}

func (p *parser) parsePrimary() (*Node, error) {
	t := p.next()
	switch t.kind {
	case tokNumber:
		return numberNode(t.num, t.start, t.end), nil
	case tokString:
		return stringNode(t.text, t.start, t.end), nil
	case tokField:
		return fieldNode(t.text, t.start, t.end), nil
	case tokLParen:
		// parentheses are dropped - the tree already carries precedence
		inner, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.peek().kind != tokRParen {
			return nil, fmt.Errorf("missing closing parenthesis for the group opened at position %d", t.start)
		}
		p.next()
		return inner, nil
	case tokIdent:
		upper := strings.ToUpper(t.text)
		switch upper {
		case "TRUE":
			return booleanNode(true, t.start, t.end), nil
		case "FALSE":
			return booleanNode(false, t.start, t.end), nil
		case "NULL":
			return nullNode(t.start, t.end), nil
		case "AND", "OR", "NOT":
			return nil, fmt.Errorf("unexpected %s at position %d", upper, t.start)
		}
		if p.peek().kind != tokLParen {
			return nil, fmt.Errorf("unknown name %q at position %d - field references must be wrapped in braces: {%s}", t.text, t.start, t.text)
		}
		return p.parseCall(upper, t.start)
	}
	return nil, fmt.Errorf("unexpected %s at position %d", p.describe(t), t.start)
}

func (p *parser) parseCall(name string, start int) (*Node, error) {
	p.next() // consume (
	node := &Node{Kind: NodeFunc, Name: name, StartPos: start}
	if p.peek().kind == tokRParen {
		end := p.next()
		node.EndPos = end.end
		return node, nil
	}
	for {
		arg, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		node.Args = append(node.Args, arg)
		switch p.peek().kind {
		case tokComma:
			p.next()
		case tokRParen:
			end := p.next()
			node.EndPos = end.end
			return node, nil
		default:
			return nil, fmt.Errorf("expected , or ) in the call to %s at position %d", name, p.peek().start)
		}
	}
}
