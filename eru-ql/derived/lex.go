package derived

import (
	"fmt"
	"strconv"
	"strings"
)

type tokenKind int

const (
	tokEOF tokenKind = iota
	tokNumber
	tokString
	tokIdent
	tokField
	tokOp
	tokLParen
	tokRParen
	tokComma
)

type token struct {
	kind  tokenKind
	text  string
	num   float64
	start int
	end   int
}

var keywords = map[string]bool{
	"AND": true, "OR": true, "NOT": true, "TRUE": true, "FALSE": true, "NULL": true,
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentChar(c byte) bool { return isIdentStart(c) || isDigit(c) }

func lex(src string) (tokens []token, err error) {
	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i = i + 1
		case c == '(':
			tokens = append(tokens, token{kind: tokLParen, text: "(", start: i, end: i + 1})
			i = i + 1
		case c == ')':
			tokens = append(tokens, token{kind: tokRParen, text: ")", start: i, end: i + 1})
			i = i + 1
		case c == ',':
			tokens = append(tokens, token{kind: tokComma, text: ",", start: i, end: i + 1})
			i = i + 1
		case c == '{':
			tok, next, ferr := lexField(src, i)
			if ferr != nil {
				return nil, ferr
			}
			tokens = append(tokens, tok)
			i = next
		case c == '\'' || c == '"':
			tok, next, serr := lexString(src, i)
			if serr != nil {
				return nil, serr
			}
			tokens = append(tokens, tok)
			i = next
		case isDigit(c) || (c == '.' && i+1 < len(src) && isDigit(src[i+1])):
			tok, next, nerr := lexNumber(src, i)
			if nerr != nil {
				return nil, nerr
			}
			tokens = append(tokens, tok)
			i = next
		case isIdentStart(c):
			start := i
			for i < len(src) && isIdentChar(src[i]) {
				i = i + 1
			}
			tokens = append(tokens, token{kind: tokIdent, text: src[start:i], start: start, end: i})
		default:
			tok, next, oerr := lexOperator(src, i)
			if oerr != nil {
				return nil, oerr
			}
			tokens = append(tokens, tok)
			i = next
		}
	}
	tokens = append(tokens, token{kind: tokEOF, start: len(src), end: len(src)})
	return tokens, nil
}

func lexField(src string, start int) (tok token, next int, err error) {
	end := strings.IndexByte(src[start:], '}')
	if end < 0 {
		return tok, 0, fmt.Errorf("unclosed field reference at position %d - a field reference looks like {column}", start)
	}
	end = start + end
	path := src[start+1 : end]
	if path == "" {
		return tok, 0, fmt.Errorf("empty field reference at position %d", start)
	}
	for _, part := range strings.Split(path, ".") {
		if part == "" || !isIdentStart(part[0]) {
			return tok, 0, fmt.Errorf("invalid field reference {%s} at position %d", path, start)
		}
		for k := 0; k < len(part); k++ {
			if !isIdentChar(part[k]) {
				return tok, 0, fmt.Errorf("invalid field reference {%s} at position %d", path, start)
			}
		}
	}
	return token{kind: tokField, text: path, start: start, end: end + 1}, end + 1, nil
}

func lexString(src string, start int) (tok token, next int, err error) {
	quote := src[start]
	var sb strings.Builder
	i := start + 1
	for i < len(src) {
		if src[i] == quote {
			if i+1 < len(src) && src[i+1] == quote {
				sb.WriteByte(quote)
				i = i + 2
				continue
			}
			return token{kind: tokString, text: sb.String(), start: start, end: i + 1}, i + 1, nil
		}
		sb.WriteByte(src[i])
		i = i + 1
	}
	return tok, 0, fmt.Errorf("unclosed text literal at position %d", start)
}

func lexNumber(src string, start int) (tok token, next int, err error) {
	i := start
	seenDot := false
	for i < len(src) {
		if isDigit(src[i]) {
			i = i + 1
			continue
		}
		if src[i] == '.' && !seenDot {
			seenDot = true
			i = i + 1
			continue
		}
		break
	}
	text := src[start:i]
	v, cerr := strconv.ParseFloat(text, 64)
	if cerr != nil {
		return tok, 0, fmt.Errorf("invalid number %s at position %d", text, start)
	}
	return token{kind: tokNumber, text: text, num: v, start: start, end: i}, i, nil
}

var twoCharOps = map[string]bool{"!=": true, "<>": true, "<=": true, ">=": true}

func lexOperator(src string, start int) (tok token, next int, err error) {
	if start+1 < len(src) {
		if two := src[start : start+2]; twoCharOps[two] {
			if two == "<>" {
				two = "!="
			}
			return token{kind: tokOp, text: two, start: start, end: start + 2}, start + 2, nil
		}
	}
	switch src[start] {
	case '+', '-', '*', '/', '%', '=', '<', '>':
		return token{kind: tokOp, text: src[start : start+1], start: start, end: start + 1}, start + 1, nil
	}
	return tok, 0, fmt.Errorf("unexpected character %q at position %d", string(src[start]), start)
}
