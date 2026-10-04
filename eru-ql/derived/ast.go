package derived

import (
	common_types "github.com/eru-os/eru/eru-ql/common_types"
)

const (
	NodeNumber  = "number"
	NodeString  = "string"
	NodeBoolean = "boolean"
	NodeNull    = "null"
	NodeField   = "field"
	NodeUnary   = "unary"
	NodeBinary  = "binary"
	NodeFunc    = "func"
)

const (
	TypeNumber  = common_types.CalcTypeNumber
	TypeText    = common_types.CalcTypeText
	TypeDate    = common_types.CalcTypeDate
	TypeBoolean = common_types.CalcTypeBoolean
	TypeNull    = "null"
)

const (
	MaxFormulaLen = 4000
	MaxNodeCount  = 500
	MaxNodeDepth  = 32
)

type Node = common_types.CalcNode

func numberNode(v float64, start int, end int) *Node {
	return &Node{Kind: NodeNumber, Value: v, StartPos: start, EndPos: end}
}

func stringNode(v string, start int, end int) *Node {
	return &Node{Kind: NodeString, Value: v, StartPos: start, EndPos: end}
}

func booleanNode(v bool, start int, end int) *Node {
	return &Node{Kind: NodeBoolean, Value: v, StartPos: start, EndPos: end}
}

func nullNode(start int, end int) *Node {
	return &Node{Kind: NodeNull, StartPos: start, EndPos: end}
}

func fieldNode(path string, start int, end int) *Node {
	return &Node{Kind: NodeField, Path: path, StartPos: start, EndPos: end}
}

func nodeCount(n *Node) int {
	if n == nil {
		return 0
	}
	count := 1
	count = count + nodeCount(n.Operand) + nodeCount(n.Left) + nodeCount(n.Right)
	for _, a := range n.Args {
		count = count + nodeCount(a)
	}
	return count
}

func nodeDepth(n *Node) int {
	if n == nil {
		return 0
	}
	deepest := 0
	for _, child := range append([]*Node{n.Operand, n.Left, n.Right}, n.Args...) {
		if d := nodeDepth(child); d > deepest {
			deepest = d
		}
	}
	return deepest + 1
}

func walkFields(n *Node, fn func(*Node)) {
	if n == nil {
		return
	}
	if n.Kind == NodeField {
		fn(n)
	}
	walkFields(n.Operand, fn)
	walkFields(n.Left, fn)
	walkFields(n.Right, fn)
	for _, a := range n.Args {
		walkFields(a, fn)
	}
}

func numberValue(n *Node) (float64, bool) {
	if n == nil || n.Kind != NodeNumber {
		return 0, false
	}
	switch v := n.Value.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	}
	return 0, false
}

func stringValue(n *Node) (string, bool) {
	if n == nil || n.Kind != NodeString {
		return "", false
	}
	if s, ok := n.Value.(string); ok {
		return s, true
	}
	return "", false
}
